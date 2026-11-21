package app.musicdecryptor.engine

import android.app.Notification
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationChannelCompat
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.app.ServiceCompat
import app.musicdecryptor.MainActivity
import app.musicdecryptor.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

class DecryptService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var watch: Job? = null

    override fun onCreate() {
        super.onCreate()
        ensureChannel()
        startInForeground(buildNotification(DecryptEngine.session.value))
        watch = scope.launch {
            while (isActive) {
                val current = DecryptEngine.session.value
                val running = DecryptEngine.isActive() || current.phase == Phase.RUNNING
                NotificationManagerCompat.from(this@DecryptService).notify(ID, buildNotification(current))
                if (!running) break
                delay(400)
            }
            delay(200)
            NotificationManagerCompat.from(this@DecryptService)
                .notify(ID, buildNotification(DecryptEngine.session.value))
            ServiceCompat.stopForeground(this@DecryptService, ServiceCompat.STOP_FOREGROUND_DETACH)
            stopSelf()
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_CANCEL) DecryptEngine.cancel()
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        watch?.cancel()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun startInForeground(notification: Notification) {
        if (Build.VERSION.SDK_INT >= 29) {
            ServiceCompat.startForeground(this, ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        } else {
            startForeground(ID, notification)
        }
    }

    private fun ensureChannel() {
        val channel = NotificationChannelCompat.Builder(CHANNEL, NotificationManagerCompat.IMPORTANCE_LOW)
            .setName(getString(R.string.channel_name))
            .setDescription(getString(R.string.channel_desc))
            .build()
        NotificationManagerCompat.from(this).createNotificationChannel(channel)
    }

    private fun buildNotification(session: Session): Notification {
        val open = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val cancel = PendingIntent.getService(
            this,
            1,
            Intent(this, DecryptService::class.java).setAction(ACTION_CANCEL),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val done = session.success + session.failed + session.skipped
        val title = when (session.phase) {
            Phase.RUNNING -> "正在解密 $done/${session.tracks.size}"
            Phase.DONE -> "解密结束：成功 ${session.success}，失败 ${session.failed}"
            Phase.IDLE -> "音乐解密"
        }
        val text = session.tracks.firstOrNull { it.id == session.runningId }?.name
            ?: session.banner
            ?: "源文件不会被改动"
        val builder = NotificationCompat.Builder(this, CHANNEL)
            .setSmallIcon(R.drawable.ic_stat_disc)
            .setContentTitle(title)
            .setContentText(text)
            .setContentIntent(open)
            .setOnlyAlertOnce(true)
            .setOngoing(session.phase == Phase.RUNNING)
            .setSilent(true)
        if (session.totalBytes > 0) {
            builder.setProgress(1000, (session.fraction * 1000).toInt(), session.phase != Phase.RUNNING && session.totalBytes == 0L)
        }
        if (session.phase == Phase.RUNNING) {
            builder.addAction(0, "取消", cancel)
        }
        return builder.build()
    }

    companion object {
        const val ACTION_CANCEL = "app.musicdecryptor.CANCEL"
        private const val CHANNEL = "decrypt"
        private const val ID = 42
    }
}
