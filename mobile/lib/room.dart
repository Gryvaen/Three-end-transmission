import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:flutter/foundation.dart';

import 'client.dart';
import 'discovery.dart';
import 'platform.dart';

class Room extends ChangeNotifier {
  final AndroidBridge bridge;
  Room({AndroidBridge? bridge}) : bridge = bridge ?? AndroidBridge();
  String status = '正在准备连接';
  String? error;
  String nickname = '我的手机';
  String key = '';
  String? preferred;
  FoundServer? selected;
  List<FoundServer> servers = [];
  LanClient? client;
  bool busy = false, connected = false, needsPin = false, uploading = false;
  String? ownID;
  List<Map<String, dynamic>> devices = [], messages = [];
  List<String> recipients = [];
  Map<String, dynamic>? pendingUpload;
  double progress = 0;
  String? transferStatus;
  bool downloading = false;
  bool picking = false;
  double downloadProgress = 0;
  bool _disposed = false, _foreground = true;
  int _generation = 0, _transferGeneration = 0;
  StreamSubscription<dynamic>? _network, _socket;
  Timer? _retry, _debounce;
  String _directory = '';

  void changed() {
    if (!_disposed) {
      notifyListeners();
    }
  }

  Future<void> initialize() async {
    try {
      _directory = await bridge.directory;
      key = await bridge.read('deviceKey') ?? randomID(32);
      await bridge.write('deviceKey', key);
      nickname = await bridge.read('nickname') ?? nickname;
      preferred = await bridge.read('preferred');
      final pending = await bridge.read('upload');
      if (pending != null && pending.isNotEmpty) {
        pendingUpload = Map<String, dynamic>.from(jsonDecode(pending) as Map);
        transferStatus = '有未完成的上传，连接后点击继续';
      }
      // Remove files left by a killed picker/download, preserving the saved job.
      await for (final file in Directory(_directory).list()) {
        if (file is File &&
            (file.uri.pathSegments.last.startsWith('pending-') ||
                file.uri.pathSegments.last.startsWith('download-')) &&
            file.path != pendingUpload?['path']) {
          await file.delete();
        }
      }
      _network = bridge.network.listen(
        (_) {
          _debounce?.cancel();
          _debounce = Timer(const Duration(seconds: 2), () {
            if (_foreground && !busy) {
              reconnect();
            }
          });
        },
        onError: (Object e) {
          error = '网络状态监听不可用：$e';
          changed();
        },
      );
      await search();
    } catch (e) {
      fail('初始化失败：$e');
    }
  }

  void fail(String text) {
    error = text;
    changed();
  }

  void foreground(bool active) {
    _foreground = active;
    if (active && !busy) {
      reconnect();
    }
  }

  void _schedule() {
    _retry?.cancel();
    if (!_disposed) {
      _retry = Timer(const Duration(seconds: 5), () {
        if (_foreground && !busy && !needsPin && !connected) {
          search();
        } else if (!_disposed && !connected) {
          _schedule();
        }
      });
    }
  }

  Future<void> reconnect() async {
    if (busy) {
      return;
    }
    if (selected != null && connected) {
      try {
        final info = await client!.info();
        if (info['authorized'] == true) {
          return;
        }
        await attach(selected!);
        return;
      } catch (_) {
        /* Rediscover on address change. */
      }
    }
    await search();
  }

  Future<void> search({bool chooseAnother = false}) async {
    if (busy || _disposed) {
      return;
    }
    busy = true;
    error = null;
    status = '正在查找局域网服务';
    changed();
    final token = ++_generation;
    connected = false;
    needsPin = false;
    devices = [];
    await _socket?.cancel();
    client?.close();
    client = null;
    try {
      final allowed =
          await AndroidBridge.channel.invokeMethod<bool>('permission') ?? false;
      if (!allowed) {
        throw StateError('请在系统设置中允许本地网络访问');
      }
      final found = await discover(await bridge.interfaces);
      final verified = <FoundServer>[];
      for (final server in found) {
        final probe = LanClient(server.uri);
        try {
          final info = await probe.info();
          if (info['serviceId'] == server.id &&
              info['discoveryVersion'] == 1 &&
              info['pinRequired'] is bool) {
            verified.add(server);
          }
        } catch (_) {
          /* Never connect to an unverified UDP offer. */
        } finally {
          probe.close();
        }
      }
      if (_disposed || token != _generation) {
        return;
      }
      final byID = <String, FoundServer>{};
      for (final server in verified) {
        byID.putIfAbsent(server.id, () => server);
      }
      servers = byID.values.toList();
      final match = chooseAnother ? null : preferredServer(servers, preferred);
      busy = false;
      if (match != null) {
        await attach(match);
        return;
      }
      status = servers.isEmpty ? '未发现服务' : '请选择服务器';
      if (preferred != null && !chooseAnother) {
        status = '未找到上次连接的服务器';
      }
      error = servers.isEmpty
          ? '请确认电脑服务已启动、处于同一网络，且防火墙允许 UDP 8788 / TCP 8787。Mihomo 需放行局域网路由。'
          : null;
      _schedule();
    } catch (e) {
      status = '自动发现失败';
      error = e.toString();
      _schedule();
    } finally {
      if (token == _generation) {
        busy = false;
        changed();
      }
    }
  }

  Future<void> manual(String address) async {
    if (busy) {
      return;
    }
    try {
      final uri = Uri.parse(
        address.contains('://') ? address.trim() : 'http://${address.trim()}',
      );
      if (!['http', 'https'].contains(uri.scheme) ||
          uri.host.isEmpty ||
          uri.userInfo.isNotEmpty) {
        throw const FormatException('请输入 HTTP 或 HTTPS 服务地址');
      }
      final base = uri.replace(path: '', query: null, fragment: null);
      final probe = LanClient(base);
      late Map<String, dynamic> info;
      try {
        info = await probe.info();
      } finally {
        probe.close();
      }
      if (info['pinRequired'] is! bool || info['port'] is! int) {
        throw const FormatException('不是兼容的 LanRoom 服务');
      }
      final id = info['serviceId'] as String?;
      await attach(
        FoundServer(id == null || id.isEmpty ? 'manual:$base' : id, base),
      );
    } catch (e) {
      fail('连接失败：$e');
    }
  }

  Future<void> attach(FoundServer server) async {
    _retry?.cancel();
    final token = ++_generation;
    busy = true;
    connected = false;
    needsPin = false;
    error = null;
    status = '正在连接';
    selected = server;
    devices = [];
    messages = [];
    recipients = [];
    changed();
    await _socket?.cancel();
    client?.close();
    final next = LanClient(server.uri);
    client = next;
    try {
      // Check identity before sending a persisted credential to a new address.
      final plainInfo = await next.info();
      if (!server.id.startsWith('manual:') &&
          plainInfo['serviceId'] != server.id) {
        throw StateError('服务器标识不匹配');
      }
      next.cookie = await bridge.read('cookie:${server.id}');
      final info = await next.info();
      if (token != _generation || _disposed) {
        return;
      }
      if (info['pinRequired'] == true && info['authorized'] != true) {
        next.cookie = null;
        needsPin = true;
        status = '请输入房间口令';
        return;
      }
      await _open(token);
    } catch (e) {
      status = '连接失败';
      error = e.toString();
      _schedule();
    } finally {
      if (token == _generation) {
        busy = false;
        changed();
      }
    }
  }

  Future<void> login(String pin) async {
    if (busy || client == null) {
      return;
    }
    busy = true;
    error = null;
    changed();
    final token = _generation;
    final active = client!;
    try {
      await active.login(pin);
      if (_disposed || token != _generation) {
        return;
      }
      await bridge.write('cookie:${selected!.id}', client!.cookie ?? '');
      needsPin = false;
      await _open(_generation);
    } catch (e) {
      error = e.toString();
    } finally {
      busy = false;
      changed();
    }
  }

  Future<void> _open(int token) async {
    await _socket?.cancel();
    await client?.socket?.close();
    final ws = await client!.connect(nickname, key);
    if (_disposed || token != _generation) {
      await ws.close();
      return;
    }
    preferred = selected!.id;
    await bridge.write('preferred', preferred!);
    await bridge.write('lastAddress', selected!.uri.toString());
    connected = true;
    status = '已连接';
    _socket = ws.listen(
      (raw) {
        if (token != _generation) {
          return;
        }
        try {
          final message = Map<String, dynamic>.from(
            jsonDecode(raw as String) as Map,
          );
          switch (message['type']) {
            case 'welcome':
              ownID = (message['device'] as Map)['id'] as String?;
            case 'presence':
              devices = (message['users'] as List)
                  .map((e) => Map<String, dynamic>.from(e as Map))
                  .toList();
            case 'history':
              messages = (message['messages'] as List)
                  .map((e) => Map<String, dynamic>.from(e as Map))
                  .toList();
            case 'message':
              messages.add(message);
              if (messages.length > 500) {
                messages.removeAt(0);
              }
          }
          changed();
        } catch (_) {
          /* Ignore unknown protocol events. */
        }
      },
      onDone: () => _lost(token),
      onError: (Object e) {
        _lost(token);
      },
      cancelOnError: true,
    );
    changed();
  }

  void _lost(int token) {
    if (token != _generation || _disposed) {
      return;
    }
    connected = false;
    devices = [];
    status = '连接中断，正在等待重连';
    changed();
    _schedule();
  }

  Future<void> rename(String value) async {
    if (busy) {
      return;
    }
    if (value.trim().isEmpty) {
      return;
    }
    nickname = value.trim();
    await bridge.write('nickname', nickname);
    if (selected != null) {
      await attach(selected!);
    } else {
      changed();
    }
  }

  void text(String value) {
    if (value.trim().isEmpty) {
      return;
    }
    try {
      client!.send({'kind': 'text', 'content': value.trim()}, recipients);
    } catch (e) {
      fail(e.toString());
    }
  }

  Future<void> pickFile() async {
    if (!connected || pendingUpload != null || picking) {
      return;
    }
    try {
      final targetServer = selected!.id;
      final targetRecipients = List<String>.from(recipients);
      picking = true;
      changed();
      final file = await bridge.pick();
      if (file == null) {
        return;
      }
      pendingUpload = {
        ...file,
        'serviceId': targetServer,
        'to': targetRecipients,
      };
      await _saveUpload();
      changed();
      await resumeUpload();
    } catch (e) {
      fail('选择文件失败：$e');
    } finally {
      picking = false;
      changed();
    }
  }

  Future<void> _saveUpload() => bridge.write(
    'upload',
    pendingUpload == null ? '' : jsonEncode(pendingUpload),
  );
  Future<void> resumeUpload() async {
    if (uploading || !connected || pendingUpload == null) {
      return;
    }
    final job = pendingUpload!;
    if (job['serviceId'] != selected!.id) {
      fail('此上传属于另一台服务器，请连接原服务器或取消');
      return;
    }
    uploading = true;
    progress = 0;
    transferStatus = '准备上传';
    changed();
    final transfer = ++_transferGeneration;
    final activeClient = client!;
    RandomAccessFile? source;
    try {
      source = await File(job['path'] as String).open();
      Map<String, dynamic> state;
      if (job['uploadId'] != null) {
        try {
          state = await activeClient.json(
            'GET',
            '/api/uploads/${job['uploadId']}',
          );
        } on ApiException catch (e) {
          if (e.status != 404) {
            rethrow;
          }
          state = await _createUpload(activeClient, job);
        }
      } else {
        state = await _createUpload(activeClient, job);
      }
      if (transfer != _transferGeneration) {
        return;
      }
      await _saveUpload();
      int offset = state['offset'] as int;
      final size = job['size'] as int;
      int conflicts = 0;
      while (state['file'] == null) {
        if (transfer != _transferGeneration ||
            !connected ||
            activeClient != client) {
          throw StateError('上传暂停，连接恢复后可继续');
        }
        if (offset < 0 || offset > size) {
          throw StateError('服务器返回无效上传进度');
        }
        await source.setPosition(offset);
        final length = min(
          min(state['chunkSize'] as int? ?? 8388608, 1048576),
          size - offset,
        );
        if (length < 0) {
          throw StateError('无效分片大小');
        }
        final bytes = await source.read(length);
        if (bytes.length != length) {
          throw StateError('本地文件长度已变化');
        }
        final result = await activeClient.json(
          'PUT',
          '/api/uploads/${job['uploadId']}?offset=$offset',
          bytes: bytes,
          acceptConflict: true,
        );
        final next = result['offset'] as int;
        if (next == offset && result['file'] == null) {
          conflicts++;
          if (conflicts > 3) {
            throw StateError('服务器上传进度停滞');
          }
        } else {
          conflicts = 0;
        }
        offset = next;
        state = {...state, ...result};
        progress = size == 0 ? 1 : offset / size;
        transferStatus = '上传 ${(progress * 100).round()}%';
        changed();
      }
      if (transfer != _transferGeneration) {
        return;
      }
      final file = Map<String, dynamic>.from(state['file'] as Map);
      activeClient.send({
        'kind': 'file',
        'fileId': file['fileId'],
        'meta': {
          'name': file['name'],
          'size': file['size'],
          'mime': file['mime'],
        },
      }, List<String>.from(job['to'] as List));
      pendingUpload = null;
      await _saveUpload();
      await source.close();
      source = null;
      await File(job['path'] as String).delete();
      transferStatus = '上传完成';
      progress = 1;
    } catch (e) {
      if (transfer == _transferGeneration) {
        transferStatus = '上传暂停，点击继续';
        _handleExpired(e);
        error = e.toString();
      }
    } finally {
      await source?.close();
      uploading = false;
      changed();
    }
  }

  Future<Map<String, dynamic>> _createUpload(
    LanClient active,
    Map<String, dynamic> job,
  ) async {
    final state = await active.json(
      'POST',
      '/api/uploads',
      body: {'name': job['name'], 'size': job['size'], 'mime': job['mime']},
    );
    job['uploadId'] = state['uploadId'];
    return state;
  }

  void _handleExpired(Object error) {
    if (error is ApiException && error.status == 401) {
      needsPin = true;
      connected = false;
      status = '会话已过期，请输入房间口令';
    }
  }

  Future<void> cancelUpload() async {
    final job = pendingUpload;
    if (job == null) {
      return;
    }
    _transferGeneration++;
    pendingUpload = null;
    await _saveUpload();
    if (client != null &&
        selected?.id == job['serviceId'] &&
        job['uploadId'] != null) {
      try {
        final response = await client!.request(
          'DELETE',
          '/api/uploads/${job['uploadId']}',
        );
        await response.drain<void>();
      } catch (_) {
        /* Server expiry removes unreachable partial files. */
      }
    }
    try {
      await File(job['path'] as String).delete();
    } catch (_) {}
    transferStatus = '已取消上传';
    changed();
  }

  Future<String?> download(
    Map<String, dynamic> payload, {
    bool export = true,
  }) async {
    final active = client;
    if (!connected || active == null) {
      fail('请先连接服务器');
      return null;
    }
    if (downloading) {
      fail('请等待当前下载完成');
      return null;
    }
    downloading = true;
    downloadProgress = 0;
    transferStatus = '正在下载';
    changed();
    final path = '$_directory/download-${randomID()}';
    final file = File(path);
    try {
      final response = await active.request(
        'GET',
        '/api/files/${Uri.encodeComponent(payload['fileId'] as String)}',
      );
      final output = file.openWrite();
      try {
        var received = 0;
        await for (final bytes in response.timeout(
          const Duration(seconds: 45),
        )) {
          output.add(bytes);
          received += bytes.length;
          downloadProgress = response.contentLength > 0
              ? received / response.contentLength
              : 0;
          transferStatus = '下载 $received 字节';
          changed();
        }
      } finally {
        await output.close();
      }
      final name = ((payload['meta'] as Map?)?['name'] as String? ?? '文件')
          .split(RegExp(r'[/\\]'))
          .last;
      if (export) {
        final saved = await bridge.save(path, name);
        transferStatus = saved ? '已保存 $name' : '已取消保存';
        await file.delete();
        changed();
        return null;
      }
      return path;
    } catch (e) {
      if (await file.exists()) {
        await file.delete();
      }
      _handleExpired(e);
      fail('下载失败，可点击文件重试：$e');
      return null;
    } finally {
      downloading = false;
      changed();
    }
  }

  @override
  void dispose() {
    _disposed = true;
    _generation++;
    _transferGeneration++;
    _retry?.cancel();
    _debounce?.cancel();
    _network?.cancel();
    _socket?.cancel();
    client?.close();
    super.dispose();
  }
}
