package app.musicdecryptor.engine

import android.content.ContentValues
import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import android.provider.OpenableColumns
import androidx.documentfile.provider.DocumentFile
import app.musicdecryptor.crypto.ArraySource
import app.musicdecryptor.crypto.AudioKind
import app.musicdecryptor.crypto.ByteSource
import java.io.ByteArrayOutputStream
import java.io.Closeable
import java.io.File
import java.io.FileInputStream
import java.io.FileOutputStream
import java.io.OutputStream
import java.nio.ByteBuffer

class OwnedSource(
    val source: ByteSource,
    private val closeable: Closeable,
) : Closeable {
    override fun close() = closeable.close()
}

fun queryName(context: Context, uri: Uri): String? {
    context.contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { cursor ->
        if (cursor.moveToFirst()) {
            val index = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
            if (index >= 0) return cursor.getString(index)
        }
    }
    return uri.lastPathSegment?.substringAfterLast('/')
}

fun openSource(context: Context, uri: Uri): OwnedSource {
    try {
        val pfd = context.contentResolver.openFileDescriptor(uri, "r")
            ?: error("无法打开文件")
        val stream = FileInputStream(pfd.fileDescriptor)
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
        if (size > 0) source.read(0, ByteArray(1), 0, 1)
        return OwnedSource(source) {
            runCatching { channel.close() }
            runCatching { stream.close() }
            runCatching { pfd.close() }
        }
    } catch (_: Exception) {
        val tmp = File.createTempFile("qmc-", ".bin", context.cacheDir)
        context.contentResolver.openInputStream(uri)?.use { input ->
            FileOutputStream(tmp).use { input.copyTo(it) }
        } ?: error("无法读取文件")
        val bytes = tmp.readBytes()
        tmp.delete()
        return OwnedSource(ArraySource(bytes)) {}
    }
}

fun appOutputDir(context: Context): File {
    val base = context.getExternalFilesDir(Environment.DIRECTORY_MUSIC) ?: context.filesDir
    return File(base, "decrypted").apply { mkdirs() }
}

fun shrinkCover(bytes: ByteArray): ByteArray? {
    return try {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        if (bounds.outWidth <= 0 || bounds.outHeight <= 0) return null
        var sample = 1
        while (bounds.outWidth / sample > 480 || bounds.outHeight / sample > 480) sample *= 2
        val bitmap = BitmapFactory.decodeByteArray(
            bytes,
            0,
            bytes.size,
            BitmapFactory.Options().apply { inSampleSize = sample },
        ) ?: return null
        val out = ByteArrayOutputStream()
        bitmap.compress(Bitmap.CompressFormat.JPEG, 82, out)
        bitmap.recycle()
        out.toByteArray()
    } catch (_: Exception) {
        null
    }
}

class OutputPlanner(
    private val context: Context,
    private val settings: UserSettings,
) {
    private val used = HashSet<String>()
    var warning: String? = null
        private set
    private val mode: String
    private var tree: DocumentFile? = null

    init {
        mode = when {
            settings.outputMode == UserSettings.MODE_TREE && settings.treeUri.isNotBlank() -> {
                val root = DocumentFile.fromTreeUri(context, Uri.parse(settings.treeUri))
                if (root == null || !root.canWrite()) {
                    warning = "自选文件夹的授权失效了，这次改存到下载目录"
                    UserSettings.MODE_DOWNLOADS
                } else {
                    tree = root
                    UserSettings.MODE_TREE
                }
            }
            settings.outputMode == UserSettings.MODE_APP -> UserSettings.MODE_APP
            else -> UserSettings.MODE_DOWNLOADS
        }
        if (mode == UserSettings.MODE_DOWNLOADS && Build.VERSION.SDK_INT < 29) {
            warning = "当前系统不能直接写入「下载」，已改存到应用音乐目录"
        }
    }

    fun allocate(stem: String, ext: String, mime: String): Allocation {
        var name = stem + ext
        var n = 2
        while (name.lowercase() in used) {
            name = "$stem ($n)$ext"
            n++
        }
        if (exists(name)) {
            if (!settings.overwrite) {
                used += name.lowercase()
                return Allocation.Skip(name)
            }
            delete(name)
        }
        used += name.lowercase()
        return create(name, mime)
    }

    private fun effectiveMode(): String {
        return if (mode == UserSettings.MODE_DOWNLOADS && Build.VERSION.SDK_INT < 29) {
            UserSettings.MODE_APP
        } else {
            mode
        }
    }

    private fun exists(name: String): Boolean = when (effectiveMode()) {
        UserSettings.MODE_TREE -> tree?.findFile(name) != null
        UserSettings.MODE_APP -> File(appOutputDir(context), name).exists()
        else -> findDownload(name) != null
    }

    private fun delete(name: String) {
        when (effectiveMode()) {
            UserSettings.MODE_TREE -> tree?.findFile(name)?.delete()
            UserSettings.MODE_APP -> File(appOutputDir(context), name).delete()
            else -> findDownload(name)?.let { context.contentResolver.delete(it, null, null) }
        }
    }

    private fun create(name: String, mime: String): Allocation = when (effectiveMode()) {
        UserSettings.MODE_TREE -> createTree(name, mime)
        UserSettings.MODE_APP -> createAppFile(name)
        else -> createDownload(name, mime)
    }

    private fun createAppFile(name: String): Allocation {
        val file = File(appOutputDir(context), name)
        return Allocation.Ready(
            name = name,
            open = { FileOutputStream(file) },
            complete = {},
            discard = { file.delete() },
            uri = null,
            path = file.absolutePath,
        )
    }

    private fun createTree(name: String, mime: String): Allocation {
        val root = tree ?: error("没有可写的输出文件夹")
        val doc = root.createFile(mime, name) ?: error("无法在输出文件夹里创建 $name")
        val uri = doc.uri
        return Allocation.Ready(
            name = doc.name ?: name,
            open = {
                context.contentResolver.openOutputStream(uri) ?: error("无法写入 $name")
            },
            complete = {},
            discard = { doc.delete() },
            uri = uri.toString(),
            path = null,
        )
    }

    private fun createDownload(name: String, mime: String): Allocation {
        val values = ContentValues().apply {
            put(MediaStore.MediaColumns.DISPLAY_NAME, name)
            put(MediaStore.MediaColumns.MIME_TYPE, mime)
            put(MediaStore.MediaColumns.RELATIVE_PATH, DOWNLOAD_RELATIVE)
            put(MediaStore.MediaColumns.IS_PENDING, 1)
        }
        val uri = context.contentResolver.insert(downloadCollection(), values)
            ?: error("无法写入下载目录")
        return Allocation.Ready(
            name = name,
            open = {
                context.contentResolver.openOutputStream(uri) ?: error("无法写入 $name")
            },
            complete = {
                val done = ContentValues().apply { put(MediaStore.MediaColumns.IS_PENDING, 0) }
                context.contentResolver.update(uri, done, null, null)
            },
            discard = { context.contentResolver.delete(uri, null, null) },
            uri = uri.toString(),
            path = null,
        )
    }

    private fun findDownload(name: String): Uri? {
        val projection = arrayOf(MediaStore.MediaColumns._ID)
        val selection = "${MediaStore.MediaColumns.DISPLAY_NAME}=? AND ${MediaStore.MediaColumns.RELATIVE_PATH}=?"
        context.contentResolver.query(
            downloadCollection(),
            projection,
            selection,
            arrayOf(name, DOWNLOAD_RELATIVE),
            null,
        )?.use { cursor ->
            if (cursor.moveToFirst()) {
                val id = cursor.getLong(0)
                return Uri.withAppendedPath(downloadCollection(), id.toString())
            }
        }
        return null
    }

    private fun downloadCollection(): Uri {
        return if (Build.VERSION.SDK_INT >= 29) {
            MediaStore.Downloads.EXTERNAL_CONTENT_URI
        } else {
            MediaStore.Files.getContentUri("external")
        }
    }

    companion object {
        const val DOWNLOAD_RELATIVE = "Download/MusicDecryptor/"
    }
}

sealed interface Allocation {
    data class Skip(val name: String) : Allocation
    data class Ready(
        val name: String,
        val open: () -> OutputStream,
        val complete: () -> Unit,
        val discard: () -> Unit,
        val uri: String?,
        val path: String?,
    ) : Allocation
}

fun mimeFor(kind: AudioKind, extension: String): String = when {
    kind != AudioKind.UNKNOWN -> kind.mime
    extension == ".ogg" -> "audio/ogg"
    extension == ".mp3" -> "audio/mpeg"
    extension == ".m4a" -> "audio/mp4"
    extension == ".wav" -> "audio/wav"
    else -> "audio/flac"
}
