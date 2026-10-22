import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:lanroom_mobile/client.dart';

void main() {
  test('认证 cookie 同时用于 HTTP 与 WebSocket，拒绝未授权和重定向', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final subscription = server.listen((request) async {
      final authenticated = request.cookies.any(
        (c) => c.name == 'lanroom_session' && c.value == 'test',
      );
      if (request.uri.path == '/api/auth') {
        request.response.cookies.add(Cookie('lanroom_session', 'test'));
        request.response.write('{"ok":true}');
      } else if (request.uri.path == '/ws') {
        if (!authenticated) {
          request.response.statusCode = 401;
          await request.response.close();
          return;
        }
        final socket = await WebSocketTransformer.upgrade(request);
        socket.listen((data) {
          socket.add(data);
        });
        return;
      } else if (request.uri.path == '/redirect') {
        request.response.statusCode = 302;
        request.response.headers.set('location', '/api/info');
      } else if (!authenticated) {
        request.response.statusCode = 401;
        request.response.write('pin required');
      } else {
        request.response.write('{"authorized":true}');
      }
      await request.response.close();
    });
    final client = LanClient(Uri.parse('http://127.0.0.1:${server.port}'));
    try {
      await expectLater(
        client.info(),
        throwsA(isA<ApiException>().having((e) => e.status, 'status', 401)),
      );
      await client.login('pin');
      expect(client.cookie, 'lanroom_session=test');
      expect((await client.info())['authorized'], true);
      final ws = await client.connect('test', 'a' * 64);
      client.send({'kind': 'text', 'content': 'hello'}, []);
      final message = jsonDecode(await ws.first as String) as Map;
      expect((message['payload'] as Map)['content'], 'hello');
      await expectLater(
        client.request('GET', '/redirect'),
        throwsA(isA<ApiException>().having((e) => e.status, 'status', 302)),
      );
    } finally {
      client.close();
      await subscription.cancel();
      await server.close(force: true);
    }
  });
}
