import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:lanroom_mobile/client.dart';
import 'package:lanroom_mobile/discovery.dart';
import 'package:lanroom_mobile/main.dart';
import 'package:lanroom_mobile/room.dart';

Future<void> waitFor(bool Function() condition, String reason) async {
  for (var i = 0; i < 120; i++) {
    if (condition()) {
      return;
    }
    await Future<void>.delayed(const Duration(milliseconds: 100));
  }
  throw StateError(reason);
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  testWidgets('真实局域网自动发现、认证、消息、文件、续传与重连', (tester) async {
    final room = Room();
    await tester.pumpWidget(LanRoomApp(room: room));
    await tester.runAsync(() async {
      await room.initialize();
      expect(room.selected, isNotNull, reason: '${room.status}: ${room.error}');
      expect(room.selected!.uri.port, 18787);
      expect(room.selected!.uri.host, isNot('127.0.0.1'));
      // Ensure the wrong-PIN path is exercised even on repeat runs.
      if (room.connected) {
        await room.bridge.write('cookie:${room.selected!.id}', '');
        await room.attach(room.selected!);
      }
      expect(room.needsPin, isTrue);
      await room.login('incorrect-pin');
      expect(room.needsPin, isTrue);
      expect(room.connected, isFalse);
      await room.login('mobile-test-2026');
      expect(room.connected, isTrue, reason: room.error);
      await waitFor(() => room.ownID != null, 'no welcome');
      debugPrint(
        'LANROOM_TEST discovery=${room.selected!.uri} uuid=${room.selected!.id} auth=PASS',
      );

      final peer = LanClient(room.selected!.uri);
      await peer.login('mobile-test-2026');
      final ws = await peer.connect('协议测试电脑', randomID(32));
      final incoming = <Map<String, dynamic>>[];
      final sub = ws.listen((raw) {
        incoming.add(
          Map<String, dynamic>.from(jsonDecode(raw as String) as Map),
        );
      });
      try {
        await waitFor(() => room.devices.length >= 2, 'presence failed');
        room.text('phone-to-peer');
        await waitFor(
          () => incoming.any(
            (m) => (m['payload'] as Map?)?['content'] == 'phone-to-peer',
          ),
          'outbound text failed',
        );
        peer.send({'kind': 'text', 'content': 'peer-to-phone'}, []);
        await waitFor(
          () => room.messages.any(
            (m) => (m['payload'] as Map?)?['content'] == 'peer-to-phone',
          ),
          'inbound text failed',
        );
        final peerID =
            (incoming.firstWhere((m) => m['type'] == 'welcome')['device']
                    as Map)['id']
                as String;
        room.recipients = [peerID];
        room.text('private-from-phone');
        await waitFor(
          () => incoming.any(
            (m) =>
                (m['payload'] as Map?)?['content'] == 'private-from-phone' &&
                (m['to'] as List).contains(peerID),
          ),
          'private failed',
        );
        debugPrint('LANROOM_TEST presence/text/private=PASS');

        final directory = await room.bridge.directory;
        final bytes = Uint8List.fromList(
          List.generate(2500123, (i) => (i * 31 + 7) % 256),
        );
        final source = File('$directory/test-source.bin');
        await source.writeAsBytes(bytes, flush: true);
        final created = await peer.json(
          'POST',
          '/api/uploads',
          body: {
            'name': 'phone-fixture.bin',
            'size': bytes.length,
            'mime': 'application/octet-stream',
          },
        );
        // Seed an interrupted upload and verify the phone queries the offset.
        final partial = await peer.json(
          'PUT',
          '/api/uploads/${created['uploadId']}?offset=0',
          bytes: bytes.sublist(0, 333333),
        );
        expect(partial['offset'], 333333);
        room.pendingUpload = {
          'path': source.path,
          'name': 'phone-fixture.bin',
          'size': bytes.length,
          'mime': 'application/octet-stream',
          'serviceId': room.selected!.id,
          'uploadId': created['uploadId'],
          'to': [peerID],
        };
        await room.bridge.write('upload', jsonEncode(room.pendingUpload));
        await room.resumeUpload();
        expect(room.pendingUpload, isNull, reason: room.error);
        await waitFor(
          () => incoming.any((m) => (m['payload'] as Map?)?['kind'] == 'file'),
          'file message failed',
        );
        final fileMessage = incoming.lastWhere(
          (m) => (m['payload'] as Map?)?['kind'] == 'file',
        );
        final response = await peer.request(
          'GET',
          '/api/files/${(fileMessage['payload'] as Map)['fileId']}',
        );
        final received = await response.fold<List<int>>(
          [],
          (a, b) => a..addAll(b),
        );
        expect(received, orderedEquals(bytes));
        final downloaded = await room.download(
          Map<String, dynamic>.from(fileMessage['payload'] as Map),
          export: false,
        );
        expect(downloaded, isNotNull);
        expect(await File(downloaded!).readAsBytes(), orderedEquals(bytes));
        // Preserve one deterministic fixture for independent host SHA-256 verification.
        await File(downloaded).copy('$directory/test-received.bin');
        await File(downloaded).delete();
        debugPrint(
          'LANROOM_TEST upload-resume/download=PASS fileId=${(fileMessage['payload'] as Map)['fileId']} size=${bytes.length}',
        );

        final oldSocket = room.client!.socket!;
        await oldSocket.close();
        await waitFor(
          () => room.connected && room.client?.socket != oldSocket,
          'automatic reconnect failed',
        );
        expect(room.connected, isTrue, reason: room.error);
        await waitFor(
          () => room.messages.any(
            (m) => (m['payload'] as Map?)?['content'] == 'phone-to-peer',
          ),
          'history failed',
        );
        final identity = room.key;
        await room.bridge.write('test-complete', 'ok');
        expect(await room.bridge.read('deviceKey'), identity);
        debugPrint('LANROOM_TEST reconnect/history/secure-storage=PASS');
      } finally {
        await sub.cancel();
        peer.close();
      }
    });
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('messageInput')), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    room.dispose();
  }, timeout: const Timeout(Duration(minutes: 3)));
  testWidgets('上传任务跨客户端重建恢复及取消清理', (tester) async {
    await tester.runAsync(() async {
      final initial = Room();
      await initial.initialize();
      expect(initial.connected, isTrue, reason: initial.error);
      final server = initial.selected!;
      final directory = await initial.bridge.directory;
      final source = File('$directory/pending-restart.bin');
      final bytes = Uint8List.fromList(List.generate(131123, (i) => i % 251));
      await source.writeAsBytes(bytes, flush: true);
      final state = await initial.client!.json(
        'POST',
        '/api/uploads',
        body: {
          'name': 'restart.bin',
          'size': bytes.length,
          'mime': 'application/octet-stream',
        },
      );
      await initial.client!.json(
        'PUT',
        '/api/uploads/${state['uploadId']}?offset=0',
        bytes: bytes.sublist(0, 17000),
      );
      await initial.bridge.write(
        'upload',
        jsonEncode({
          'path': source.path,
          'name': 'restart.bin',
          'size': bytes.length,
          'mime': 'application/octet-stream',
          'serviceId': server.id,
          'uploadId': state['uploadId'],
          'to': <String>[],
        }),
      );
      final identity = initial.key;
      initial.dispose();

      final restored = Room();
      try {
        await restored.initialize();
        expect(restored.connected, isTrue, reason: restored.error);
        expect(restored.key, identity);
        expect(restored.selected!.id, server.id);
        expect(restored.pendingUpload?['uploadId'], state['uploadId']);
        expect(await source.exists(), isTrue);
        await restored.resumeUpload();
        expect(restored.pendingUpload, isNull, reason: restored.error);
        final complete = await restored.client!.json(
          'GET',
          '/api/uploads/${state['uploadId']}',
        );
        expect(complete['offset'], bytes.length);
        expect(await source.exists(), isFalse);

        final cancelled = File('$directory/pending-cancel.bin');
        await cancelled.writeAsBytes([1, 2, 3, 4]);
        final cancelSession = await restored.client!.json(
          'POST',
          '/api/uploads',
          body: {
            'name': 'cancel.bin',
            'size': 4,
            'mime': 'application/octet-stream',
          },
        );
        restored.pendingUpload = {
          'path': cancelled.path,
          'name': 'cancel.bin',
          'size': 4,
          'mime': 'application/octet-stream',
          'serviceId': server.id,
          'uploadId': cancelSession['uploadId'],
          'to': <String>[],
        };
        await restored.bridge.write(
          'upload',
          jsonEncode(restored.pendingUpload),
        );
        await restored.cancelUpload();
        expect(restored.pendingUpload, isNull);
        expect(await cancelled.exists(), isFalse);
        expect(await restored.bridge.read('upload'), '');
        await expectLater(
          restored.client!.json(
            'GET',
            '/api/uploads/${cancelSession['uploadId']}',
          ),
          throwsA(isA<ApiException>().having((e) => e.status, 'status', 404)),
        );
        debugPrint('LANROOM_TEST persisted-upload/restore/cancel-cleanup=PASS');
      } finally {
        restored.dispose();
      }
    });
  }, timeout: const Timeout(Duration(minutes: 2)));
}
