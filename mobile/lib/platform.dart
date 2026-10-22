import 'package:flutter/services.dart';

class AndroidBridge {
  static const channel = MethodChannel('lanroom/native');
  static const events = EventChannel('lanroom/network');
  Future<String?> read(String key) =>
      channel.invokeMethod<String>('read', {'key': key});
  Future<void> write(String key, String value) =>
      channel.invokeMethod('write', {'key': key, 'value': value});
  Future<String> get directory async =>
      (await channel.invokeMethod<String>('directory'))!;
  Future<List<Map<String, dynamic>>> get interfaces async =>
      (await channel.invokeListMethod<dynamic>('interfaces') ?? [])
          .map((e) => Map<String, dynamic>.from(e as Map))
          .toList();
  Future<Map<String, dynamic>?> pick() async {
    final result = await channel.invokeMapMethod<String, dynamic>('pick');
    return result == null ? null : Map<String, dynamic>.from(result);
  }

  Future<bool> save(String path, String name) async =>
      await channel.invokeMethod<bool>('save', {'path': path, 'name': name}) ??
      false;
  Stream<dynamic> get network => events.receiveBroadcastStream();
}
