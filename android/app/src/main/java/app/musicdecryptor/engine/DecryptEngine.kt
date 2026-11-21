package app.musicdecryptor.engine

import android.content.Context
import android.net.Uri
import app.musicdecryptor.crypto.AudioKind
import app.musicdecryptor.crypto.AudioMeta
import app.musicdecryptor.crypto.readAudioMeta
import app.musicdecryptor.crypto.BadKeyException
import app.musicdecryptor.crypto.CancelledException
import app.musicdecryptor.crypto.FooterKind
import app.musicdecryptor.crypto.QmcCipher
import app.musicdecryptor.crypto.QmcProbe
import app.musicdecryptor.crypto.decryptAudio
import app.musicdecryptor.crypto.decryptPrefix
import app.musicdecryptor.crypto.deriveEKey
import app.musicdecryptor.crypto.isQmcFileName
import app.musicdecryptor.crypto.outputStem
import app.musicdecryptor.crypto.preferredExtension
import app.musicdecryptor.crypto.probeQmc
import app.musicdecryptor.crypto.qmcCipher
import app.musicdecryptor.crypto.sniffAudio
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
import java.io.FileInputStream
import java.nio.ByteBuffer
import java.security.MessageDigest
import kotlin.coroutines.coroutineContext
import app.musicdecryptor.crypto.ByteSource

object DecryptEngine {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    val session = MutableStateFlow(Session())

    private lateinit var app: Context
    private lateinit var settings: SettingsStore
    private lateinit var keys: KeyLibrary
    private var job: Job? = null
    private var scanJob: Job? = null

    fun attach(context: Context, settings: SettingsStore, keys: KeyLibrary) {
        app = context.applicationContext
        this.settings = settings
        this.keys = keys
    }

    fun isActive(): Boolean = job?.isActive == true

    fun add(uris: List<Uri>) {
        if (uris.isEmpty()) return
        if (isActive()) {
            session.update { it.copy(banner = "解密进行中，先等这一批结束再添加") }
            return
        }
        scanJob?.cancel()
        scanJob = scope.launch {
            session.update { it.copy(scanning = true, banner = null) }
            val manual = settings.flow.first().manualEkey
            val known = session.value.tracks.map { it.id }.toMutableSet()
            val added = mutableListOf<Track>()
            for (uri in uris) {
                val id = trackId(uri)
                if (!known.add(id)) continue
                added += inspect(uri, id, manual)
            }
            session.update { current ->
                current.copy(
                    scanning = false,
                    phase = if (current.phase == Phase.DONE) Phase.IDLE else current.phase,
                    tracks = current.tracks + added,
                    banner = when {
                        added.isEmpty() -> "这些文件已经在列表里"
                        added.all { it.plan == KeyPlan.MISSING || it.plan == KeyPlan.MAC } ->
                            "这些文件还没有可用密钥"
                        else -> null
                    },
                )
            }
        }
    }

    fun reprobe() {
        if (isActive()) return
        val targets = session.value.tracks.filter {
            it.status == TrackStatus.PENDING || it.status == TrackStatus.FAILED
        }
        if (targets.isEmpty()) {
            session.update { it.copy(scanning = false) }
            return
        }
        scanJob?.cancel()
        scanJob = scope.launch {
            session.update { it.copy(scanning = true) }
            val manual = settings.flow.first().manualEkey
            val refreshed = targets.associate { track ->
                track.id to inspect(Uri.parse(track.uri), track.id, manual).copy(
                    status = TrackStatus.PENDING,
                    message = "",
                )
            }
            session.update { current ->
                current.copy(
                    scanning = false,
                    tracks = current.tracks.map { refreshed[it.id] ?: it },
                )
            }
        }
    }

    fun remove(id: String) {
        if (isActive()) return
        session.update { it.copy(tracks = it.tracks.filterNot { track -> track.id == id }) }
    }

    fun clearFinished() {
        if (isActive()) return
        session.update {
            it.copy(
                phase = Phase.IDLE,
                tracks = it.tracks.filter { track ->
                    track.status == TrackStatus.PENDING || track.status == TrackStatus.FAILED
                },
                banner = null,
            )
        }
    }

    /** @return 是否真的启动了任务。调用方据此决定要不要拉起前台服务。 */
    fun start(): Boolean {
        if (isActive()) return false
        val pending = session.value.tracks.any { it.status == TrackStatus.PENDING }
        if (!pending) {
            session.update { it.copy(banner = "没有待解密的文件") }
            return false
        }
        job = scope.launch { run() }
        return true
    }

    fun retryFailed() {
        if (isActive()) return
        session.update { current ->
            current.copy(
                phase = Phase.IDLE,
                banner = null,
                tracks = current.tracks.map { track ->
                    if (track.status == TrackStatus.FAILED) {
                        track.copy(status = TrackStatus.PENDING, message = "", outputUri = null, outputPath = null)
                    } else {
                        track
                    }
                },
            )
        }
        start()
    }

    fun retryOne(id: String): Boolean {
        if (isActive()) return false
        val target = session.value.tracks.any { it.id == id && it.status == TrackStatus.FAILED }
        if (!target) return false
        session.update { current ->
            current.copy(
                phase = Phase.IDLE,
                tracks = current.tracks.map { track ->
                    if (track.id == id) track.copy(status = TrackStatus.PENDING, message = "", outputUri = null, outputPath = null) else track
                },
            )
        }
        return start()
    }

    fun cancel() {
        job?.cancel()
    }

    private suspend fun run() {
        val cfg = settings.flow.first()
        val started = System.currentTimeMillis()
        val planner = OutputPlanner(app, cfg)
        val queue = session.value.tracks.filter { it.status == TrackStatus.PENDING }
        val total = queue.sumOf { it.audioLength.coerceAtLeast(0) }
        session.update {
            it.copy(
                phase = Phase.RUNNING,
                banner = planner.warning,
                startedAt = started,
                finishedAt = 0,
                doneBytes = 0,
                totalBytes = total,
                runningId = null,
                runningFraction = 0f,
            )
        }
        var completed = 0L
        for (track in queue) {
            if (!coroutineContext.isActive) break
            updateTrack(track.id) { it.copy(status = TrackStatus.RUNNING, message = "") }
            session.update { it.copy(runningId = track.id, runningFraction = 0f) }
            val jobContext = coroutineContext
            val outcome = withContext(Dispatchers.IO) {
                decryptOne(track, cfg, planner, isCancelled = { !jobContext.isActive }) { written, length ->
                    val fraction = if (length <= 0) 1f else written.toFloat() / length
                    session.update {
                        it.copy(
                            doneBytes = completed + written,
                            runningFraction = fraction.coerceIn(0f, 1f),
                        )
                    }
                }
            }
            completed += track.audioLength.coerceAtLeast(0)
            updateTrack(track.id) { outcome }
            session.update { it.copy(doneBytes = completed, runningFraction = 1f) }
        }
        val cancelled = !coroutineContext.isActive
        session.update { current ->
            val tracks = current.tracks.map { track ->
                if (cancelled && (track.status == TrackStatus.PENDING || track.status == TrackStatus.RUNNING)) {
                    track.copy(status = TrackStatus.FAILED, message = "已取消")
                } else {
                    track
                }
            }
            current.copy(
                phase = Phase.DONE,
                tracks = tracks,
                runningId = null,
                runningFraction = 0f,
                finishedAt = System.currentTimeMillis(),
                doneBytes = current.totalBytes,
                banner = planner.warning,
            )
        }
    }

    private fun decryptOne(
        track: Track,
        cfg: UserSettings,
        planner: OutputPlanner,
        isCancelled: () -> Boolean,
        onProgress: (Long, Long) -> Unit,
    ): Track {
        if (track.plan == KeyPlan.MAC) {
            return track.copy(status = TrackStatus.FAILED, message = "这是 Mac 客户端文件，密钥在电脑的 MMKV 里，手机读不到。")
        }
        return try {
            openSource(app, Uri.parse(track.uri)).use { owned ->
                val source = owned.source
                val probe = probeQmc(source)
                if (probe.kind == FooterKind.STAG && track.plan == KeyPlan.MISSING) {
                    return track.copy(
                        status = TrackStatus.FAILED,
                        message = "这是安卓缓存（STag），文件里没有密钥。请导入 player_process_db，或粘贴这一首的 ekey。",
                    )
                }
                val opened = openCipher(source, probe, track.name, cfg.manualEkey)
                    ?: return track.copy(status = TrackStatus.FAILED, message = failureMessage(probe, track.plan))
                val (cipher, audioLength, kind) = opened
                var ext = preferredExtension(track.name)
                if (cfg.autoExt && kind != AudioKind.UNKNOWN) ext = kind.extension
                val allocation = planner.allocate(outputStem(track.name), ext, mimeFor(kind, ext))
                if (allocation is Allocation.Skip) {
                    return track.copy(status = TrackStatus.SKIPPED, message = "输出已存在", outputName = allocation.name)
                }
                val ready = allocation as Allocation.Ready
                try {
                    ready.open().use { output ->
                        decryptAudio(
                            source = source,
                            output = output,
                            cipher = cipher,
                            audioLength = audioLength,
                            onProgress = { written -> onProgress(written, audioLength) },
                            isCancelled = isCancelled,
                        )
                    }
                    ready.complete()
                } catch (e: Exception) {
                    ready.discard()
                    throw e
                }
                val meta = readMeta(ready, kind)
                track.copy(
                    status = TrackStatus.SUCCESS,
                    message = "",
                    outputName = ready.name,
                    outputUri = ready.uri,
                    outputPath = ready.path,
                    audioLength = audioLength,
                    title = meta.title,
                    artist = meta.artist,
                    album = meta.album,
                    cover = meta.cover?.let { shrinkCover(it) },
                )
            }
        } catch (_: CancelledException) {
            track.copy(status = TrackStatus.FAILED, message = "已取消")
        } catch (e: Exception) {
            track.copy(status = TrackStatus.FAILED, message = e.message ?: "解密失败")
        }
    }

    private fun readMeta(ready: Allocation.Ready, kind: AudioKind): AudioMeta {
        if (kind == AudioKind.UNKNOWN) return AudioMeta()
        return try {
            when {
                ready.path != null -> fileSource(File(ready.path)).use {
                    readAudioMeta(it.source, kind)
                }
                ready.uri != null -> openSource(app, Uri.parse(ready.uri)).use {
                    readAudioMeta(it.source, kind)
                }
                else -> AudioMeta()
            }
        } catch (_: Exception) {
            AudioMeta()
        }
    }

    private data class Opened(val cipher: QmcCipher, val audioLength: Long, val kind: AudioKind)

    private fun openCipher(source: ByteSource, probe: QmcProbe, fileName: String, manual: String): Opened? {
        probe.embeddedEKey?.let { ekey ->
            cipherOf(ekey)?.let { cipher ->
                val kind = headerKind(source, cipher, probe.audioLength)
                if (kind != null) return Opened(cipher, probe.audioLength, kind)
            }
        }
        if (probe.kind != FooterKind.MAC_EX) {
            val external = keys.lookup(fileName, probe.stag?.mediaMid) ?: manual.trim().ifBlank { null }
            if (external != null) {
                cipherOf(external)?.let { cipher ->
                    val length = if (probe.kind == FooterKind.STATIC) source.size else probe.audioLength
                    val kind = headerKind(source, cipher, length)
                    if (kind != null) return Opened(cipher, length, kind)
                }
            }
        }
        if (probe.kind == FooterKind.STATIC || probe.kind == FooterKind.EMBEDDED) {
            val cipher = qmcCipher(ByteArray(0))
            val kind = headerKind(source, cipher, source.size)
            if (kind != null) return Opened(cipher, source.size, kind)
        }
        return null
    }

    private fun cipherOf(ekey: String): QmcCipher? {
        return try {
            qmcCipher(deriveEKey(ekey))
        } catch (_: BadKeyException) {
            null
        }
    }

    private fun headerKind(source: ByteSource, cipher: QmcCipher, audioLength: Long): AudioKind? {
        if (audioLength <= 0) return null
        val kind = sniffAudio(decryptPrefix(source, cipher, audioLength))
        return kind.takeIf { it != AudioKind.UNKNOWN }
    }

    private fun failureMessage(probe: QmcProbe, plan: KeyPlan): String = when {
        probe.kind == FooterKind.STAG || plan == KeyPlan.MISSING ->
            "没有对上密钥。安卓缓存需要 player_process_db，或这一首的 ekey。"
        probe.kind == FooterKind.MAC_EX -> "Mac 客户端的密钥不在文件里。"
        probe.embeddedEKey != null -> "文件里的密钥解不开，结果也不是可识别的音频。"
        else -> "解密结果不是可识别的音频。文件可能已损坏，或不是 QQ 音乐的加密格式。"
    }

    private suspend fun inspect(uri: Uri, id: String, manual: String): Track {
        val name = queryName(app, uri) ?: baseName(uri.toString())
        if (!isQmcFileName(name)) {
            return Track(
                id = id,
                uri = uri.toString(),
                name = name,
                size = 0,
                status = TrackStatus.FAILED,
                plan = KeyPlan.MISSING,
                planLabel = "不支持",
                message = "只处理 .mflac / .mgg，以及同系列的 .qmc 文件",
            )
        }
        return try {
            openSource(app, uri).use { owned ->
                val source = owned.source
                val probe = probeQmc(source)
                val plan = when (probe.kind) {
                    FooterKind.MAC_EX -> KeyPlan.MAC
                    FooterKind.EMBEDDED, FooterKind.QTAG ->
                        if (probe.embeddedEKey != null) KeyPlan.EMBEDDED else KeyPlan.MISSING
                    FooterKind.STAG -> when {
                        keys.lookup(name, probe.stag?.mediaMid) != null -> KeyPlan.DATABASE
                        manual.isNotBlank() -> KeyPlan.MANUAL
                        else -> KeyPlan.MISSING
                    }
                    FooterKind.STATIC -> when {
                        keys.lookup(name, null) != null -> KeyPlan.DATABASE
                        manual.isNotBlank() -> KeyPlan.MANUAL
                        else -> KeyPlan.STATIC
                    }
                }
                Track(
                    id = id,
                    uri = uri.toString(),
                    name = name,
                    size = source.size,
                    audioLength = probe.audioLength,
                    plan = plan,
                    planLabel = planLabel(plan),
                    message = if (plan == KeyPlan.MISSING) "解密前需要密钥" else "",
                )
            }
        } catch (e: Exception) {
            Track(
                id = id,
                uri = uri.toString(),
                name = name,
                size = 0,
                status = TrackStatus.FAILED,
                plan = KeyPlan.MISSING,
                planLabel = "无法读取",
                message = e.message ?: "无法读取文件",
            )
        }
    }

    private fun updateTrack(id: String, block: (Track) -> Track) {
        session.update { current ->
            current.copy(tracks = current.tracks.map { if (it.id == id) block(it) else it })
        }
    }
}

private fun fileSource(file: File): OwnedSource {
    val stream = FileInputStream(file)
    val channel = stream.channel
    val size = channel.size()
    val source = object : ByteSource {
        override val size: Long get() = size
        override fun read(position: Long, dst: ByteArray, offset: Int, len: Int): Int {
            if (len <= 0) return 0
            val buf = ByteBuffer.wrap(dst, offset, len)
            var total = 0
            var pos = position
            while (buf.hasRemaining()) {
                val n = channel.read(buf, pos)
                if (n <= 0) break
                total += n
                pos += n
            }
            return if (total == 0) -1 else total
        }
    }
    return OwnedSource(source) {
        runCatching { channel.close() }
        runCatching { stream.close() }
    }
}

private fun trackId(uri: Uri): String {
    val digest = MessageDigest.getInstance("SHA-256").digest(uri.toString().toByteArray())
    return digest.take(8).joinToString("") { "%02x".format(it) }
}
