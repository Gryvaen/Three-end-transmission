import 'package:flutter/material.dart';

import 'room.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const LanRoomApp());
}

class LanRoomApp extends StatelessWidget {
  final Room? room;
  const LanRoomApp({super.key, this.room});
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
    home: RoomScreen(room: room),
  );
}

class RoomScreen extends StatefulWidget {
  final Room? room;
  const RoomScreen({super.key, this.room});
  @override
  State<RoomScreen> createState() => _RoomScreenState();
}

class _RoomScreenState extends State<RoomScreen> with WidgetsBindingObserver {
  late final Room room;
  final text = TextEditingController(), pin = TextEditingController();
  final scroll = ScrollController();
  int messageCount = 0;
  @override
  void initState() {
    super.initState();
    room = widget.room ?? Room();
    room.addListener(update);
    WidgetsBinding.instance.addObserver(this);
    if (widget.room == null) {
      room.initialize();
    }
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
    room.foreground(state == AppLifecycleState.resumed);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    room.removeListener(update);
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

  @override
  Widget build(BuildContext context) => Scaffold(
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
              showDialog<void>(
                context: context,
                builder: (context) => AlertDialog(
                  title: const Text('连接排查'),
                  content: const Text(
                    '手机和电脑需处于同一局域网。\n\n电脑需运行新版 Go 服务端，并允许 UDP 8788 和 HTTP 服务端口。\n\nMihomo TUN 可能影响路由，请检查局域网绕过配置。应用不会修改代理或防火墙。\n\n首版支持前台互传，回到应用后会自动重连。',
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
          },
          itemBuilder: (_) => const [
            PopupMenuItem(value: 'manual', child: Text('手动连接')),
            PopupMenuItem(value: 'search', child: Text('切换服务器')),
            PopupMenuItem(value: 'help', child: Text('连接排查')),
          ],
        ),
      ],
    ),
    body: SafeArea(
      child: Column(
        children: [
          Container(
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
                        style: const TextStyle(
                          color: Color(0xffc4daf8),
                          fontSize: 12,
                        ),
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
          ),
          if (room.error != null)
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Icon(
                    Icons.info_outline,
                    color: Color(0xff9b4715),
                    size: 18,
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      room.error!,
                      style: const TextStyle(
                        color: Color(0xff9b4715),
                        fontSize: 12,
                      ),
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
            ),
          if (room.needsPin)
            Padding(
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
            ),
          if (!room.connected && !room.needsPin && room.servers.isNotEmpty)
            Flexible(
              child: ListView(
                shrinkWrap: true,
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
              ),
            ),
          if (room.connected) ...[
            SizedBox(
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
                      onSelected: (_) {
                        room.recipients = [];
                        room.changed();
                      },
                    ),
                  ),
                  ...room.devices
                      .where((d) => d['id'] != room.ownID)
                      .map(
                        (device) => Padding(
                          padding: const EdgeInsets.only(right: 8),
                          child: ChoiceChip(
                            avatar: Icon(
                              device['platform'] == 'android' ||
                                      device['platform'] == 'ios'
                                  ? Icons.smartphone
                                  : Icons.computer,
                              size: 18,
                            ),
                            label: Text(device['name'] as String),
                            selected: room.recipients.contains(device['id']),
                            onSelected: (_) {
                              room.recipients = [device['id'] as String];
                              room.changed();
                            },
                          ),
                        ),
                      ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: Row(
                children: [
                  Text(
                    '${room.devices.length} 台在线设备',
                    style: const TextStyle(
                      color: Colors.blueGrey,
                      fontSize: 12,
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
            Expanded(
              child: room.messages.isEmpty
                  ? const Center(
                      child: Text(
                        '选择设备，发送第一条消息',
                        style: TextStyle(color: Colors.blueGrey),
                      ),
                    )
                  : ListView.builder(
                      controller: scroll,
                      padding: const EdgeInsets.all(16),
                      itemCount: room.messages.length,
                      itemBuilder: (_, index) => message(room.messages[index]),
                    ),
            ),
          ] else if (room.servers.isEmpty || room.needsPin)
            Expanded(
              child: Center(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(
                      Icons.devices_rounded,
                      size: 66,
                      color: Color(0xff7f9dc6),
                    ),
                    const SizedBox(height: 16),
                    const Text(
                      '让手机与电脑相连',
                      style: TextStyle(
                        fontSize: 20,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 8),
                    const Text(
                      '电脑运行 LanRoom，手机自动寻找服务',
                      style: TextStyle(color: Colors.blueGrey),
                    ),
                    const SizedBox(height: 20),
                    OutlinedButton.icon(
                      onPressed: () => edit(),
                      icon: const Icon(Icons.link),
                      label: const Text('手动输入地址'),
                    ),
                  ],
                ),
              ),
            ),
          if (room.transferStatus != null)
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 8),
              child: Column(
                children: [
                  if (room.uploading || room.downloading)
                    LinearProgressIndicator(
                      value: room.downloading
                          ? room.downloadProgress
                          : room.progress,
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
                            onPressed: room.connected
                                ? room.resumeUpload
                                : null,
                            child: const Text('继续'),
                          ),
                        TextButton(
                          onPressed: room.cancelUpload,
                          child: const Text('取消'),
                        ),
                      ],
                    ],
                  ),
                ],
              ),
            ),
          Padding(
            padding: const EdgeInsets.fromLTRB(8, 8, 12, 12),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                IconButton(
                  tooltip: '发送文件',
                  onPressed:
                      room.connected &&
                          room.pendingUpload == null &&
                          !room.picking
                      ? room.pickFile
                      : null,
                  icon: const Icon(Icons.add_circle_outline, size: 30),
                ),
                Expanded(
                  child: TextField(
                    key: const Key('messageInput'),
                    controller: text,
                    enabled: room.connected,
                    minLines: 1,
                    maxLines: 4,
                    decoration: InputDecoration(
                      hintText: room.recipients.isEmpty ? '发送给所有设备' : '发送私信',
                      contentPadding: const EdgeInsets.symmetric(
                        horizontal: 14,
                        vertical: 12,
                      ),
                    ),
                    onSubmitted: (_) => send(),
                  ),
                ),
                const SizedBox(width: 8),
                IconButton.filled(
                  tooltip: '发送',
                  onPressed: room.connected ? send : null,
                  icon: const Icon(Icons.arrow_upward),
                ),
              ],
            ),
          ),
        ],
      ),
    ),
  );
  Widget message(Map<String, dynamic> item) {
    final payload = Map<String, dynamic>.from(item['payload'] as Map? ?? {});
    final from = item['from'] as Map?;
    final own = from?['id'] == room.ownID;
    final private = (item['to'] as List? ?? []).isNotEmpty;
    final file = payload['kind'] == 'file' || payload['kind'] == 'image';
    return Align(
      alignment: own ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        constraints: const BoxConstraints(maxWidth: 320),
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
