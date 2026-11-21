package app.musicdecryptor.ui

import android.app.Application
import android.content.ClipData
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import app.musicdecryptor.MusicApp
import app.musicdecryptor.crypto.QmcSelfTest
import app.musicdecryptor.engine.DecryptEngine
import app.musicdecryptor.engine.DecryptService
import app.musicdecryptor.engine.Track
import app.musicdecryptor.engine.UserSettings
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import java.io.File

class HomeViewModel(app: Application) : AndroidViewModel(app) {
    private val music = app as MusicApp
    val settings = music.settings.flow.stateIn(
        viewModelScope,
        SharingStarted.WhileSubscribed(5_000),
        UserSettings(),
    )
    val keys = music.keys.state
    val session = DecryptEngine.session
    val toast = MutableStateFlow<String?>(null)
    val selfTest = MutableStateFlow<String?>(null)

    fun add(uris: List<Uri>) = DecryptEngine.add(uris)

    fun reprobe() = DecryptEngine.reprobe()

    fun remove(id: String) = DecryptEngine.remove(id)

    fun clearFinished() = DecryptEngine.clearFinished()

    fun start(context: Context) {
        if (DecryptEngine.start()) {
            ContextCompat.startForegroundService(context, Intent(context, DecryptService::class.java))
        }
    }

    fun cancel() = DecryptEngine.cancel()

    fun retry(context: Context) {
        DecryptEngine.retryFailed()
        if (DecryptEngine.isActive()) {
            ContextCompat.startForegroundService(context, Intent(context, DecryptService::class.java))
        }
    }

    fun retryOne(context: Context, id: String): Boolean {
        val started = DecryptEngine.retryOne(id)
        if (started) {
            ContextCompat.startForegroundService(context, Intent(context, DecryptService::class.java))
        }
        return started
    }

    fun setMode(mode: String) {
        viewModelScope.launch { music.settings.update { it.copy(outputMode = mode) } }
    }

    fun setTree(uri: Uri) {
        viewModelScope.launch {
            music.settings.update { it.copy(outputMode = UserSettings.MODE_TREE, treeUri = uri.toString()) }
        }
        toast.value = "输出文件夹已更新"
    }

    fun setOverwrite(value: Boolean) {
        viewModelScope.launch { music.settings.update { it.copy(overwrite = value) } }
    }

    fun setAutoExt(value: Boolean) {
        viewModelScope.launch { music.settings.update { it.copy(autoExt = value) } }
    }

    fun setManualEkey(value: String) {
        viewModelScope.launch {
            music.settings.update { it.copy(manualEkey = value.trim()) }
            DecryptEngine.reprobe()
        }
        toast.value = if (value.isBlank()) "已清除备用密钥" else "备用密钥已保存"
    }

    fun importKeys(uri: Uri) {
        viewModelScope.launch {
            val message = music.keys.import(uri)
            if (message.startsWith("BARE:")) {
                val ekey = message.removePrefix("BARE:")
                music.settings.update { it.copy(manualEkey = ekey) }
                toast.value = "已把这一条填进备用密钥"
            } else {
                toast.value = message
            }
            DecryptEngine.reprobe()
        }
    }

    fun clearKeys() {
        music.keys.clear()
        DecryptEngine.reprobe()
        toast.value = "密钥库已清除"
    }

    fun runSelfTest() {
        selfTest.value = "正在自检…"
        viewModelScope.launch(Dispatchers.Default) {
            val failures = QmcSelfTest.run()
            selfTest.value = if (failures.isEmpty()) "引擎自检通过" else "自检失败：${failures.joinToString("，")}"
        }
    }

    fun consumeToast() {
        toast.value = null
    }
}

fun shareTrack(context: Context, track: Track) {
    val uri = contentUri(context, track) ?: return
    val intent = Intent(Intent.ACTION_SEND).apply {
        type = "audio/*"
        putExtra(Intent.EXTRA_STREAM, uri)
        clipData = ClipData.newRawUri(track.outputName.ifBlank { track.name }, uri)
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
    }
    context.startActivity(Intent.createChooser(intent, "分享音频"))
}

fun openTrack(context: Context, track: Track) {
    val uri = contentUri(context, track) ?: return
    val intent = Intent(Intent.ACTION_VIEW).apply {
        setDataAndType(uri, "audio/*")
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
    }
    runCatching { context.startActivity(intent) }
        .onFailure { shareTrack(context, track) }
}

private fun contentUri(context: Context, track: Track): Uri? {
    track.outputPath?.let { path ->
        return FileProvider.getUriForFile(context, "${context.packageName}.files", File(path))
    }
    return track.outputUri?.let(Uri::parse)
}
