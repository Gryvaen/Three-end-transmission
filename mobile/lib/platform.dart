import 'dart:io';

import 'package:flutter/services.dart';

import 'desktop_bridge.dart';

// Everything Room needs from the host OS. Android talks to MainActivity over
// platform channels; desktop builds implement it in Dart (desktop_bridge.dart).
abstract class NativeBridge {
  factory NativeBridge.create() =>
      Platform.isAndroid ? AndroidBridge() : DesktopBridge();
  NativeBridge.base();

  /// Reported to the server so other devices can pick a matching icon.
  String get platform;
  Future<String?> read(String key);
  Future<void> write(String key, String value);
  Future<String?> get deviceName;
  Future<String> get directory;
  Future<List<Map<String, dynamic>>> get interfaces;

  /// Resolves to `{path, name, size, mime}`; `owned: false` means [path] is the
  /// user's original file and must not be deleted after the upload.
  Future<Map<String, dynamic>?> pick();
  Future<bool> save(String path, String name);
  Future<bool> permission();
  Future<void> keepAlive(bool on, [String? label]);
  Future<void> transferring(bool on);
  Stream<dynamic> get network;
}

class AndroidBridge extends NativeBridge {
  AndroidBridge() : super.base();
  static const channel = MethodChannel('lanroom/native');
  static const events = EventChannel('lanroom/network');
  @override
  String get platform => 'android';
  @override
  Future<String?> read(String key) =>
      channel.invokeMethod<String>('read', {'key': key});
  @override
  Future<void> write(String key, String value) =>
      channel.invokeMethod('write', {'key': key, 'value': value});
  @override
  Future<String?> get deviceName => channel.invokeMethod<String>('deviceName');
  @override
  Future<String> get directory async =>
      (await channel.invokeMethod<String>('directory'))!;
  @override
  Future<List<Map<String, dynamic>>> get interfaces async =>
      (await channel.invokeListMethod<dynamic>('interfaces') ?? [])
          .map((e) => Map<String, dynamic>.from(e as Map))
          .toList();
  @override
  Future<Map<String, dynamic>?> pick() async {
    final result = await channel.invokeMapMethod<String, dynamic>('pick');
    return result == null ? null : Map<String, dynamic>.from(result);
  }

  @override
  Future<bool> save(String path, String name) async =>
      await channel.invokeMethod<bool>('save', {'path': path, 'name': name}) ??
      false;
  @override
  Future<bool> permission() async =>
      await channel.invokeMethod<bool>('permission') ?? false;
  @override
  Future<void> keepAlive(bool on, [String? label]) =>
      channel.invokeMethod('service', {'on': on, 'label': label});
  @override
  Future<void> transferring(bool on) =>
      channel.invokeMethod('wakelock', {'on': on});
  @override
  Stream<dynamic> get network => events.receiveBroadcastStream();
}
