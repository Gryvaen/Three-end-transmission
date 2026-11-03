import 'dart:async';
import 'dart:io';

import 'package:desktop_drop/desktop_drop.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'desktop_bridge.dart';
import 'local_server.dart';
import 'room.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const LanRoomApp());
}

class LanRoomApp extends StatelessWidget {
  final Room? room;
  final LocalServer? server;
  const LanRoomApp({super.key, this.room, this.server});
  @override
  Widget build(BuildContext context) => MaterialApp(
    title: 'LanRoom',
    debugShowCheckedModeBanner: false,
    theme: ThemeData(
      useMaterial3: true,
      colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xff2458be)),
      scaffoldBackgroundColor: const Color(0xfff4f7fc),
      appBarTheme: const AppBarTheme(
        backgroundColor: Color(0xfff4f7fc),
        centerTitle: false,
      ),
      inputDecorationTheme: const InputDecorationTheme(
        border: OutlineInputBorder(),
        filled: true,
        fillColor: Colors.white,
      ),
    ),
    home: RoomScreen(room: room, server: server),
  );
}

class RoomScreen extends StatefulWidget {
  final Room? room;
  final LocalServer? server;
  const RoomScreen({super.key, this.room, this.server});
  @override
  State<RoomScreen> createState() => _RoomScreenState();
}

class _RoomScreenState extends State<RoomScreen> with WidgetsBindingObserver {
  static const _wide = 900.0;
  late final Room room;
  LocalServer? server;
  final text = TextEditingController(), pin = TextEditingController();
  final scroll = ScrollController();
  int messageCount = 0;
  bool dragging = false;
  bool get desktop => room.bridge.platform != 'android';
  // Drag and drop needs the native plugin, which only the real app has.
  bool get dropEnabled => desktop && widget.room == null;

  @override
  void initState() {
    super.initState();
    room = widget.room ?? Room();
    room.addListener(update);
    server = widget.server;
    if (server == null && widget.room == null && desktop) {
      final local = LocalServer(room.bridge);
      if (local.available) {
        server = local;
      }
    }
    server?.addListener(update);
    WidgetsBinding.instance.addObserver(this);
    if (widget.room == null) {
      unawaited(_boot());
    }
  }

  Future<void> _boot() async {
    final local = widget.server == null ? server : null;
    if (local != null) {
      await local.load();
      if (local.autoStart) {
        unawaited(toggleServer(true));
      }
    }
    await room.initialize();
  }

  void update() {
    if (!mounted) {
      return;
    }
    setState(() {});
    if (room.messages.length != messageCount) {
      messageCount = room.messages.length;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (scroll.hasClients) {
          scroll.animateTo(
            scroll.position.maxScrollExtent,
            duration: const Duration(milliseconds: 200),
            curve: Curves.easeOut,
          );
        }
      });
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    // Desktop processes keep running when minimized or unfocused.
    if (!desktop) {
      room.foreground(state == AppLifecycleState.resumed);
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    room.removeListener(update);
    server?.removeListener(update);
    if (widget.server == null) {
      server?.dispose();
    }
    if (widget.room == null) {
      room.dispose();
    }
    text.dispose();
    pin.dispose();
    scroll.dispose();
    super.dispose();
  }

  Future<void> edit({bool name = false}) async {
    final input = TextEditingController(text: name ? room.nickname : '');
    final value = await showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(name ? '设备昵称' : '手动连接'),
        content: TextField(
          controller: input,
          autofocus: true,
          decoration: InputDecoration(
            labelText: name ? '昵称' : '服务器地址',
            hintText: name ? null : 'http://192.168.1.10:8787',
          ),
          onSubmitted: (value) => Navigator.pop(context, value),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, input.text),
            child: const Text('确定'),
          ),
        ],
      ),
    );
    // Dialog exit animations may still reference the controller; dispose next frame.
    Future<void>.delayed(const Duration(seconds: 1), input.dispose);
    if (value != null && value.trim().isNotEmpty) {
      if (name) {
        await room.rename(value);
      } else {
        await room.manual(value);
      }
    }
  }

  void send() {
    if (text.text.trim().isEmpty) {
      return;
    }
    room.text(text.text);
    text.clear();
  }

  Future<void> toggleServer(bool on) async {
    final local = server;
    if (local == null) {
      return;
    }
    if (!on) {
      await local.stop();
      return;
    }
    if (!await local.start()) {
      return;
    }
    // Startup discovery may still be running; join once the room is idle.
    while (room.busy && mounted) {
      await Future<void>.delayed(const Duration(milliseconds: 200));
    }
    final id = local.info?['serviceId'];
    if (mounted && !(room.connected && room.selected?.id == id)) {
      await room.manual(local.localUri.toString());
    }
  }

  Future<void> serverSettings() async {
    final local = server!;
    final port = TextEditingController(text: '${local.port}');
    final secret = TextEditingController(text: local.pin);
    var auto = local.autoStart;
    final saved = await showDialog<bool>(
      context: context,
      builder: (context) => StatefulBuilder(
        builder: (context, setDialog) => AlertDialog(
          title: const Text('本机服务器设置'),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: port,
                keyboardType: TextInputType.number,
                inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                decoration: const InputDecoration(labelText: 'HTTP 端口'),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: secret,
                decoration: const InputDecoration(labelText: '房间口令（留空则不需要）'),
              ),
              const SizedBox(height: 8),
              CheckboxListTile(
                contentPadding: EdgeInsets.zero,
                value: auto,
                onChanged: (value) => setDialog(() => auto = value ?? false),
                title: const Text('打开 App 时自动启动'),
              ),
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('保存'),
            ),
          ],
        ),
      ),
    );
    final number = int.tryParse(port.text);
    final pinText = secret.text.trim();
    Future<void>.delayed(const Duration(seconds: 1), () {
      port.dispose();
      secret.dispose();
    });
    if (saved == true) {
      await local.configure(
        port: number != null && number > 0 && number < 65536 ? number : null,
        pin: pinText,
        autoStart: auto,
      );
    }
  }

  void showLogs() {
    showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('服务器日志'),
        content: SizedBox(
          width: 640,
          height: 360,
          child: SingleChildScrollView(
            reverse: true,
            child: SelectableText(
              server!.logs.isEmpty ? '暂无日志' : server!.logs.join('\n'),
              style: const TextStyle(fontFamily: 'monospace', fontSize: 12),
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('关闭'),
          ),
        ],
      ),
    );
  }

  void help() {
    showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('连接排查'),
        content: Text(
          desktop
              ? '设备需处于同一局域网。\n\n可在左侧开启“本机服务器”，让这台电脑直接当服务器；首次启动时 Windows 防火墙会弹窗，请勾选“专用网络”并允许。也可以连接其他电脑上运行的 LanRoom 服务。\n\n需放行 UDP 8788 和 HTTP 服务端口（默认 8787）。Mihomo TUN 可能影响路由，请检查局域网绕过配置。应用不会修改代理或防火墙。\n\n断线时会直接重连原服务器，地址变化时才重新搜索。'
              : '手机和电脑需处于同一局域网。\n\n电脑需运行新版 Go 服务端，并允许 UDP 8788 和 HTTP 服务端口。\n\nMihomo TUN 可能影响路由，请检查局域网绕过配置。应用不会修改代理或防火墙。\n\n连接后通知栏会显示“LanRoom 已连接”，切到后台、锁屏或选择文件时保持在线；划掉应用即断开。断线时会直接重连原服务器，地址变化时才重新搜索。',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('知道了'),
          ),
        ],
      ),
    );
  }

  Future<void> dropped(DropDoneDetails details) async {
    setState(() => dragging = false);
    final paths = details.files
        .map((f) => f.path)
        .where((p) => p.isNotEmpty && FileSystemEntity.isFileSync(p))
        .toList();
    if (paths.isEmpty) {
      room.fail('只能拖入文件，文件夹请先压缩');
      return;
    }
    if (paths.length > 1) {
      room.fail('一次只能发送一个文件');
      return;
    }
    await room.sendFile(await describeFile(paths.single));
  }

  @override
  Widget build(BuildContext context) {
    final body = SafeArea(
      child: LayoutBuilder(
        builder: (context, constraints) =>
            constraints.maxWidth >= _wide ? wideLayout() : narrowLayout(),
      ),
    );
    return Scaffold(
      appBar: AppBar(
        title: const Text(
          'LanRoom',
          style: TextStyle(fontWeight: FontWeight.w800, letterSpacing: -0.8),
        ),
        actions: [
          IconButton(
            tooltip: '修改昵称',
            onPressed: () => edit(name: true),
            icon: const Icon(Icons.badge_outlined),
          ),
          PopupMenuButton<String>(
            onSelected: (value) {
              if (value == 'manual') {
                edit();
              }
              if (value == 'search') {
                room.search(chooseAnother: true);
              }
              if (value == 'help') {
                help();
              }
            },
            itemBuilder: (_) => const [
              PopupMenuItem(value: 'manual', child: Text('手动连接')),
              PopupMenuItem(value: 'search', child: Text('切换服务器')),
              PopupMenuItem(value: 'help', child: Text('连接排查')),
            ],
          ),
        ],
      ),
      body: dropEnabled
          ? DropTarget(
              onDragEntered: (_) => setState(() => dragging = true),
              onDragExited: (_) => setState(() => dragging = false),
              onDragDone: dropped,
              child: Stack(
                children: [
                  body,
                  if (dragging)
                    Positioned.fill(
                      child: IgnorePointer(
                        child: Container(
                          margin: const EdgeInsets.all(12),
                          decoration: BoxDecoration(
                            color: const Color(0x222458be),
                            border: Border.all(
                              color: const Color(0xff2458be),
                              width: 2,
                            ),
                            borderRadius: BorderRadius.circular(18),
                          ),
                          alignment: Alignment.center,
                          child: Text(
                            room.connected
                                ? (room.recipients.isEmpty
                                      ? '松开发送给所有设备'
                                      : '松开发送私信')
                                : '请先连接服务器',
                            style: const TextStyle(
                              fontSize: 20,
                              fontWeight: FontWeight.w600,
                              color: Color(0xff183e86),
                            ),
                          ),
                        ),
                      ),
                    ),
                ],
              ),
            )
          : body,
    );
  }

  Widget narrowLayout() => Column(
    children: [
      statusCard(),
      if (room.error != null) errorBar(),
      if (room.needsPin) pinRow(),
      if (server != null && !room.connected) serverPanel(),
      if (!room.connected && !room.needsPin && room.servers.isNotEmpty)
        Flexible(child: serverList()),
      if (room.connected) ...[
        deviceChips(),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 20),
          child: Row(
            children: [
              Text(
                '${room.devices.length} 台在线设备',
                style: const TextStyle(color: Colors.blueGrey, fontSize: 12),
              ),
              const Spacer(),
              Text(
                room.nickname,
                style: const TextStyle(color: Colors.blueGrey, fontSize: 12),
              ),
            ],
          ),
        ),
        Expanded(child: messageList()),
      ] else if (room.servers.isEmpty || room.needsPin)
        Expanded(child: emptyState()),
      if (room.transferStatus != null) transferBar(),
      composer(),
    ],
  );

  Widget wideLayout() => Row(
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      SizedBox(
        width: 320,
        child: ListView(
          padding: const EdgeInsets.only(bottom: 16),
          children: [
            statusCard(),
            if (room.error != null) errorBar(),
            if (room.needsPin) pinRow(),
            if (server != null) serverPanel(),
            if (room.connected) deviceList(),
            if (!room.connected && !room.needsPin && room.servers.isNotEmpty)
              serverList(),
          ],
        ),
      ),
      const VerticalDivider(width: 1),
      Expanded(
        child: Column(
          children: [
            if (room.connected)
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 0),
                child: Row(
                  children: [
                    Text(
                      room.recipients.isEmpty
                          ? '发送给所有设备'
                          : '私信 · ${recipientName()}',
                      style: const TextStyle(
                        fontWeight: FontWeight.w600,
                        fontSize: 15,
                      ),
                    ),
                    const Spacer(),
                    Text(
                      room.nickname,
                      style: const TextStyle(
                        color: Colors.blueGrey,
                        fontSize: 12,
                      ),
                    ),
                  ],
                ),
              ),
            Expanded(child: room.connected ? messageList() : emptyState()),
            if (room.transferStatus != null) transferBar(),
            composer(),
          ],
        ),
      ),
    ],
  );

  String recipientName() {
    final id = room.recipients.isEmpty ? null : room.recipients.first;
    for (final device in room.devices) {
      if (device['id'] == id) {
        return device['name'] as String? ?? '设备';
      }
    }
    return '已离线';
  }

  Widget statusCard() => Container(
    margin: const EdgeInsets.fromLTRB(16, 4, 16, 12),
    padding: const EdgeInsets.all(16),
    decoration: BoxDecoration(
      color: const Color(0xff183e86),
      borderRadius: BorderRadius.circular(18),
    ),
    child: Row(
      children: [
        Icon(
          room.connected ? Icons.lan_outlined : Icons.radar,
          color: const Color(0xffa9d7ff),
          size: 30,
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                room.status,
                style: const TextStyle(
                  color: Colors.white,
                  fontWeight: FontWeight.w700,
                  fontSize: 16,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                room.selected?.uri.toString() ?? '打开即查找，地址变化也能重新连接',
                style: const TextStyle(color: Color(0xffc4daf8), fontSize: 12),
              ),
            ],
          ),
        ),
        if (room.busy)
          const SizedBox(
            width: 22,
            height: 22,
            child: CircularProgressIndicator(
              strokeWidth: 2,
              color: Colors.white,
            ),
          )
        else
          IconButton(
            tooltip: '重新发现',
            onPressed: () => room.search(),
            icon: const Icon(Icons.refresh, color: Colors.white),
          ),
      ],
    ),
  );

  Widget errorBar() => Padding(
    padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Icon(Icons.info_outline, color: Color(0xff9b4715), size: 18),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            room.error!,
            style: const TextStyle(color: Color(0xff9b4715), fontSize: 12),
          ),
        ),
        IconButton(
          tooltip: '关闭提示',
          visualDensity: VisualDensity.compact,
          onPressed: () {
            room.error = null;
            room.changed();
          },
          icon: const Icon(Icons.close, size: 18),
        ),
      ],
    ),
  );

  Widget pinRow() => Padding(
    padding: const EdgeInsets.all(16),
    child: Row(
      children: [
        Expanded(
          child: TextField(
            key: const Key('pin'),
            controller: pin,
            obscureText: true,
            decoration: const InputDecoration(labelText: '房间口令'),
            onSubmitted: (_) => room.login(pin.text),
          ),
        ),
        const SizedBox(width: 8),
        FilledButton(
          onPressed: room.busy ? null : () => room.login(pin.text),
          child: const Text('加入'),
        ),
      ],
    ),
  );

  Widget serverList() => ListView(
    shrinkWrap: true,
    physics: desktop ? const NeverScrollableScrollPhysics() : null,
    children: room.servers
        .map(
          (server) => ListTile(
            leading: const Icon(Icons.computer),
            title: Text(server.uri.toString()),
            subtitle: Text('服务 ${server.id.substring(0, 8)}'),
            trailing: const Icon(Icons.chevron_right),
            onTap: room.busy ? null : () => room.attach(server),
          ),
        )
        .toList(),
  );

  IconData deviceIcon(Map<String, dynamic> device) =>
      device['platform'] == 'android' || device['platform'] == 'ios'
      ? Icons.smartphone
      : Icons.computer;

  void target(String? id) {
    room.recipients = id == null ? [] : [id];
    room.changed();
  }

  Widget deviceChips() => SizedBox(
    height: 60,
    child: ListView(
      scrollDirection: Axis.horizontal,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      children: [
        Padding(
          padding: const EdgeInsets.only(right: 8),
          child: ChoiceChip(
            label: const Text('所有设备'),
            selected: room.recipients.isEmpty,
            onSelected: (_) => target(null),
          ),
        ),
        ...room.devices
            .where((d) => d['id'] != room.ownID)
            .map(
              (device) => Padding(
                padding: const EdgeInsets.only(right: 8),
                child: ChoiceChip(
                  avatar: Icon(deviceIcon(device), size: 18),
                  label: Text(device['name'] as String),
                  selected: room.recipients.contains(device['id']),
                  onSelected: (_) => target(device['id'] as String),
                ),
              ),
            ),
      ],
    ),
  );

  Widget deviceList() => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Padding(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 4),
        child: Text(
          '${room.devices.length} 台在线设备',
          style: const TextStyle(color: Colors.blueGrey, fontSize: 12),
        ),
      ),
      ListTile(
        leading: const Icon(Icons.groups_outlined),
        title: const Text('所有设备'),
        selected: room.recipients.isEmpty,
        onTap: () => target(null),
      ),
      ...room.devices
          .where((d) => d['id'] != room.ownID)
          .map(
            (device) => ListTile(
              leading: Icon(deviceIcon(device)),
              title: Text(device['name'] as String),
              subtitle: const Text('点击发送私信'),
              selected: room.recipients.contains(device['id']),
              onTap: () => target(device['id'] as String),
            ),
          ),
    ],
  );

  Widget serverPanel() {
    final local = server!;
    final on = local.running || local.starting;
    return Container(
      margin: const EdgeInsets.fromLTRB(16, 4, 16, 8),
      padding: const EdgeInsets.fromLTRB(16, 8, 8, 12),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(18),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(Icons.dns_outlined, color: Color(0xff183e86)),
              const SizedBox(width: 8),
              const Expanded(
                child: Text(
                  '本机作为服务器',
                  style: TextStyle(fontWeight: FontWeight.w600),
                ),
              ),
              Switch(
                key: const Key('serverSwitch'),
                value: on,
                onChanged: local.starting ? null : toggleServer,
              ),
            ],
          ),
          if (local.starting) const LinearProgressIndicator(),
          if (local.running) ...[
            Text(
              '端口 ${local.port} · ${local.pin.isEmpty ? '无口令' : '需要口令'}',
              style: const TextStyle(color: Colors.blueGrey, fontSize: 12),
            ),
            const SizedBox(height: 8),
            const Text('其他设备扫码或打开：', style: TextStyle(fontSize: 12)),
            ...local.urls.map(
              (url) => Row(
                children: [
                  Expanded(
                    child: SelectableText(
                      url,
                      style: const TextStyle(fontSize: 13),
                    ),
                  ),
                  IconButton(
                    tooltip: '复制',
                    visualDensity: VisualDensity.compact,
                    onPressed: () =>
                        Clipboard.setData(ClipboardData(text: url)),
                    icon: const Icon(Icons.copy, size: 16),
                  ),
                ],
              ),
            ),
            if (local.qr != null)
              Center(
                child: Image.memory(
                  local.qr!,
                  width: 160,
                  height: 160,
                  filterQuality: FilterQuality.none,
                ),
              ),
          ] else if (!local.starting)
            const Text(
              '让手机和其他电脑连到这台电脑，无需另外运行服务端。首次启动时请在防火墙弹窗中允许“专用网络”。',
              style: TextStyle(color: Colors.blueGrey, fontSize: 12),
            ),
          if (local.error != null)
            Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Text(
                local.error!,
                style: const TextStyle(color: Color(0xff9b4715), fontSize: 12),
              ),
            ),
          Wrap(
            children: [
              if (local.running)
                TextButton.icon(
                  onPressed: () => openUrl(
                    local.info?['joinUrl'] as String? ?? '${local.localUri}',
                  ),
                  icon: const Icon(Icons.open_in_browser, size: 18),
                  label: const Text('浏览器打开'),
                ),
              TextButton.icon(
                onPressed: on ? null : serverSettings,
                icon: const Icon(Icons.tune, size: 18),
                label: const Text('设置'),
              ),
              TextButton.icon(
                onPressed: showLogs,
                icon: const Icon(Icons.article_outlined, size: 18),
                label: const Text('日志'),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget messageList() => room.messages.isEmpty
      ? Center(
          child: Text(
            desktop ? '选择设备，发送消息，或把文件拖到窗口里' : '选择设备，发送第一条消息',
            style: const TextStyle(color: Colors.blueGrey),
          ),
        )
      : ListView.builder(
          controller: scroll,
          padding: const EdgeInsets.all(16),
          itemCount: room.messages.length,
          itemBuilder: (_, index) => message(room.messages[index]),
        );

  Widget emptyState() => Center(
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const Icon(Icons.devices_rounded, size: 66, color: Color(0xff7f9dc6)),
        const SizedBox(height: 16),
        const Text(
          '让手机与电脑相连',
          style: TextStyle(fontSize: 20, fontWeight: FontWeight.w600),
        ),
        const SizedBox(height: 8),
        Text(
          server != null
              ? '开启左侧“本机作为服务器”，或连接局域网里的 LanRoom 服务'
              : desktop
              ? '自动寻找局域网里的 LanRoom 服务'
              : '电脑运行 LanRoom，手机自动寻找服务',
          style: const TextStyle(color: Colors.blueGrey),
        ),
        const SizedBox(height: 20),
        OutlinedButton.icon(
          onPressed: () => edit(),
          icon: const Icon(Icons.link),
          label: const Text('手动输入地址'),
        ),
      ],
    ),
  );

  Widget transferBar() => Padding(
    padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
    child: Column(
      children: [
        if (room.uploading || room.downloading)
          LinearProgressIndicator(
            value: room.downloading ? room.downloadProgress : room.progress,
          ),
        Row(
          children: [
            Expanded(
              child: Text(
                '${room.pendingUpload?['name'] ?? ''} ${room.transferStatus}',
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(fontSize: 12),
              ),
            ),
            if (room.pendingUpload != null) ...[
              if (!room.uploading)
                TextButton(
                  onPressed: room.connected ? room.resumeUpload : null,
                  child: const Text('继续'),
                ),
              TextButton(onPressed: room.cancelUpload, child: const Text('取消')),
            ],
          ],
        ),
      ],
    ),
  );

  // Desktop: Enter sends, Shift+Enter inserts a newline.
  KeyEventResult enterToSend(FocusNode _, KeyEvent event) {
    if (event is KeyDownEvent &&
        (event.logicalKey == LogicalKeyboardKey.enter ||
            event.logicalKey == LogicalKeyboardKey.numpadEnter) &&
        !HardwareKeyboard.instance.isShiftPressed) {
      if (room.connected) {
        send();
      }
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  Widget composer() {
    final input = TextField(
      key: const Key('messageInput'),
      controller: text,
      enabled: room.connected,
      minLines: 1,
      maxLines: desktop ? 6 : 4,
      decoration: InputDecoration(
        hintText: room.recipients.isEmpty
            ? '发送给所有设备'
            : desktop
            ? '发送私信（Enter 发送，Shift+Enter 换行）'
            : '发送私信',
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 14,
          vertical: 12,
        ),
      ),
      onSubmitted: (_) => send(),
    );
    return Padding(
      padding: const EdgeInsets.fromLTRB(8, 8, 12, 12),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          IconButton(
            tooltip: '发送文件',
            onPressed:
                room.connected && room.pendingUpload == null && !room.picking
                ? room.pickFile
                : null,
            icon: const Icon(Icons.add_circle_outline, size: 30),
          ),
          Expanded(
            child: desktop
                ? Focus(onKeyEvent: enterToSend, child: input)
                : input,
          ),
          const SizedBox(width: 8),
          IconButton.filled(
            tooltip: '发送',
            onPressed: room.connected ? send : null,
            icon: const Icon(Icons.arrow_upward),
          ),
        ],
      ),
    );
  }

  Widget message(Map<String, dynamic> item) {
    final payload = Map<String, dynamic>.from(item['payload'] as Map? ?? {});
    final from = item['from'] as Map?;
    final own = from?['id'] == room.ownID;
    final private = (item['to'] as List? ?? []).isNotEmpty;
    final file = payload['kind'] == 'file' || payload['kind'] == 'image';
    return Align(
      alignment: own ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        constraints: BoxConstraints(maxWidth: desktop ? 520 : 320),
        margin: const EdgeInsets.only(bottom: 12),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: own ? const Color(0xffdce9ff) : Colors.white,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              '${from?['name'] ?? '设备'}${private ? ' · 私信' : ''}',
              style: const TextStyle(color: Color(0xff567299), fontSize: 11),
            ),
            const SizedBox(height: 5),
            if (file)
              InkWell(
                onTap: room.connected ? () => room.download(payload) : null,
                child: Padding(
                  padding: const EdgeInsets.symmetric(vertical: 4),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      const Icon(Icons.file_download_outlined),
                      const SizedBox(width: 8),
                      Flexible(
                        child: Text(
                          (payload['meta'] as Map?)?['name'] as String? ??
                              '下载文件',
                        ),
                      ),
                    ],
                  ),
                ),
              )
            else
              SelectableText(
                payload['content'] as String? ?? '',
                style: const TextStyle(fontSize: 15, height: 1.45),
              ),
          ],
        ),
      ),
    );
  }
}
