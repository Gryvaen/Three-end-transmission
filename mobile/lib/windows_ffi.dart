import 'dart:ffi';
import 'dart:typed_data';

import 'package:ffi/ffi.dart';
import 'package:win32/win32.dart';

// Thin wrappers over the few Win32 calls the desktop bridge needs. Only call
// these when Platform.isWindows; the DLL lookups fail elsewhere.

const _cryptprotectUiForbidden = 0x1;
const _ifTypeEthernet = 6, _ifTypeWifi = 71;

// DPAPI with the current user's key, the Windows counterpart to the Android
// Keystore: another account or machine cannot read the ciphertext.
Uint8List protect(Uint8List data) => _dpapi(data, encrypt: true);
Uint8List unprotect(Uint8List data) => _dpapi(data, encrypt: false);

Uint8List _dpapi(Uint8List data, {required bool encrypt}) {
  final input = calloc<CRYPT_INTEGER_BLOB>();
  final output = calloc<CRYPT_INTEGER_BLOB>();
  final bytes = calloc<Uint8>(data.isEmpty ? 1 : data.length);
  try {
    bytes.asTypedList(data.length).setAll(0, data);
    input.ref
      ..cbData = data.length
      ..pbData = bytes;
    final result = encrypt
        ? CryptProtectData(
            input,
            null,
            null,
            null,
            _cryptprotectUiForbidden,
            output,
          )
        : CryptUnprotectData(
            input,
            null,
            null,
            null,
            _cryptprotectUiForbidden,
            output,
          );
    if (!result.value) {
      throw StateError('DPAPI 失败 (${result.error})');
    }
    final out = Uint8List.fromList(
      output.ref.pbData.asTypedList(output.ref.cbData),
    );
    LocalFree(HLOCAL(output.ref.pbData.cast()));
    return out;
  } finally {
    calloc.free(bytes);
    calloc.free(input);
    calloc.free(output);
  }
}

// Physical Ethernet and Wi-Fi adapters that are up, with their real prefix.
// TUN adapters (Mihomo, Tailscale, WireGuard) report other interface types
// and are skipped, matching the Android wlan/eth filter.
List<Map<String, String>> adapters() {
  final size = calloc<Uint32>()..value = 16 * 1024;
  Pointer<Uint8> buffer = nullptr;
  try {
    var status = 0;
    for (var attempt = 0; attempt < 3; attempt++) {
      buffer = calloc<Uint8>(size.value);
      status = GetAdaptersAddresses(
        AF_INET,
        GET_ADAPTERS_ADDRESSES_FLAGS(
          GAA_FLAG_SKIP_ANYCAST |
              GAA_FLAG_SKIP_MULTICAST |
              GAA_FLAG_SKIP_DNS_SERVER,
        ),
        buffer.cast(),
        size,
      );
      if (status != ERROR_BUFFER_OVERFLOW) {
        break;
      }
      calloc.free(buffer);
      buffer = nullptr;
    }
    if (status != 0) {
      throw StateError('GetAdaptersAddresses 失败 ($status)');
    }
    final list = <Map<String, String>>[];
    var adapter = buffer.cast<IP_ADAPTER_ADDRESSES_LH>();
    while (adapter != nullptr) {
      final ref = adapter.ref;
      if (ref.OperStatus == IfOperStatusUp &&
          (ref.IfType == _ifTypeEthernet || ref.IfType == _ifTypeWifi)) {
        final name = ref.FriendlyName.toDartString();
        var unicast = ref.FirstUnicastAddress;
        while (unicast != nullptr) {
          final entry = ipv4Entry(
            name,
            unicast.ref.Address.lpSockaddr.cast<Uint8>(),
            unicast.ref.OnLinkPrefixLength,
          );
          if (entry != null) {
            list.add(entry);
          }
          unicast = unicast.ref.Next;
        }
      }
      adapter = ref.Next;
    }
    return list;
  } finally {
    if (buffer != nullptr) {
      calloc.free(buffer);
    }
    calloc.free(size);
  }
}

Map<String, String>? ipv4Entry(
  String name,
  Pointer<Uint8> sockaddr,
  int prefix,
) {
  // SOCKADDR_IN: u16 family, u16 port, then the address in network order.
  if (sockaddr == nullptr || sockaddr.cast<Uint16>().value != AF_INET) {
    return null;
  }
  final ip = (sockaddr + 4).asTypedList(4);
  final address = ip.join('.');
  final broadcast = broadcastAddress(address, prefix);
  return broadcast == null
      ? null
      : {'name': name, 'ip': address, 'broadcast': broadcast};
}

/// The directed broadcast for [ip]/[prefix]; null when the subnet has none
/// (point-to-point /31, host /32) or the address is link-local.
String? broadcastAddress(String ip, int prefix) {
  final parts = ip.split('.').map(int.parse).toList();
  if (parts.length != 4 || prefix < 1 || prefix > 30) {
    return null;
  }
  if (parts[0] == 169 && parts[1] == 254) {
    return null;
  }
  final value =
      (parts[0] << 24) | (parts[1] << 16) | (parts[2] << 8) | parts[3];
  final host = (1 << (32 - prefix)) - 1;
  final result = value | host;
  return [24, 16, 8, 0].map((s) => (result >> s) & 0xff).join('.');
}

// Keep the machine from sleeping mid-transfer; the display may still turn off.
void keepAwake(bool on) {
  SetThreadExecutionState(
    EXECUTION_STATE(on ? ES_CONTINUOUS | ES_SYSTEM_REQUIRED : ES_CONTINUOUS),
  );
}
