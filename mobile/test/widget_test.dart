import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lanroom_mobile/main.dart';
import 'package:lanroom_mobile/room.dart';
import 'package:lanroom_mobile/discovery.dart';

void main() {
  test('发现回复必须匹配请求、版本及服务标识', () {
    final data = {
      'type': 'lanroom.offer',
      'version': 1,
      'requestId': 'nonce',
      'serviceId': 'a1234567-1234-1234-1234-123456789abc',
      'port': 8787,
    };
    expect(
      FoundServer.parse(
        utf8.encode(jsonEncode(data)),
        '192.168.1.1',
        'nonce',
      )?.uri.host,
      '192.168.1.1',
    );
    expect(
      FoundServer.parse(utf8.encode(jsonEncode(data)), '192.168.1.1', 'other'),
      isNull,
    );
    data['port'] = 0;
    expect(
      FoundServer.parse(utf8.encode(jsonEncode(data)), '192.168.1.1', 'nonce'),
      isNull,
    );
    expect(FoundServer.parse([0xff], '192.168.1.1', 'nonce'), isNull);
  });
  test('服务地址改变时仍按 UUID 选择，不切换到其他服务器', () {
    final moved = FoundServer('original', Uri.parse('http://192.168.2.5:8787'));
    final other = FoundServer('other', Uri.parse('http://192.168.2.8:8787'));
    expect(preferredServer([other, moved], 'original'), same(moved));
    expect(preferredServer([other], 'original'), isNull);
    expect(preferredServer([other, moved], null), isNull);
    expect(preferredServer([moved], null), same(moved));
  });
  testWidgets('未连接时展示发现状态，禁止发送', (tester) async {
    final room = Room()..status = '未发现服务';
    await tester.pumpWidget(LanRoomApp(room: room));
    expect(find.text('未发现服务'), findsOneWidget);
    expect(find.text('手动输入地址'), findsOneWidget);
    final input = tester.widget<TextField>(
      find.byKey(const Key('messageInput')),
    );
    expect(input.enabled, isFalse);
    await tester.pumpWidget(const SizedBox());
    room.dispose();
  });
  testWidgets('在线设备可切换私信目标并显示文件', (tester) async {
    final room = Room()
      ..connected = true
      ..ownID = 'self'
      ..devices = [
        {'id': 'self', 'name': '手机', 'platform': 'android'},
        {'id': 'peer', 'name': '电脑', 'platform': 'linux'},
      ]
      ..messages = [
        {
          'type': 'message',
          'from': {'id': 'peer', 'name': '电脑'},
          'payload': {
            'kind': 'file',
            'fileId': 'abc',
            'meta': {'name': 'example.txt'},
          },
        },
      ];
    await tester.pumpWidget(LanRoomApp(room: room));
    await tester.pumpAndSettle();
    await tester.tap(find.byType(ChoiceChip).last);
    await tester.pump();
    expect(room.recipients, ['peer']);
    expect(find.text('发送私信'), findsOneWidget);
    expect(find.text('example.txt'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    room.dispose();
  });
}
