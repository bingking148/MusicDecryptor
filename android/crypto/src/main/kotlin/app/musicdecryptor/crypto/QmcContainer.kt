package app.musicdecryptor.crypto

import java.io.OutputStream

enum class FooterKind {
    /** 尾部是 ekey 文本 + 4 字节小端长度。 */
    EMBEDDED,

    /** 尾部 QTag，载荷里带 ekey。 */
    QTAG,

    /** 安卓客户端 STag，只有曲目 id，没有密钥。 */
    STAG,

    /** 没有密钥尾，尝试旧版静态掩码。 */
    STATIC,

    /** Mac 客户端的 cex 尾，密钥在电脑的 MMKV 里。 */
    MAC_EX,
}

data class STagInfo(
    val resourceId: String,
    val mediaMid: String,
    val footerSize: Long,
)

data class QmcProbe(
    val kind: FooterKind,
    val audioLength: Long,
    val embeddedEKey: String? = null,
    val stag: STagInfo? = null,
)

fun probeQmc(source: ByteSource): QmcProbe {
    val size = source.size
    if (size < 16) return QmcProbe(FooterKind.STATIC, size)
    val tail4 = source.readFully(size - 4, 4)
    val marker = tail4.toString(Charsets.ISO_8859_1)
    when (marker) {
        "QTag" -> return readQTag(source, size)
        "STag" -> return readSTag(source, size)
        "cex\u0000" -> return readMusicEx(source, size)
    }
    val keyLen = le32(tail4, 0).toLong() and 0xFFFFFFFFL
    if (keyLen in 1L..0xFFFFL && size > keyLen + 4) {
        val raw = source.readFully(size - 4 - keyLen, keyLen.toInt())
        var end = raw.size
        while (end > 0 && raw[end - 1] == 0.toByte()) end--
        val text = raw.copyOfRange(0, end).toString(Charsets.ISO_8859_1).trim()
        if (looksLikeEKey(text)) {
            return QmcProbe(
                kind = FooterKind.EMBEDDED,
                audioLength = size - 4 - keyLen,
                embeddedEKey = text,
            )
        }
    }
    return QmcProbe(FooterKind.STATIC, size)
}

private fun readMusicEx(source: ByteSource, size: Long): QmcProbe {
    if (size < 16) return QmcProbe(FooterKind.MAC_EX, size)
    val tail = source.readFully(size - 16, 16)
    val tagSize = le32(tail, 0).toLong() and 0xFFFFFFFFL
    val version = le32(tail, 4)
    val magicOk = tail.copyOfRange(8, 16).contentEquals("musicex\u0000".encodeToByteArray())
    if (!magicOk || version != 1 || tagSize < 0xC0 || tagSize > size) {
        return QmcProbe(FooterKind.MAC_EX, size)
    }
    val tag = source.readFully(size - tagSize, tagSize.toInt())
    val fileName = utf16Ascii(tag, 0x48, 50)
    val mediaId = utf16Ascii(tag, 0x0C, 30)
    return QmcProbe(
        kind = FooterKind.MAC_EX,
        audioLength = size - tagSize,
        stag = STagInfo(
            resourceId = "",
            mediaMid = fileName.ifBlank { mediaId },
            footerSize = tagSize,
        ),
    )
}

private fun utf16Ascii(data: ByteArray, offset: Int, chars: Int): String {
    val out = StringBuilder()
    for (i in 0 until chars) {
        val at = offset + i * 2
        if (at >= data.size) break
        val value = data[at].toInt() and 0xFF
        if (value == 0) break
        out.append(value.toChar())
    }
    return out.toString()
}

private fun readQTag(source: ByteSource, size: Long): QmcProbe {
    if (size < 8) throw BadKeyException("QTag 文件太短")
    val len = be32Long(source.readFully(size - 8, 4), 0)
    if (len <= 0 || len > size - 8) throw BadKeyException("QTag 长度不合法")
    val payload = source.readFully(size - 8 - len, len.toInt()).toString(Charsets.UTF_8).trimEnd('\u0000')
    val parts = payload.split(',')
    if (parts.size < 3 || parts[0].isBlank()) throw BadKeyException("QTag 里没有 ekey")
    val ekey = parts.dropLast(2).joinToString(",")
    return QmcProbe(FooterKind.QTAG, size - 8 - len, embeddedEKey = ekey)
}

private fun readSTag(source: ByteSource, size: Long): QmcProbe {
    if (size < 8) return QmcProbe(FooterKind.STAG, size)
    val len = be32Long(source.readFully(size - 8, 4), 0)
    if (len <= 0 || len > 4096 || size < len + 8) {
        return QmcProbe(FooterKind.STAG, size)
    }
    val payload = source.readFully(size - 8 - len, len.toInt()).toString(Charsets.UTF_8).trimEnd('\u0000')
    val parts = payload.split(',')
    val info = if (parts.size >= 3) {
        STagInfo(parts[0], parts[2], len + 8)
    } else {
        STagInfo("", "", len + 8)
    }
    return QmcProbe(FooterKind.STAG, size - info.footerSize, stag = info)
}

private fun looksLikeEKey(text: String): Boolean {
    if (text.length < 16) return false
    return text.all { it.isLetterOrDigit() || it == '+' || it == '/' || it == '=' || it == '-' || it == '_' }
}

enum class AudioKind(val extension: String, val mime: String) {
    FLAC(".flac", "audio/flac"),
    OGG(".ogg", "audio/ogg"),
    MP3(".mp3", "audio/mpeg"),
    M4A(".m4a", "audio/mp4"),
    WAV(".wav", "audio/wav"),
    UNKNOWN("", "application/octet-stream"),
}

fun sniffAudio(header: ByteArray): AudioKind {
    if (header.size >= 4 && header.copyOfRange(0, 4).contentEquals("fLaC".encodeToByteArray())) return AudioKind.FLAC
    if (header.size >= 4 && header.copyOfRange(0, 4).contentEquals("OggS".encodeToByteArray())) return AudioKind.OGG
    if (header.size >= 3 && header.copyOfRange(0, 3).contentEquals("ID3".encodeToByteArray())) return AudioKind.MP3
    if (header.size >= 4 && header.copyOfRange(0, 4).contentEquals("RIFF".encodeToByteArray())) return AudioKind.WAV
    if (header.size >= 12 && header.copyOfRange(4, 8).contentEquals("ftyp".encodeToByteArray())) return AudioKind.M4A
    if (header.size >= 2 && header[0] == 0xFF.toByte() && (header[1].toInt() and 0xE0) == 0xE0) return AudioKind.MP3
    return AudioKind.UNKNOWN
}

fun decryptPrefix(source: ByteSource, cipher: QmcCipher, audioLength: Long, length: Int = 64): ByteArray {
    val n = minOf(length.toLong(), audioLength).toInt()
    if (n <= 0) return ByteArray(0)
    val buf = source.readFully(0, n)
    cipher.decrypt(buf, 0, buf.size, 0)
    return buf
}

fun decryptAudio(
    source: ByteSource,
    output: OutputStream,
    cipher: QmcCipher,
    audioLength: Long,
    onProgress: (Long) -> Unit = {},
    isCancelled: () -> Boolean = { false },
) {
    val buf = ByteArray(256 * 1024)
    var offset = 0L
    while (offset < audioLength) {
        if (isCancelled()) throw CancelledException()
        val want = minOf(buf.size.toLong(), audioLength - offset).toInt()
        val got = source.read(offset, buf, 0, want)
        if (got <= 0) throw java.io.EOFException("文件在 $offset 处被截断")
        cipher.decrypt(buf, 0, got, offset)
        output.write(buf, 0, got)
        offset += got
        onProgress(offset)
    }
    output.flush()
}

class CancelledException : Exception("已取消")
