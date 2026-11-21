package app.musicdecryptor.crypto

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.test.assertNotNull

class QmcSelfTestTest {
    @Test
    fun mmkvVaultRoundTripAndDelete() {
        val vault = mmkvVault(
            "song.mflac" to "QUJDRA==",
            "song.mflac" to "NEWERKEY",
            "gone.mgg" to "",
        )
        val parsed = parseMmkvEkeys(vault)
        assertNotNull(parsed)
        assertEquals(listOf("song.mflac" to "NEWERKEY"), parsed)
        assertEquals(listOf("a.mflac" to "abc"), parseJsonEkeys("""{"a.mflac":"abc"}"""))
    }

    @Test
    fun musicExExposesFileNameWithoutKey() {
        val tag = ByteArray(0xC0)
        "Hello.mflac".forEachIndexed { index, ch ->
            tag[0x48 + index * 2] = ch.code.toByte()
        }
        val size = 0xC0
        tag[size - 16] = size.toByte()
        tag[size - 12] = 1
        "musicex\u0000".encodeToByteArray().copyInto(tag, size - 8)
        val audio = "fLaC-body".encodeToByteArray()
        val file = audio + tag
        val probe = probeQmc(ArraySource(file))
        assertEquals(FooterKind.MAC_EX, probe.kind)
        assertEquals(audio.size.toLong(), probe.audioLength)
        assertEquals("Hello.mflac", probe.stag?.mediaMid)
        assertEquals(null, probe.embeddedEKey)
    }
    @Test
    fun vectorsMatchGoOracle() {
        val failures = QmcSelfTest.run()
        assertTrue(failures.isEmpty(), failures.joinToString())
    }

    @Test
    fun flacCommentAndPicture() {
        val comment = vorbisComment(
            "TITLE" to "夜航",
            "ARTIST" to "测试",
            "ALBUM" to "黑胶",
        )
        val png = smallestPng()
        val picture = flacPicture(png)
        val file = ByteArray(4 + 4 + comment.size + 4 + picture.size)
        "fLaC".encodeToByteArray().copyInto(file)
        var o = 4
        o = writeBlock(file, o, type = 4, last = false, comment)
        writeBlock(file, o, type = 6, last = true, picture)
        val meta = readAudioMeta(ArraySource(file), AudioKind.FLAC)
        assertEquals("夜航", meta.title)
        assertEquals("测试", meta.artist)
        assertEquals("黑胶", meta.album)
        assertTrue(meta.cover!!.contentEquals(png))
    }

    @Test
    fun id3Title() {
        val title = id3TextFrame("TIT2", "夜航")
        val artist = id3TextFrame("TPE1", "测试")
        val frames = title + artist
        val tag = ByteArray(10 + frames.size)
        "ID3".encodeToByteArray().copyInto(tag)
        tag[3] = 3
        putSyncsafe(tag, 6, frames.size)
        frames.copyInto(tag, 10)
        val meta = readAudioMeta(ArraySource(tag + "extra".encodeToByteArray()), AudioKind.MP3)
        assertEquals("夜航", meta.title)
        assertEquals("测试", meta.artist)
        assertNull(meta.album)
    }
}

private fun mmkvVault(vararg pairs: Pair<String, String>): ByteArray {
    val body = ArrayList<Byte>()
    body += 0
    for ((key, value) in pairs) {
        writeMmkvString(body, key)
        if (value.isEmpty()) {
            body += 0
        } else {
            val inner = ArrayList<Byte>()
            writeMmkvString(inner, value)
            writeVarint(body, inner.size)
            body.addAll(inner)
        }
    }
    val out = ByteArray(4 + body.size)
    out[0] = body.size.toByte()
    body.forEachIndexed { index, byte -> out[4 + index] = byte }
    return out
}

private fun writeMmkvString(out: MutableList<Byte>, text: String) {
    val bytes = text.encodeToByteArray()
    writeVarint(out, bytes.size)
    bytes.forEach { out += it }
}

private fun writeVarint(out: MutableList<Byte>, value: Int) {
    var rest = value
    while (rest >= 0x80) {
        out += ((rest and 0x7F) or 0x80).toByte()
        rest = rest ushr 7
    }
    out += rest.toByte()
}

private fun vorbisComment(vararg pairs: Pair<String, String>): ByteArray {
    val vendor = "qmc".encodeToByteArray()
    val entries = pairs.map { (k, v) -> "$k=$v".encodeToByteArray() }
    val size = 4 + vendor.size + 4 + entries.sumOf { 4 + it.size }
    val out = ByteArray(size)
    putLe32(out, 0, vendor.size)
    vendor.copyInto(out, 4)
    var o = 4 + vendor.size
    putLe32(out, o, entries.size)
    o += 4
    for (entry in entries) {
        putLe32(out, o, entry.size)
        entry.copyInto(out, o + 4)
        o += 4 + entry.size
    }
    return out
}

private fun flacPicture(image: ByteArray): ByteArray {
    val mime = "image/png".encodeToByteArray()
    val out = ByteArray(4 + 4 + mime.size + 4 + 16 + 4 + image.size)
    var o = 4
    putBe32(out, o, mime.size)
    o += 4
    mime.copyInto(out, o)
    o += mime.size
    putBe32(out, o, 0)
    o += 4 + 16
    putBe32(out, o, image.size)
    image.copyInto(out, o + 4)
    return out
}

private fun writeBlock(dst: ByteArray, offset: Int, type: Int, last: Boolean, payload: ByteArray): Int {
    dst[offset] = (type or if (last) 0x80 else 0).toByte()
    dst[offset + 1] = (payload.size shr 16).toByte()
    dst[offset + 2] = (payload.size shr 8).toByte()
    dst[offset + 3] = payload.size.toByte()
    payload.copyInto(dst, offset + 4)
    return offset + 4 + payload.size
}

private fun id3TextFrame(id: String, text: String): ByteArray {
    val body = byteArrayOf(3) + text.encodeToByteArray()
    val frame = ByteArray(10 + body.size)
    id.encodeToByteArray().copyInto(frame)
    putBe32(frame, 4, body.size)
    body.copyInto(frame, 10)
    return frame
}

private fun putSyncsafe(dst: ByteArray, offset: Int, value: Int) {
    dst[offset] = ((value shr 21) and 0x7F).toByte()
    dst[offset + 1] = ((value shr 14) and 0x7F).toByte()
    dst[offset + 2] = ((value shr 7) and 0x7F).toByte()
    dst[offset + 3] = (value and 0x7F).toByte()
}

private fun putLe32(dst: ByteArray, offset: Int, value: Int) {
    dst[offset] = value.toByte()
    dst[offset + 1] = (value shr 8).toByte()
    dst[offset + 2] = (value shr 16).toByte()
    dst[offset + 3] = (value shr 24).toByte()
}

private fun putBe32(dst: ByteArray, offset: Int, value: Int) {
    dst[offset] = (value shr 24).toByte()
    dst[offset + 1] = (value shr 16).toByte()
    dst[offset + 2] = (value shr 8).toByte()
    dst[offset + 3] = value.toByte()
}

private fun smallestPng(): ByteArray {
    return hexToBytes(
        "89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c489" +
            "0000000a49444154789c63000100000500010d0a2db40000000049454e44ae426082",
    )
}
