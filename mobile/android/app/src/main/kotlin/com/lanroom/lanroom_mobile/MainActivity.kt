package com.lanroom.lanroom_mobile

import android.app.Activity
import android.content.Intent
import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.wifi.WifiManager
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.PowerManager
import android.provider.OpenableColumns
import android.provider.Settings
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import io.flutter.plugin.common.EventChannel
import java.io.File
import java.net.Inet4Address
import java.net.NetworkInterface
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import java.util.UUID
import java.util.concurrent.Executors

class MainActivity: FlutterActivity() {
    private val worker = Executors.newSingleThreadExecutor()
    private val main = Handler(Looper.getMainLooper())
    private var pending: MethodChannel.Result? = null
    private var pendingSource: String? = null
    private var lock: WifiManager.MulticastLock? = null
    private var networkCallback: ConnectivityManager.NetworkCallback? = null
    private var permissionResult: MethodChannel.Result? = null
    private var serviceLabel: String? = null
    private var wakeLock: PowerManager.WakeLock? = null

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "lanroom/native").setMethodCallHandler { call, result ->
            try {
                when(call.method) {
                    "directory" -> result.success(filesDir.absolutePath)
                    "deviceName" -> result.success(deviceName())
                    "read" -> result.success(readSecret(call.argument<String>("key")!!))
                    "write" -> { writeSecret(call.argument<String>("key")!!, call.argument<String>("value")!!); result.success(null) }
                    "interfaces" -> {
                        if (lock == null) {
                            lock = (applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager).createMulticastLock("lanroom-discovery").apply { setReferenceCounted(false); acquire() }
                        }
                        val list = mutableListOf<Map<String,String>>()
                        for (iface in NetworkInterface.getNetworkInterfaces()) {
                            val name = iface.name.lowercase()
                            if (!iface.isUp || iface.isLoopback || iface.isPointToPoint ||
                                !(name.startsWith("wlan") || name.startsWith("eth") || name.startsWith("ap") || name.startsWith("usb"))) continue
                            for (address in iface.interfaceAddresses) {
                                if (address.address is Inet4Address && address.broadcast != null) {
                                    list.add(mapOf("name" to iface.name, "ip" to address.address.hostAddress!!, "broadcast" to address.broadcast.hostAddress!!))
                                }
                            }
                        }
                        result.success(list)
                    }
                    "pick" -> {
                        if (pending != null) { result.error("BUSY", "文件选择器正在使用", null) } else {
                            pending = result
                            startActivityForResult(Intent(Intent.ACTION_OPEN_DOCUMENT).apply { type = "*/*"; addCategory(Intent.CATEGORY_OPENABLE) }, 101)
                        }
                    }
                    "save" -> {
                        if (pending != null) { result.error("BUSY", "文件选择器正在使用", null) } else {
                            pending = result; pendingSource = call.argument<String>("path")!!
                            startActivityForResult(Intent(Intent.ACTION_CREATE_DOCUMENT).apply {
                                type = "application/octet-stream"; addCategory(Intent.CATEGORY_OPENABLE)
                                putExtra(Intent.EXTRA_TITLE, call.argument<String>("name"))
                            }, 102)
                        }
                    }
                    "permission" -> {
                        // Android 17 / target 37 introduces runtime local-network access.
                        if (Build.VERSION.SDK_INT >= 37 && applicationInfo.targetSdkVersion >= 37 &&
                            checkSelfPermission("android.permission.ACCESS_LOCAL_NETWORK") != android.content.pm.PackageManager.PERMISSION_GRANTED) {
                            permissionResult = result
                            requestPermissions(arrayOf("android.permission.ACCESS_LOCAL_NETWORK"), 103)
                        } else result.success(true)
                    }
                    "service" -> {
                        if (call.argument<Boolean>("on") == true) {
                            serviceLabel = call.argument<String>("label")
                            // Without the permission the service still runs; only the notification is hidden.
                            if (Build.VERSION.SDK_INT >= 33 &&
                                checkSelfPermission("android.permission.POST_NOTIFICATIONS") != android.content.pm.PackageManager.PERMISSION_GRANTED) {
                                requestPermissions(arrayOf("android.permission.POST_NOTIFICATIONS"), 104)
                            } else startConnectionService()
                        } else stopService(Intent(this, ConnectionService::class.java))
                        result.success(null)
                    }
                    "wakelock" -> {
                        if (call.argument<Boolean>("on") == true) {
                            if (wakeLock == null) {
                                // Timeout guards against a lock leaked by a stuck transfer.
                                wakeLock = (getSystemService(Context.POWER_SERVICE) as PowerManager)
                                    .newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "lanroom:transfer").apply { acquire(30 * 60 * 1000L) }
                            }
                        } else releaseWakeLock()
                        result.success(null)
                    }
                    else -> result.notImplemented()
                }
            } catch(e: Exception) { result.error("NATIVE", e.message, null) }
        }
        val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        EventChannel(flutterEngine.dartExecutor.binaryMessenger, "lanroom/network").setStreamHandler(object: EventChannel.StreamHandler {
            override fun onListen(arguments: Any?, sink: EventChannel.EventSink) {
                networkCallback = object: ConnectivityManager.NetworkCallback() {
                    override fun onAvailable(network: Network) { main.post { sink.success("changed") } }
                    override fun onLost(network: Network) { main.post { sink.success("changed") } }
                    override fun onLinkPropertiesChanged(network: Network, properties: android.net.LinkProperties) { main.post { sink.success("changed") } }
                }
                cm.registerNetworkCallback(android.net.NetworkRequest.Builder().addTransportType(NetworkCapabilities.TRANSPORT_WIFI).build(), networkCallback!!)
            }
            override fun onCancel(arguments: Any?) { networkCallback?.let { cm.unregisterNetworkCallback(it) }; networkCallback = null }
        })
    }

    // The name set under Settings > About phone; falls back to the model on ROMs without it.
    // Hidden keys such as bluetooth_name throw SecurityException for targetSdk 31+.
    private fun deviceName(): String {
        val name = if (Build.VERSION.SDK_INT >= 25) {
            try { Settings.Global.getString(contentResolver, Settings.Global.DEVICE_NAME) } catch (e: Exception) { null }
        } else null
        if (!name.isNullOrBlank()) return name.trim()
        val model = Build.MODEL.trim()
        return if (model.startsWith(Build.MANUFACTURER, ignoreCase = true)) model else "${Build.MANUFACTURER} $model"
    }
    private fun key(): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey("lanroom-secrets", null) as? SecretKey)?.let { return it }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder("lanroom-secrets", KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
    }
    private fun writeSecret(name: String, value: String) {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding"); cipher.init(Cipher.ENCRYPT_MODE, key())
        val bytes = cipher.doFinal(value.toByteArray(Charsets.UTF_8))
        val encoded = Base64.encodeToString(cipher.iv, Base64.NO_WRAP) + ":" + Base64.encodeToString(bytes, Base64.NO_WRAP)
        if (!getSharedPreferences("secrets", MODE_PRIVATE).edit().putString(name, encoded).commit()) error("保存失败")
    }
    private fun readSecret(name: String): String? {
        val value = getSharedPreferences("secrets", MODE_PRIVATE).getString(name, null) ?: return null
        val parts = value.split(":")
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, Base64.decode(parts[0], Base64.NO_WRAP)))
        return String(cipher.doFinal(Base64.decode(parts[1], Base64.NO_WRAP)), Charsets.UTF_8)
    }
    @Deprecated("Activity result compatibility")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != 101 && requestCode != 102) return
        val result = pending ?: return
        pending = null
        val source = pendingSource; pendingSource = null
        val uri = data?.data
        if (resultCode != Activity.RESULT_OK || uri == null) { result.success(if(requestCode == 102) false else null); return }
        worker.execute {
            var staged: File? = null
            try {
                if (requestCode == 101) {
                    var name = "文件"
                    contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { cursor -> if(cursor.moveToFirst()) name = cursor.getString(0) }
                    val dest = File(filesDir, "pending-${UUID.randomUUID()}"); staged = dest
                    contentResolver.openInputStream(uri)!!.use { input -> dest.outputStream().use { output -> input.copyTo(output) } }
                    val mime = contentResolver.getType(uri) ?: "application/octet-stream"
                    main.post { result.success(mapOf("path" to dest.absolutePath, "name" to name, "size" to dest.length(), "mime" to mime)) }
                } else {
                    File(source!!).inputStream().use { input -> contentResolver.openOutputStream(uri, "w")!!.use { output -> input.copyTo(output) } }
                    main.post { result.success(true) }
                }
            } catch(e: Exception) { staged?.delete(); main.post { result.error("FILE", e.message, null) } }
        }
    }
    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if(requestCode == 103) { permissionResult?.success(grantResults.isNotEmpty() && grantResults[0] == 0); permissionResult = null }
        if(requestCode == 104) startConnectionService()
    }
    private fun startConnectionService() {
        val intent = Intent(this, ConnectionService::class.java).putExtra("label", serviceLabel)
        try {
            if (Build.VERSION.SDK_INT >= 26) startForegroundService(intent) else startService(intent)
        } catch (e: Exception) { /* Background start is refused; the app reconnects on resume instead. */ }
    }
    private fun releaseWakeLock() { wakeLock?.let { if(it.isHeld) it.release() }; wakeLock = null }
    // The connection lives in this activity's Flutter engine; nothing survives it.
    override fun onDestroy() {
        lock?.let { if(it.isHeld) it.release() }; releaseWakeLock()
        stopService(Intent(this, ConnectionService::class.java))
        worker.shutdown(); super.onDestroy()
    }
}
