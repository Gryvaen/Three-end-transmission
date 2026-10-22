import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

String randomID([int bytes = 16]) {
  final random = Random.secure();
  return List.generate(
    bytes,
    (_) => random.nextInt(256).toRadixString(16).padLeft(2, '0'),
  ).join();
}

class FoundServer {
  final String id;
  final Uri uri;
  const FoundServer(this.id, this.uri);
  static FoundServer? parse(List<int> bytes, String sender, String requestID) {
    try {
      final data = jsonDecode(utf8.decode(bytes));
      if (data is! Map ||
          data['type'] != 'lanroom.offer' ||
          data['version'] != 1 ||
          data['requestId'] != requestID) {
        return null;
      }
      final port = data['port'];
      final id = data['serviceId'];
      if (port is! int ||
          port < 1 ||
          port > 65535 ||
          id is! String ||
          !RegExp(r'^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$')
              .hasMatch(id)) {
        return null;
      }
      return FoundServer(id, Uri(scheme: 'http', host: sender, port: port));
    } catch (_) {
      return null;
    }
  }
}

// Bind each physical interface independently so the default TUN route is not
// used to choose our source address. Native code supplies actual prefix masks.
Future<List<FoundServer>> discover(
  List<Map<String, dynamic>> interfaces, {
  int port = 8788,
  Duration duration = const Duration(seconds: 3),
}) async {
  final id = randomID();
  final query = utf8.encode(
    jsonEncode({'type': 'lanroom.discover', 'version': 1, 'requestId': id}),
  );
  final results = <String, FoundServer>{};
  final sockets = <RawDatagramSocket>[];
  final subscriptions = <StreamSubscription<RawSocketEvent>>[];
  final targets = <RawDatagramSocket, Set<String>>{};
  Timer? timer;
  try {
    for (final iface in interfaces) {
      try {
        final socket = await RawDatagramSocket.bind(
          InternetAddress(iface['ip'] as String),
          0,
        );
        socket.broadcastEnabled = true;
        sockets.add(socket);
        targets[socket] = {iface['broadcast'] as String, '255.255.255.255'};
        subscriptions.add(
          socket.listen((event) {
            if (event != RawSocketEvent.read) {
              return;
            }
            Datagram? packet;
            while ((packet = socket.receive()) != null) {
              final p = packet!;
              if (p.port != port || p.data.length > 1024) {
                continue;
              }
              final server = FoundServer.parse(p.data, p.address.address, id);
              if (server != null) {
                results['${server.id}@${server.uri}'] = server;
              }
            }
          }, onError: (Object _) {}),
        );
      } on SocketException {
        /* Another physical interface may still work. */
      }
    }
    if (sockets.isEmpty) {
      throw const SocketException('没有可用的局域网 IPv4 接口');
    }
    void send() {
      for (final entry in targets.entries) {
        for (final target in entry.value) {
          try {
            entry.key.send(query, InternetAddress(target), port);
          } on SocketException {
            /* Retry in this window. */
          }
        }
      }
    }

    send();
    timer = Timer.periodic(const Duration(milliseconds: 650), (_) => send());
    await Future<void>.delayed(duration);
    return results.values.toList();
  } finally {
    timer?.cancel();
    for (final subscription in subscriptions) {
      await subscription.cancel();
    }
    for (final socket in sockets) {
      socket.close();
    }
  }
}

FoundServer? preferredServer(List<FoundServer> servers, String? preferred) {
  if (preferred != null && preferred.isNotEmpty) {
    for (final server in servers) {
      if (server.id == preferred) {
        return server;
      }
    }
    return null;
  }
  return servers.length == 1 ? servers.single : null;
}
