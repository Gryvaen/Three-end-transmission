import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:file_selector/file_selector.dart';

import 'platform.dart';
import 'windows_ffi.dart' as win;

typedef Codec = Uint8List Function(Uint8List data);

// Desktop implementation of the bridge in plain Dart. Windows uses DPAPI,
// GetAdaptersAddresses and SetThreadExecutionState; the Linux fallbacks exist
// so the desktop layout can be developed and smoke-tested off Windows.
class DesktopBridge extends NativeBridge {
  final String? root;
  final Codec _encrypt, _decrypt;
  final Duration pollInterval;
  Map<String, String>? _secrets;
  Future<void> _writes = Future.value();

  DesktopBridge({
    this.root,
    Codec? encrypt,
    Codec? decrypt,
    this.pollInterval = const Duration(seconds: 5),
  }) : _encrypt = encrypt ?? (Platform.isWindows ? win.protect : _identity),
       _decrypt = decrypt ?? (Platform.isWindows ? win.unprotect : _identity),
       super.base();

  static Uint8List _identity(Uint8List data) => data;

  @override
  String get platform => Platform.operatingSystem;

  @override
  Future<String> get directory async {
    final env = Platform.environment;
    final path =
        root ??
        (Platform.isWindows
            ? '${env['LOCALAPPDATA'] ?? env['APPDATA'] ?? Directory.systemTemp.path}\\LanRoom'
            : '${env['XDG_DATA_HOME'] ?? '${env['HOME']}/.local/share'}/lanroom');
    await Directory(path).create(recursive: true);
    return path;
  }

  Future<File> get _store async =>
      File('${await directory}${Platform.pathSeparator}secrets.json');

  Future<Map<String, String>> _load() async {
    if (_secrets != null) {
      return _secrets!;
    }
    final file = await _store;
    final map = <String, String>{};
    if (await file.exists()) {
      final data = jsonDecode(await file.readAsString()) as Map;
      data.forEach((k, v) => map[k as String] = v as String);
    }
    return _secrets = map;
  }

  @override
  Future<String?> read(String key) async {
    final value = (await _load())[key];
    if (value == null) {
      return null;
    }
    // A value we cannot decrypt (e.g. copied from another account) is treated
    // as missing rather than blocking startup.
    try {
      return utf8.decode(_decrypt(base64.decode(value)));
    } catch (_) {
      return null;
    }
  }

  // Writes are chained so the file on disk always reflects the latest map;
  // the temp-file rename keeps a crash from leaving a torn JSON file.
  @override
  Future<void> write(String key, String value) {
    final next = _writes.then((_) async {
      final map = await _load();
      map[key] = base64.encode(_encrypt(utf8.encode(value)));
      final file = await _store;
      final temp = File('${file.path}.tmp');
      await temp.writeAsString(jsonEncode(map), flush: true);
      await temp.rename(file.path);
    });
    _writes = next.catchError((Object _) {});
    return next;
  }

  @override
  Future<String?> get deviceName async {
    final name = Platform.isWindows
        ? Platform.environment['COMPUTERNAME']
        : Platform.localHostname;
    return name == null || name.trim().isEmpty ? null : name.trim();
  }

  @override
  Future<List<Map<String, dynamic>>> get interfaces async {
    try {
      if (Platform.isWindows) {
        return win.adapters();
      }
      if (Platform.isLinux) {
        final result = await Process.run('ip', ['-4', '-o', 'addr', 'show']);
        if (result.exitCode == 0) {
          return parseIpAddr(result.stdout as String);
        }
      }
    } catch (_) {
      /* Fall back to the portable list below. */
    }
    final list = <Map<String, dynamic>>[];
    for (final iface in await NetworkInterface.list(
      type: InternetAddressType.IPv4,
    )) {
      if (_virtual(iface.name)) {
        continue;
      }
      for (final address in iface.addresses) {
        if (!address.isLoopback && !address.isLinkLocal) {
          list.add({
            'name': iface.name,
            'ip': address.address,
            'broadcast': '255.255.255.255',
          });
        }
      }
    }
    return list;
  }

  @override
  Future<Map<String, dynamic>?> pick() async {
    final file = await openFile(confirmButtonText: '发送');
    return file == null ? null : describeFile(file.path);
  }

  @override
  Future<bool> save(String path, String name) async {
    final target = await getSaveLocation(
      suggestedName: name,
      confirmButtonText: '保存',
    );
    if (target == null) {
      return false;
    }
    try {
      await File(path).rename(target.path);
    } on FileSystemException {
      // Rename cannot cross volumes; copy instead and let the caller clean up.
      await File(path).copy(target.path);
    }
    return true;
  }

  @override
  Future<bool> permission() async => true;

  // Desktop processes are not frozen in the background; nothing to keep alive.
  @override
  Future<void> keepAlive(bool on, [String? label]) async {}

  @override
  Future<void> transferring(bool on) async {
    if (Platform.isWindows) {
      win.keepAwake(on);
    }
  }

  // No OS callback without a native plugin: poll the address set instead.
  @override
  Stream<dynamic> get network {
    Timer? timer;
    String? last;
    late final StreamController<dynamic> controller;
    controller = StreamController<dynamic>(
      onListen: () {
        timer = Timer.periodic(pollInterval, (_) async {
          try {
            final current = await addressSignature();
            if (last != null && current != last) {
              controller.add('changed');
            }
            last = current;
          } catch (_) {}
        });
      },
      onCancel: () => timer?.cancel(),
    );
    return controller.stream;
  }
}

Future<String> addressSignature() async {
  final entries = <String>[];
  for (final iface in await NetworkInterface.list(
    type: InternetAddressType.IPv4,
  )) {
    for (final address in iface.addresses) {
      entries.add('${iface.name}=${address.address}');
    }
  }
  entries.sort();
  return entries.join(',');
}

bool _virtual(String name) => RegExp(
  r'^(lo|tun|tap|utun|wg|tailscale|docker|veth|br-|virbr|meta|mihomo|clash)',
  caseSensitive: false,
).hasMatch(name);

/// Parses `ip -4 -o addr show` lines such as
/// `2: wlan0    inet 192.168.1.5/24 brd 192.168.1.255 scope global ...`.
List<Map<String, dynamic>> parseIpAddr(String output) {
  final pattern = RegExp(
    r'^\d+:\s+(\S+)\s+inet\s+([\d.]+)/\d+\s+brd\s+([\d.]+)',
  );
  final list = <Map<String, dynamic>>[];
  for (final line in const LineSplitter().convert(output)) {
    final match = pattern.firstMatch(line);
    if (match != null && !_virtual(match[1]!)) {
      list.add({'name': match[1], 'ip': match[2], 'broadcast': match[3]});
    }
  }
  return list;
}

/// Upload job fields for a file the user picked or dropped. The path is the
/// original file, so Room must not delete it afterwards (`owned: false`).
Future<Map<String, dynamic>> describeFile(String path) async {
  final last = path.split(RegExp(r'[/\\]')).last;
  final name = last.isEmpty ? '文件' : last;
  return {
    'path': path,
    'name': name,
    'size': await File(path).length(),
    'mime': mimeFor(name),
    'owned': false,
  };
}

const _mimes = {
  'jpg': 'image/jpeg',
  'jpeg': 'image/jpeg',
  'png': 'image/png',
  'gif': 'image/gif',
  'webp': 'image/webp',
  'bmp': 'image/bmp',
  'heic': 'image/heic',
  'svg': 'image/svg+xml',
  'mp4': 'video/mp4',
  'mov': 'video/quicktime',
  'mkv': 'video/x-matroska',
  'mp3': 'audio/mpeg',
  'm4a': 'audio/mp4',
  'wav': 'audio/wav',
  'txt': 'text/plain',
  'md': 'text/markdown',
  'csv': 'text/csv',
  'json': 'application/json',
  'pdf': 'application/pdf',
  'zip': 'application/zip',
  'apk': 'application/vnd.android.package-archive',
  'doc': 'application/msword',
  'docx':
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'xls': 'application/vnd.ms-excel',
  'xlsx': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'ppt': 'application/vnd.ms-powerpoint',
  'pptx': 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
};

String mimeFor(String name) {
  final dot = name.lastIndexOf('.');
  return dot < 0
      ? 'application/octet-stream'
      : _mimes[name.substring(dot + 1).toLowerCase()] ??
            'application/octet-stream';
}

/// Opens [url] in the default browser.
Future<void> openUrl(String url) async {
  if (Platform.isWindows) {
    await Process.run('explorer.exe', [url]);
  } else if (Platform.isMacOS) {
    await Process.run('open', [url]);
  } else {
    await Process.run('xdg-open', [url]);
  }
}
