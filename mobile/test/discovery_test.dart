import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:lanroom_mobile/discovery.dart';

void main() {
  test('UDP 发现接收匹配回复并去重，忽略错误请求标识', () async {
    final responder = await RawDatagramSocket.bind(
      InternetAddress.loopbackIPv4,
      0,
    );
    final subscription = responder.listen((event) {
      if (event != RawSocketEvent.read) {
        return;
      }
      final packet = responder.receive();
      if (packet == null) {
        return;
      }
      final request = jsonDecode(utf8.decode(packet.data)) as Map;
      final offer = {
        'type': 'lanroom.offer',
        'version': 1,
        'requestId': request['requestId'],
        'serviceId': 'a1234567-1234-1234-1234-123456789abc',
        'port': 8787,
      };
      for (var i = 0; i < 2; i++) {
        responder.send(
          utf8.encode(jsonEncode(offer)),
          packet.address,
          packet.port,
        );
      }
      offer['requestId'] = 'wrong-request';
      responder.send(
        utf8.encode(jsonEncode(offer)),
        packet.address,
        packet.port,
      );
    });
    try {
      final result = await discover(
        [
          {'ip': '127.0.0.1', 'broadcast': '127.0.0.1'},
        ],
        port: responder.port,
        duration: const Duration(milliseconds: 150),
      );
      expect(result, hasLength(1));
      expect(result.single.uri.toString(), 'http://127.0.0.1:8787');
    } finally {
      await subscription.cancel();
      responder.close();
    }
  });
}
