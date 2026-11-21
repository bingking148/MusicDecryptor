package app.musicdecryptor.engine

enum class KeyPlan {
    EMBEDDED,
    DATABASE,
    MANUAL,
    STATIC,
    MISSING,
    MAC,
}

enum class TrackStatus {
    PENDING,
    RUNNING,
    SUCCESS,
    SKIPPED,
    FAILED,
}

enum class Phase {
    IDLE,
    RUNNING,
    DONE,
}

data class Track(
    val id: String,
    val uri: String,
    val name: String,
    val size: Long,
    val audioLength: Long = 0,
    val plan: KeyPlan = KeyPlan.MISSING,
    val planLabel: String = "",
    val status: TrackStatus = TrackStatus.PENDING,
    val message: String = "",
    val outputName: String = "",
    val outputUri: String? = null,
    val outputPath: String? = null,
    val title: String? = null,
    val artist: String? = null,
    val album: String? = null,
    val cover: ByteArray? = null,
)

data class Session(
    val phase: Phase = Phase.IDLE,
    val tracks: List<Track> = emptyList(),
    val doneBytes: Long = 0,
    val totalBytes: Long = 0,
    val runningId: String? = null,
    val runningFraction: Float = 0f,
    val startedAt: Long = 0,
    val finishedAt: Long = 0,
    val banner: String? = null,
    val scanning: Boolean = false,
) {
    val success: Int get() = tracks.count { it.status == TrackStatus.SUCCESS }
    val failed: Int get() = tracks.count { it.status == TrackStatus.FAILED }
    val skipped: Int get() = tracks.count { it.status == TrackStatus.SKIPPED }
    val pending: Int get() = tracks.count { it.status == TrackStatus.PENDING }

    val fraction: Float
        get() = if (totalBytes <= 0) 0f else (doneBytes.toFloat() / totalBytes).coerceIn(0f, 1f)
}

data class UserSettings(
    val outputMode: String = MODE_DOWNLOADS,
    val treeUri: String = "",
    val overwrite: Boolean = false,
    val autoExt: Boolean = true,
    val manualEkey: String = "",
    val computerHost: String = "",
) {
    companion object {
        const val MODE_DOWNLOADS = "downloads"
        const val MODE_APP = "app"
        const val MODE_TREE = "tree"
    }
}

fun outputLabel(settings: UserSettings): String = when (settings.outputMode) {
    UserSettings.MODE_APP -> "应用音乐目录"
    UserSettings.MODE_TREE -> "自选文件夹"
    else -> "下载 / MusicDecryptor"
}

fun planLabel(plan: KeyPlan): String = when (plan) {
    KeyPlan.EMBEDDED -> "内嵌密钥"
    KeyPlan.DATABASE -> "密钥库"
    KeyPlan.MANUAL -> "备用密钥"
    KeyPlan.STATIC -> "旧版静态密钥"
    KeyPlan.MISSING -> "缺少密钥"
    KeyPlan.MAC -> "需 Mac 密钥"
}
