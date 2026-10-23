package com.lanroom.lanroom_mobile

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder

// Keeps the process out of the cached/frozen state so the room WebSocket
// survives backgrounding and the system file picker.
class ConnectionService: Service() {
    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val open = PendingIntent.getActivity(this, 0,
            Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val builder = if (Build.VERSION.SDK_INT >= 26) {
            getSystemService(NotificationManager::class.java).createNotificationChannel(
                NotificationChannel(CHANNEL, "连接状态", NotificationManager.IMPORTANCE_LOW))
            Notification.Builder(this, CHANNEL)
        } else @Suppress("DEPRECATION") Notification.Builder(this)
        val notification = builder.setSmallIcon(R.drawable.ic_lanroom)
            .setContentTitle("LanRoom 已连接")
            .setContentText(intent?.getStringExtra("label") ?: "保持局域网连接")
            .setContentIntent(open).setOngoing(true).build()
        if (Build.VERSION.SDK_INT >= 29) startForeground(1, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE)
        else startForeground(1, notification)
        return START_NOT_STICKY
    }

    // The Flutter engine dies with the task, so there is nothing left to keep alive.
    override fun onTaskRemoved(rootIntent: Intent?) { stopSelf(); super.onTaskRemoved(rootIntent) }

    companion object { const val CHANNEL = "lanroom_connection" }
}
