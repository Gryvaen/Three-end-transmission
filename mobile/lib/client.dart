import 'dart:async';
import 'dart:convert';
import 'dart:io';

class ApiException implements Exception {
  final int status;
  final String message;
  ApiException(this.status, this.message);
  @override
  String toString() => status == 401 ? '需要重新输入房间口令' : '请求失败 ($status)：$message';
}

class LanClient {
  final Uri base;
  final HttpClient http = HttpClient()
    ..connectionTimeout = const Duration(seconds: 5);
  String? cookie;
  WebSocket? socket;
  LanClient(this.base, {this.cookie}) {
    // Do not inherit HTTP_PROXY for a discovered LAN endpoint.
    http.findProxy = (_) => 'DIRECT';
  }
  Uri url(String path) => base.resolve(path);
  Future<HttpClientResponse> request(
    String method,
    String path, {
    Object? json,
    List<int>? bytes,
    bool acceptConflict = false,
  }) async {
    final req = await http
        .openUrl(method, url(path))
        .timeout(const Duration(seconds: 8));
    req.followRedirects = false;
    if (cookie != null) {
      req.headers.set(HttpHeaders.cookieHeader, cookie!);
    }
    if (json != null) {
      req.headers.contentType = ContentType.json;
      req.add(utf8.encode(jsonEncode(json)));
    } else if (bytes != null) {
      req.contentLength = bytes.length;
      req.add(bytes);
    }
    final response = await req.close().timeout(const Duration(seconds: 45));
    for (final c in response.cookies) {
      if (c.name == 'lanroom_session') {
        cookie = '${c.name}=${c.value}';
      }
    }
    if (response.statusCode >= 300 &&
        !(acceptConflict && response.statusCode == 409)) {
      final body = await utf8.decoder.bind(response).join();
      throw ApiException(response.statusCode, body.trim());
    }
    return response;
  }

  Future<Map<String, dynamic>> json(
    String method,
    String path, {
    Object? body,
    List<int>? bytes,
    bool acceptConflict = false,
  }) async {
    final response = await request(
      method,
      path,
      json: body,
      bytes: bytes,
      acceptConflict: acceptConflict,
    );
    return Map<String, dynamic>.from(
      jsonDecode(await utf8.decoder.bind(response).join()) as Map,
    );
  }

  Future<Map<String, dynamic>> info() => json('GET', '/api/info');
  Future<void> login(String pin) async {
    await json('POST', '/api/auth', body: {'pin': pin});
  }

  Future<WebSocket> connect(String name, String key) async {
    final uri = base.replace(
      scheme: base.scheme == 'https' ? 'wss' : 'ws',
      path: '/ws',
      queryParameters: {'name': name, 'key': key, 'platform': 'android'},
    );
    final ws = await WebSocket.connect(
      uri.toString(),
      customClient: http,
      headers: cookie == null ? null : {'Cookie': cookie!},
    ).timeout(const Duration(seconds: 8));
    ws.pingInterval = const Duration(seconds: 20);
    socket = ws;
    return ws;
  }

  void send(Map<String, dynamic> payload, List<String> recipients) {
    if (socket?.readyState != WebSocket.open) {
      throw StateError('连接已断开，请稍后重试');
    }
    socket!.add(
      jsonEncode({'type': 'message', 'to': recipients, 'payload': payload}),
    );
  }

  void close() {
    socket?.close();
    http.close(force: true);
  }
}
