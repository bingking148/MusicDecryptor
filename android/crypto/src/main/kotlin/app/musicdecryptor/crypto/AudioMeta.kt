package app.musicdecryptor.crypto

import java.util.Base64

data class AudioMeta(
    val title: String? = null,
    val artist: String? = null,
    val album: String? = null,
    val cover: ByteArray? = null,
) {
    val isEmpty: Boolean get() = title == null && artist == null && album == null && cover == null
}

fun readAudioMeta(source: ByteSource, kind: AudioKind): AudioMeta {
    if (source.size < 8) return AudioMeta()
    return try {
        when (kind) {
            AudioKind.FLAC -> readFlac(source)
            AudioKind.OGG -> readOgg(source)
            AudioKind.MP3 -> readId3(source)
            AudioKind.M4A -> readMp4(source)
            else -> AudioMeta()
        }
    } catch (_: Exception) {
        AudioMeta()
    }
}

private fun readFlac(source: ByteSource): AudioMeta {
    if (!source.readFully(0, 4).contentEquals("fLaC".encodeToByteArray())) return AudioMeta()
    var pos = 4L
    var title: String? = null
    var artist: String? = null
    var album: String? = null
    var cover: ByteArray? = null
    val limit = minOf(source.size, 12L * 1024 * 1024)
    while (pos + 4 <= limit) {
        val hdr = source.readFully(pos, 4)
        val last = hdr[0].toInt() and 0x80 != 0
        val type = hdr[0].toInt() and 0x7F
        val size = ((hdr[1].toInt() and 0xFF) shl 16) or
            ((hdr[2].toInt() and 0xFF) shl 8) or
            (hdr[3].toInt() and 0xFF)
        val start = pos + 4
        if (size < 0 || start + size > source.size) break
        when (type) {
            4 -> if (size in 1..1_000_000) {
                val fields = parseVorbisComment(source.readFully(start, size), 0)
                title = fields.title
                artist = fields.artist
                album = fields.album
                if (cover == null) cover = fields.cover
            }
            6 -> if (cover == null && size in 32..2_000_000) {
                cover = pictureData(source.readFully(start, size))
            }
        }
        pos = start + size
        if (last || (title != null && cover != null)) break
    }
    return AudioMeta(title, artist, album, cover)
}

private fun readOgg(source: ByteSource): AudioMeta {
    val n = minOf(source.size, 512L * 1024).toInt()
    val data = source.readFully(0, n)
    val packets = ArrayList<ByteArray>()
    val current = java.io.ByteArrayOutputStream()
    var i = 0
    while (i + 27 <= data.size && packets.size < 6) {
        if (data[i] != 'O'.code.toByte() || data[i + 1] != 'g'.code.toByte() ||
            data[i + 2] != 'g'.code.toByte() || data[i + 3] != 'S'.code.toByte()
        ) {
            break
        }
        val segs = data[i + 26].toInt() and 0xFF
        if (i + 27 + segs > data.size) break
        var body = i + 27 + segs
        for (s in 0 until segs) {
            val len = data[i + 27 + s].toInt() and 0xFF
            if (body + len > data.size) return AudioMeta()
            current.write(data, body, len)
            body += len
            if (len < 255) {
                packets += current.toByteArray()
                current.reset()
            }
        }
        i = body
    }
    var title: String? = null
    var artist: String? = null
    var album: String? = null
    var cover: ByteArray? = null
    for (packet in packets) {
        val off = commentOffset(packet) ?: continue
        val fields = parseVorbisComment(packet, off)
        title = fields.title ?: title
        artist = fields.artist ?: artist
        album = fields.album ?: album
        cover = fields.cover ?: cover
    }
    return AudioMeta(title, artist, album, cover)
}

private fun commentOffset(packet: ByteArray): Int? {
    if (packet.size >= 7 && packet[0] == 3.toByte() &&
        packet.copyOfRange(1, 7).contentEquals("vorbis".encodeToByteArray())
    ) {
        return 7
    }
    if (packet.size >= 8 && packet.copyOfRange(0, 8).contentEquals("OpusTags".encodeToByteArray())) return 8
    return null
}

private data class Fields(val title: String?, val artist: String?, val album: String?, val cover: ByteArray?)

private fun parseVorbisComment(buf: ByteArray, offset: Int): Fields {
    fun u32(o: Int): Int = if (o + 4 <= buf.size) le32(buf, o) else -1
    var o = offset
    val vendorLen = u32(o)
    if (vendorLen < 0) return Fields(null, null, null, null)
    o += 4 + vendorLen
    val count = u32(o)
    if (count < 0) return Fields(null, null, null, null)
    o += 4
    var title: String? = null
    var artist: String? = null
    var album: String? = null
    var cover: ByteArray? = null
    for (i in 0 until minOf(count, 256)) {
        val len = u32(o)
        o += 4
        if (len < 0 || o + len > buf.size) break
        val entry = buf.copyOfRange(o, o + len).toString(Charsets.UTF_8)
        o += len
        val eq = entry.indexOf('=')
        if (eq <= 0) continue
        val key = entry.substring(0, eq).uppercase()
        val value = entry.substring(eq + 1).trim().trim('\u0000')
        when (key) {
            "TITLE" -> title = value.ifBlank { null }
            "ARTIST", "ALBUMARTIST" -> if (artist == null) artist = value.ifBlank { null }
            "ALBUM" -> album = value.ifBlank { null }
            "METADATA_BLOCK_PICTURE" -> if (cover == null) cover = decodePictureValue(value)
        }
    }
    return Fields(title, artist, album, cover)
}

private fun decodePictureValue(value: String): ByteArray? {
    return try {
        pictureData(Base64.getDecoder().decode(value))
    } catch (_: Exception) {
        null
    }
}

private fun pictureData(block: ByteArray): ByteArray? {
    var o = 4
    if (block.size < 32) return null
    val mimeLen = be32(block, o)
    o += 4 + mimeLen
    if (mimeLen < 0 || o + 4 > block.size) return null
    val descLen = be32(block, o)
    o += 4 + descLen
    o += 16
    if (descLen < 0 || o + 4 > block.size) return null
    val dataLen = be32(block, o)
    o += 4
    if (dataLen <= 0 || dataLen > 2_000_000 || o + dataLen > block.size) return null
    return block.copyOfRange(o, o + dataLen)
}

private fun readId3(source: ByteSource): AudioMeta {
    val head = source.readFully(0, 10)
    if (!head.copyOfRange(0, 3).contentEquals("ID3".encodeToByteArray())) return AudioMeta()
    val version = head[3].toInt() and 0xFF
    val tagSize = syncsafe(head, 6)
    if (tagSize <= 0 || tagSize > source.size - 10) return AudioMeta()
    val body = source.readFully(10, minOf(tagSize, 512 * 1024))
    val idLen = if (version == 2) 3 else 4
    val headerLen = if (version == 2) 6 else 10
    var o = 0
    var title: String? = null
    var artist: String? = null
    var album: String? = null
    var cover: ByteArray? = null
    while (o + headerLen <= body.size) {
        if (body[o] == 0.toByte()) break
        val id = body.copyOfRange(o, o + idLen).toString(Charsets.ISO_8859_1)
        val frameSize = if (version == 4) syncsafe(body, o + 4) else if (version == 2) {
            ((body[o + 3].toInt() and 0xFF) shl 16) or
                ((body[o + 4].toInt() and 0xFF) shl 8) or
                (body[o + 5].toInt() and 0xFF)
        } else {
            be32(body, o + 4)
        }
        val dataAt = o + headerLen
        if (frameSize <= 0 || dataAt + frameSize > body.size) break
        val frame = body.copyOfRange(dataAt, dataAt + frameSize)
        when (id) {
            "TIT2", "TT2" -> title = decodeTextFrame(frame)
            "TPE1", "TP1" -> artist = decodeTextFrame(frame)
            "TALB", "TAL" -> album = decodeTextFrame(frame)
            "APIC", "PIC" -> if (cover == null) cover = decodeApic(frame, version == 2)
        }
        o = dataAt + frameSize
    }
    return AudioMeta(title, artist, album, cover)
}

private fun syncsafe(b: ByteArray, o: Int): Int {
    return ((b[o].toInt() and 0x7F) shl 21) or
        ((b[o + 1].toInt() and 0x7F) shl 14) or
        ((b[o + 2].toInt() and 0x7F) shl 7) or
        (b[o + 3].toInt() and 0x7F)
}

private fun decodeTextFrame(frame: ByteArray): String? {
    if (frame.isEmpty()) return null
    val text = decodeEncoded(frame[0].toInt() and 0xFF, frame.copyOfRange(1, frame.size))
    return text.trimEnd('\u0000').trim().ifBlank { null }
}

private fun decodeApic(frame: ByteArray, v22: Boolean): ByteArray? {
    if (frame.size < 4) return null
    val enc = frame[0].toInt() and 0xFF
    var o = 1
    if (v22) {
        o += 3
    } else {
        while (o < frame.size && frame[o] != 0.toByte()) o++
        o++
    }
    o++ // picture type
    if (o >= frame.size) return null
    o = skipEncodedCString(frame, o, enc)
    if (o < 0 || o >= frame.size) return null
    val data = frame.copyOfRange(o, frame.size)
    return if (data.size in 32..2_000_000) data else null
}

private fun skipEncodedCString(data: ByteArray, start: Int, encoding: Int): Int {
    if (encoding == 1 || encoding == 2) {
        var i = start
        while (i + 1 < data.size) {
            if (data[i] == 0.toByte() && data[i + 1] == 0.toByte()) return i + 2
            i += 2
        }
        return -1
    }
    var i = start
    while (i < data.size) {
        if (data[i] == 0.toByte()) return i + 1
        i++
    }
    return -1
}

private fun decodeEncoded(encoding: Int, data: ByteArray): String {
    return when (encoding) {
        0 -> data.toString(Charsets.ISO_8859_1)
        1 -> data.toString(Charsets.UTF_16)
        2 -> data.toString(Charsets.UTF_16BE)
        else -> data.toString(Charsets.UTF_8)
    }
}

private fun readMp4(source: ByteSource): AudioMeta {
    var title: String? = null
    var artist: String? = null
    var album: String? = null
    var cover: ByteArray? = null
    val limit = minOf(source.size, 8L * 1024 * 1024)
    fun walk(start: Long, end: Long, depth: Int) {
        if (depth > 8 || start >= end) return
        var p = start
        while (p + 8 <= end) {
            val hdr = source.readFully(p, 8)
            var size = be32Long(hdr, 0)
            val type = hdr.copyOfRange(4, 8).toString(Charsets.ISO_8859_1)
            var header = 8L
            if (size == 1L && p + 16 <= end) {
                val ext = source.readFully(p + 8, 8)
                size = ((be32Long(ext, 0) shl 32) or be32Long(ext, 4))
                header = 16
            } else if (size == 0L) {
                size = end - p
            }
            if (size < header || p + size > end) return
            val childStart = p + header
            val childEnd = p + size
            when (type) {
                "moov", "trak", "mdia", "minf", "stbl", "udta", "ilst" -> walk(childStart, childEnd, depth + 1)
                "meta" -> walk(childStart + 4, childEnd, depth + 1)
                "\u00A9nam" -> title = ilstText(source, childStart, childEnd)
                "\u00A9ART", "aART" -> if (artist == null) artist = ilstText(source, childStart, childEnd)
                "\u00A9alb" -> album = ilstText(source, childStart, childEnd)
                "covr" -> if (cover == null) cover = ilstCover(source, childStart, childEnd)
            }
            p += size
        }
    }
    walk(0, limit, 0)
    return AudioMeta(title, artist, album, cover)
}

private fun ilstText(source: ByteSource, start: Long, end: Long): String? {
    val data = ilstData(source, start, end) ?: return null
    if (data.size <= 8) return null
    return data.copyOfRange(8, data.size).toString(Charsets.UTF_8).trimEnd('\u0000').trim().ifBlank { null }
}

private fun ilstCover(source: ByteSource, start: Long, end: Long): ByteArray? {
    val data = ilstData(source, start, end) ?: return null
    if (data.size <= 8) return null
    val image = data.copyOfRange(8, data.size)
    return if (image.size in 32..2_000_000) image else null
}

private fun ilstData(source: ByteSource, start: Long, end: Long): ByteArray? {
    var p = start
    while (p + 8 <= end) {
        val hdr = source.readFully(p, 8)
        val size = be32Long(hdr, 0)
        val type = hdr.copyOfRange(4, 8).toString(Charsets.ISO_8859_1)
        if (size < 8 || p + size > end) return null
        if (type == "data") return source.readFully(p + 8, (size - 8).toInt())
        p += size
    }
    return null
}
