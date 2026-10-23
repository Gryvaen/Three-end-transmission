import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:lanroom_mobile/discovery.dart';
import 'package:lanroom_mobile/platform.dart';
import 'package:lanroom_mobile/room.dart';

const serviceID = 'a1234567-1234-1234-1234-123456789abc';

class FakeBridge extends AndroidBridge {
  final store = <String, String>{};
  final service = <bool>[];
  int scans = 0;
  @override
  Future<String?> read(String key) async => store[key];
  @override
  Future<void> write(String key, String value) async => store[key] = value;
  @override
  Future<List<Map<String, dynamic>>> get interfaces async {
    scans++;
    return [];
  }

  @override
  Future<void> keepAlive(bool on, [String? label]) async => service.add(on);
  @override
  Future<void> transferring(bool on) async {}
}

class FakeServer {
  late final HttpServer http;
  final sockets = <WebSocket>[];
  int infoRequests = 0;
  Uri get uri => Uri.parse('http://127.0.0.1:${http.port}');
  Future<void> start() async {
    http = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    http.listen((request) async {
      if (request.uri.path == '/ws') {
        sockets.add(await WebSocketTransformer.upgrade(request));
        return;
      }
      infoRequests++;
      request.response.write(
        jsonEncode({
          'serviceId': serviceID,
          'discoveryVersion': 1,
          'pinRequired': false,
          'authorized': true,
          'port': http.port,
        }),
      );
      await request.response.close();
    });
  }
}

Future<void> until(bool Function() condition) async {
  final deadline = DateTime.now().add(const Duration(seconds: 5));
  while (!condition()) {
    if (DateTime.now().isAfter(deadline)) {
      fail('等待超时');
    }
    await Future<void>.delayed(const Duration(milliseconds: 20));
  }
}

void main() {
  late FakeServer server;
  late FakeBridge bridge;
  late Room room;
  setUp(() async {
    server = FakeServer();
    await server.start();
    bridge = FakeBridge();
    room = Room(bridge: bridge);
    await room.attach(FoundServer(serviceID, server.uri));
  });
  tearDown(() async {
    room.dispose();
    await server.http.close(force: true);
  });

  test('连接后启动前台服务，回到前台时不再请求服务器', () async {
    expect(room.connected, isTrue);
    expect(bridge.service, [true]);
    final before = server.infoRequests;
    room.foreground(false);
    room.foreground(true);
    await Future<void>.delayed(const Duration(milliseconds: 100));
    expect(server.infoRequests, before);
    expect(room.connected, isTrue);
    expect(server.sockets.length, 1);
  });

  test('断线后直接连回原服务器，不重新搜索，保留消息', () async {
    room.messages = [
      {'type': 'message', 'id': 'old'},
    ];
    await server.sockets.single.close();
    await until(() => server.sockets.length == 2 && room.connected);
    expect(bridge.scans, 0);
    expect(room.messages.single['id'], 'old');
    expect(room.status, '已连接');
  });

  test('直连失败时才回退到局域网搜索', () async {
    final socket = server.sockets.single;
    await server.http.close(force: true);
    await socket.close();
    await until(() => room.status == '自动发现失败');
    expect(room.connected, isFalse);
  });
}
