import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:lanroom_mobile/desktop_bridge.dart';
import 'package:lanroom_mobile/discovery.dart';
import 'package:lanroom_mobile/local_server.dart';
import 'package:lanroom_mobile/room.dart';

Future<int> freePort() async {
  final socket = await ServerSocket.bind(InternetAddress.anyIPv4, 0);
  final port = socket.port;
  await socket.close();
  return port;
}

Future<void> until(bool Function() condition) async {
  final deadline = DateTime.now().add(const Duration(seconds: 10));
  while (!condition()) {
    if (DateTime.now().isAfter(deadline)) {
      fail('等待超时');
    }
    await Future<void>.delayed(const Duration(milliseconds: 20));
  }
}

// Exercises the real Go server built from the repository root.
void main() {
  late Directory temp;
  String? binary;
  setUpAll(() async {
    temp = await Directory.systemTemp.createTemp('lanroom-local-');
    final out =
        '${temp.path}${Platform.pathSeparator}${Platform.isWindows ? 'lanroom.exe' : 'lanroom'}';
    try {
      final result = await Process.run('go', [
        'build',
        '-o',
        out,
        '.',
      ], workingDirectory: '..');
      if (result.exitCode == 0) {
        binary = out;
      } else {
        // ignore: avoid_print
        print('go build failed: ${result.stderr}');
      }
    } on ProcessException {
      /* No Go toolchain; the tests below are skipped. */
    }
  });
  tearDownAll(() => temp.delete(recursive: true));

  LocalServer create(String name) {
    final bridge = DesktopBridge(root: '${temp.path}/$name');
    return LocalServer(bridge, executable: binary)
      ..port = 0
      ..autoStart = false;
  }

  test('内置服务器启动后连接自身，上传不删除原文件，停止后释放端口', () async {
    if (binary == null) {
      markTestSkipped('需要 Go 工具链');
      return;
    }
    final server = create('host');
    server.port = await freePort();
    expect(await server.start(), isTrue, reason: server.logs.join('\n'));
    expect(server.running, isTrue);
    expect(server.info!['serviceId'], isNotEmpty);
    expect(server.qr, isNotNull);

    final room = Room(bridge: server.bridge)..key = randomID(32);
    try {
      await room.manual(server.localUri.toString());
      expect(room.connected, isTrue, reason: room.error);
      expect(room.selected!.id, server.info!['serviceId']);
      await until(() => room.devices.isNotEmpty);
      expect(room.devices.single['platform'], Platform.operatingSystem);

      final source = File('${temp.path}/hello.txt')
        ..writeAsStringSync('hello lanroom');
      await room.sendFile(await describeFile(source.path));
      expect(room.transferStatus, '上传完成', reason: room.error);
      expect(room.pendingUpload, isNull);
      expect(source.existsSync(), isTrue);
      await until(
        () =>
            room.messages.any((m) => (m['payload'] as Map?)?['kind'] == 'file'),
      );
    } finally {
      room.dispose();
    }

    final port = server.port;
    await server.stop();
    expect(server.running, isFalse);
    final socket = await ServerSocket.bind(InternetAddress.anyIPv4, port);
    await socket.close();
    server.dispose();
  });

  test('端口被占用时启动失败并给出提示', () async {
    if (binary == null) {
      markTestSkipped('需要 Go 工具链');
      return;
    }
    final busy = await ServerSocket.bind(InternetAddress.anyIPv4, 0);
    final server = create('busy')..port = busy.port;
    try {
      expect(await server.start(), isFalse);
      expect(server.running, isFalse);
      expect(server.error, contains('端口 ${busy.port}'));
    } finally {
      server.dispose();
      await busy.close();
    }
  });

  test('端口上已有其他 LanRoom 时不会误认为启动成功', () async {
    if (binary == null) {
      markTestSkipped('需要 Go 工具链');
      return;
    }
    final other = await HttpServer.bind(InternetAddress.anyIPv4, 0);
    other.listen((request) {
      request.response
        ..write(
          '{"serviceId":"a1234567-1234-1234-1234-123456789abc",'
          '"pinRequired":false,"port":${other.port},"urls":[]}',
        )
        ..close();
    });
    final server = create('other')..port = other.port;
    try {
      expect(await server.start(), isFalse);
      expect(server.running, isFalse);
      expect(server.error, contains('端口 ${other.port}'));
    } finally {
      server.dispose();
      await other.close(force: true);
    }
  });

  test('父进程关闭 stdin 后服务端自行退出', () async {
    if (binary == null) {
      markTestSkipped('需要 Go 工具链');
      return;
    }
    final process = await Process.start(
      binary!,
      ['-port', '${await freePort()}'],
      environment: {
        'LANROOM_DATA_DIR': '${temp.path}/eof',
        'LANROOM_DISCOVERY': 'false',
        'LANROOM_EXIT_ON_STDIN_EOF': '1',
      },
    );
    process.stdout.drain<void>();
    process.stderr.drain<void>();
    await Future<void>.delayed(const Duration(milliseconds: 300));
    await process.stdin.close();
    final code = await process.exitCode.timeout(
      const Duration(seconds: 5),
      onTimeout: () {
        process.kill();
        return -1;
      },
    );
    expect(code, isNot(-1));
  });

  test('找不到可执行文件时不可用', () {
    final server = LocalServer(
      DesktopBridge(root: '${temp.path}/none'),
      executable: null,
    );
    expect(server.available, LocalServer.bundled() != null);
    server.dispose();
  });
}
