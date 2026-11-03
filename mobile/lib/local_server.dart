import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/foundation.dart';

import 'client.dart';
import 'platform.dart';

// Runs the bundled Go server (lanroom-server.exe next to the app) as a child
// process so this computer can host the room without a separate `go run`.
class LocalServer extends ChangeNotifier {
  final NativeBridge bridge;
  final String? executable;
  final Duration readyTimeout;
  int port = 8787;
  String pin = '';
  bool autoStart = false;
  bool starting = false;
  String? error;
  Map<String, dynamic>? info;
  Uint8List? qr;
  final logs = <String>[];
  Process? _process;
  bool _disposed = false;

  LocalServer(
    this.bridge, {
    String? executable,
    this.readyTimeout = const Duration(seconds: 10),
  }) : executable = executable ?? bundled();

  /// The server binary shipped beside the app executable, if any.
  static String? bundled() {
    final dir = File(Platform.resolvedExecutable).parent.path;
    // Not lanroom.exe: Windows paths are case-insensitive and LanRoom.exe is
    // the app itself.
    final name = Platform.isWindows ? 'lanroom-server.exe' : 'lanroom-server';
    final file = File('$dir${Platform.pathSeparator}$name');
    return file.existsSync() ? file.path : null;
  }

  bool get available => executable != null;
  bool get running => _process != null && info != null;
  Uri get localUri => Uri.parse('http://127.0.0.1:$port');
  List<String> get urls =>
      ((info?['urls'] as List?) ?? const []).whereType<String>().toList();

  void _changed() {
    if (!_disposed) {
      notifyListeners();
    }
  }

  Future<void> load() async {
    port = int.tryParse(await bridge.read('server.port') ?? '') ?? port;
    pin = await bridge.read('server.pin') ?? pin;
    autoStart = await bridge.read('server.auto') == 'true';
    _changed();
  }

  Future<void> configure({int? port, String? pin, bool? autoStart}) async {
    if (port != null) {
      this.port = port;
      await bridge.write('server.port', '$port');
    }
    if (pin != null) {
      this.pin = pin;
      await bridge.write('server.pin', pin);
    }
    if (autoStart != null) {
      this.autoStart = autoStart;
      await bridge.write('server.auto', '$autoStart');
    }
    _changed();
  }

  /// Starts the server and waits until /api/info answers. Returns false with
  /// [error] set when the process could not start or exited early.
  Future<bool> start() async {
    if (!available || starting || _process != null) {
      return running;
    }
    starting = true;
    error = null;
    info = null;
    logs.clear();
    _changed();
    try {
      final sep = Platform.pathSeparator;
      final data = '${await bridge.directory}${sep}server';
      final process = await Process.start(
        executable!,
        ['-port', '$port'],
        environment: {
          'LANROOM_DATA_DIR': data,
          'LANROOM_UPLOAD_DIR': '$data${sep}uploads',
          'LANROOM_PIN': pin,
          // Ignore any public-server settings inherited from the user's shell.
          'LANROOM_PUBLIC_URL': '',
          'LANROOM_LISTEN': '',
          'LANROOM_EXIT_ON_STDIN_EOF': '1',
        },
      );
      _process = process;
      for (final stream in [process.stdout, process.stderr]) {
        stream
            .transform(const Utf8Decoder(allowMalformed: true))
            .transform(const LineSplitter())
            .listen(_log);
      }
      unawaited(process.exitCode.then((code) => _exited(process, code)));
      final deadline = DateTime.now().add(readyTimeout);
      while (_process == process) {
        final probe = LanClient(localUri);
        try {
          final answer = await probe.info().timeout(const Duration(seconds: 2));
          // Another program (often an older LanRoom) may already own the port
          // and answer for us; only our own data directory's identity counts.
          final own = File('$data${sep}service-id');
          if (!await own.exists() ||
              (await own.readAsString()).trim() != answer['serviceId']) {
            throw StateError('端口 $port 上是其他服务');
          }
          info = answer;
          qr = await _qrcode(probe);
          return true;
        } catch (_) {
          if (DateTime.now().isAfter(deadline)) {
            throw TimeoutException('服务器启动超时');
          }
          await Future<void>.delayed(const Duration(milliseconds: 200));
        } finally {
          probe.close();
        }
      }
      return false;
    } catch (e) {
      error ??= '启动服务器失败：$e';
      await stop();
      return false;
    } finally {
      starting = false;
      _changed();
    }
  }

  // Encode the advertised LAN URL explicitly; asked over loopback the server
  // could not tell which address other devices should use.
  Future<Uint8List?> _qrcode(LanClient probe) async {
    final target = info?['joinUrl'] as String? ?? '';
    if (target.isEmpty) {
      return null;
    }
    try {
      final response = await probe.request(
        'GET',
        '/api/qrcode?url=${Uri.encodeQueryComponent(target)}',
      );
      final builder = BytesBuilder(copy: false);
      await response.forEach(builder.add);
      return builder.takeBytes();
    } catch (_) {
      return null;
    }
  }

  void _log(String line) {
    logs.add(line);
    if (logs.length > 200) {
      logs.removeAt(0);
    }
    _changed();
  }

  void _exited(Process process, int code) {
    if (_process != process) {
      return;
    }
    _process = null;
    info = null;
    qr = null;
    final tail = logs.length > 3 ? logs.sublist(logs.length - 3) : logs;
    error =
        '服务器已退出（代码 $code），端口 $port 可能被占用'
        '${tail.isEmpty ? '' : '\n${tail.join('\n')}'}';
    _changed();
  }

  Future<void> stop() async {
    final process = _process;
    _process = null;
    info = null;
    qr = null;
    if (process == null) {
      _changed();
      return;
    }
    // Closing stdin asks the server to exit; kill it if it does not.
    try {
      await process.stdin.close();
    } catch (_) {}
    try {
      await process.exitCode.timeout(const Duration(seconds: 2));
    } on TimeoutException {
      process.kill();
    }
    _changed();
  }

  @override
  void dispose() {
    _disposed = true;
    final process = _process;
    _process = null;
    if (process != null) {
      unawaited(process.stdin.close().catchError((Object _) {}));
      Timer(const Duration(seconds: 2), process.kill);
    }
    super.dispose();
  }
}
