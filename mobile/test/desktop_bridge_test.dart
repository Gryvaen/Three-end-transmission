import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:lanroom_mobile/desktop_bridge.dart';
import 'package:lanroom_mobile/windows_ffi.dart' as win;

Uint8List reverse(Uint8List data) => Uint8List.fromList(data.reversed.toList());

void main() {
  late Directory temp;
  setUp(() async {
    temp = await Directory.systemTemp.createTemp('lanroom-bridge-');
  });
  tearDown(() => temp.delete(recursive: true));

  test('凭据加密落盘，重启后可读，不可解密的值视为缺失', () async {
    final bridge = DesktopBridge(
      root: temp.path,
      encrypt: reverse,
      decrypt: reverse,
    );
    expect(await bridge.read('cookie'), isNull);
    await Future.wait([
      bridge.write('cookie', 'lanroom_session=secret'),
      bridge.write('nickname', '书房电脑'),
    ]);
    final raw = await File('${temp.path}/secrets.json').readAsString();
    expect(raw, isNot(contains('secret')));

    final again = DesktopBridge(
      root: temp.path,
      encrypt: reverse,
      decrypt: reverse,
    );
    expect(await again.read('cookie'), 'lanroom_session=secret');
    expect(await again.read('nickname'), '书房电脑');

    final broken = DesktopBridge(
      root: temp.path,
      decrypt: (_) => throw StateError('wrong key'),
    );
    expect(await broken.read('cookie'), isNull);
  });

  test('拖入或选择的文件标记为非自有，按扩展名识别类型', () async {
    final file = File('${temp.path}/照片 50%.JPG')..writeAsBytesSync([1, 2, 3]);
    final info = await describeFile(file.path);
    expect(info['name'], '照片 50%.JPG');
    expect(info['size'], 3);
    expect(info['mime'], 'image/jpeg');
    expect(info['owned'], isFalse);
    expect(mimeFor('README'), 'application/octet-stream');
  });

  test('解析 ip addr 输出，跳过 TUN 和容器网卡', () {
    const output = '''
1: lo    inet 127.0.0.1/8 scope host lo\\       valid_lft forever preferred_lft forever
2: wlan0    inet 192.168.1.5/24 brd 192.168.1.255 scope global dynamic wlan0\\       valid_lft 1000sec
3: Meta    inet 198.18.0.1/30 brd 198.18.0.3 scope global Meta\\       valid_lft forever
4: docker0    inet 172.17.0.1/16 brd 172.17.255.255 scope global docker0
5: enp3s0    inet 10.0.0.8/16 brd 10.0.255.255 scope global enp3s0
''';
    expect(parseIpAddr(output), [
      {'name': 'wlan0', 'ip': '192.168.1.5', 'broadcast': '192.168.1.255'},
      {'name': 'enp3s0', 'ip': '10.0.0.8', 'broadcast': '10.0.255.255'},
    ]);
  });

  test('按前缀计算子网广播地址', () {
    expect(win.broadcastAddress('192.168.1.5', 24), '192.168.1.255');
    expect(win.broadcastAddress('10.0.3.9', 16), '10.0.255.255');
    expect(win.broadcastAddress('172.20.5.1', 22), '172.20.7.255');
    expect(win.broadcastAddress('192.168.1.5', 31), isNull);
    expect(win.broadcastAddress('169.254.3.4', 16), isNull);
  });

  test('Windows DPAPI 往返与网卡枚举', () async {
    final secret = Uint8List.fromList('口令 pin'.codeUnits);
    final sealed = win.protect(secret);
    expect(sealed, isNot(secret));
    expect(win.unprotect(sealed), secret);
    final list = win.adapters();
    for (final entry in list) {
      expect(InternetAddress.tryParse(entry['ip']!), isNotNull);
      expect(InternetAddress.tryParse(entry['broadcast']!), isNotNull);
    }
    win.keepAwake(true);
    win.keepAwake(false);
  }, skip: !Platform.isWindows);

  test('真实接口列表可用于发现', () async {
    final list = await DesktopBridge(root: temp.path).interfaces;
    for (final entry in list) {
      expect(entry['ip'], isNot('127.0.0.1'));
      expect(entry['broadcast'], isNotNull);
    }
  });
}
