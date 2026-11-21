package app.musicdecryptor.crypto

/**
 * QMCv2 流密码。密钥短于等于 300 字节用 map，更长用分段 RC4，空密钥是旧版静态掩码。
 * 解密是对密文字节异或，同一个偏移调用两次会还原。
 */
interface QmcCipher {
    fun decrypt(data: ByteArray, index: Int, length: Int, fileOffset: Long)
}

fun qmcCipher(decodedKey: ByteArray): QmcCipher {
    return when {
        decodedKey.size > 300 -> Rc4Cipher(decodedKey)
        decodedKey.isNotEmpty() -> MapCipher(decodedKey)
        else -> StaticCipher
    }
}

private object StaticCipher : QmcCipher {
    override fun decrypt(data: ByteArray, index: Int, length: Int, fileOffset: Long) {
        for (i in 0 until length) {
            data[index + i] = (data[index + i].toInt() xor mask(fileOffset + i)).toByte()
        }
    }

    private fun mask(offset: Long): Int {
        var off = offset
        if (off > 0x7FFF) off %= 0x7FFF
        val idx = ((off * off + 27) and 0xFF).toInt()
        return BOX[idx].toInt() and 0xFF
    }

    private val BOX = byteArrayOf(
        0x77, 0x48, 0x32, 0x73, 0xDE.toByte(), 0xF2.toByte(), 0xC0.toByte(), 0xC8.toByte(),
        0x95.toByte(), 0xEC.toByte(), 0x30, 0xB2.toByte(), 0x51, 0xC3.toByte(), 0xE1.toByte(), 0xA0.toByte(),
        0x9E.toByte(), 0xE6.toByte(), 0x9D.toByte(), 0xCF.toByte(), 0xFA.toByte(), 0x7F, 0x14, 0xD1.toByte(),
        0xCE.toByte(), 0xB8.toByte(), 0xDC.toByte(), 0xC3.toByte(), 0x4A, 0x67, 0x93.toByte(), 0xD6.toByte(),
        0x28, 0xC2.toByte(), 0x91.toByte(), 0x70, 0xCA.toByte(), 0x8D.toByte(), 0xA2.toByte(), 0xA4.toByte(),
        0xF0.toByte(), 0x08, 0x61, 0x90.toByte(), 0x7E, 0x6F, 0xA2.toByte(), 0xE0.toByte(),
        0xEB.toByte(), 0xAE.toByte(), 0x3E, 0xB6.toByte(), 0x67, 0xC7.toByte(), 0x92.toByte(), 0xF4.toByte(),
        0x91.toByte(), 0xB5.toByte(), 0xF6.toByte(), 0x6C, 0x5E, 0x84.toByte(), 0x40, 0xF7.toByte(),
        0xF3.toByte(), 0x1B, 0x02, 0x7F, 0xD5.toByte(), 0xAB.toByte(), 0x41, 0x89.toByte(),
        0x28, 0xF4.toByte(), 0x25, 0xCC.toByte(), 0x52, 0x11, 0xAD.toByte(), 0x43,
        0x68, 0xA6.toByte(), 0x41, 0x8B.toByte(), 0x84.toByte(), 0xB5.toByte(), 0xFF.toByte(), 0x2C,
        0x92.toByte(), 0x4A, 0x26, 0xD8.toByte(), 0x47, 0x6A, 0x7C, 0x95.toByte(),
        0x61, 0xCC.toByte(), 0xE6.toByte(), 0xCB.toByte(), 0xBB.toByte(), 0x3F, 0x47, 0x58,
        0x89.toByte(), 0x75, 0xC3.toByte(), 0x75, 0xA1.toByte(), 0xD9.toByte(), 0xAF.toByte(), 0xCC.toByte(),
        0x08, 0x73, 0x17, 0xDC.toByte(), 0xAA.toByte(), 0x9A.toByte(), 0xA2.toByte(), 0x16,
        0x41, 0xD8.toByte(), 0xA2.toByte(), 0x06, 0xC6.toByte(), 0x8B.toByte(), 0xFC.toByte(), 0x66,
        0x34, 0x9F.toByte(), 0xCF.toByte(), 0x18, 0x23, 0xA0.toByte(), 0x0A, 0x74,
        0xE7.toByte(), 0x2B, 0x27, 0x70, 0x92.toByte(), 0xE9.toByte(), 0xAF.toByte(), 0x37,
        0xE6.toByte(), 0x8C.toByte(), 0xA7.toByte(), 0xBC.toByte(), 0x62, 0x65, 0x9C.toByte(), 0xC2.toByte(),
        0x08, 0xC9.toByte(), 0x88.toByte(), 0xB3.toByte(), 0xF3.toByte(), 0x43, 0xAC.toByte(), 0x74,
        0x2C, 0x0F, 0xD4.toByte(), 0xAF.toByte(), 0xA1.toByte(), 0xC3.toByte(), 0x01, 0x64,
        0x95.toByte(), 0x4E, 0x48, 0x9F.toByte(), 0xF4.toByte(), 0x35, 0x78, 0x95.toByte(),
        0x7A, 0x39, 0xD6.toByte(), 0x6A, 0xA0.toByte(), 0x6D, 0x40, 0xE8.toByte(),
        0x4F, 0xA8.toByte(), 0xEF.toByte(), 0x11, 0x1D, 0xF3.toByte(), 0x1B, 0x3F,
        0x3F, 0x07, 0xDD.toByte(), 0x6F, 0x5B, 0x19, 0x30, 0x19,
        0xFB.toByte(), 0xEF.toByte(), 0x0E, 0x37, 0xF0.toByte(), 0x0E, 0xCD.toByte(), 0x16,
        0x49, 0xFE.toByte(), 0x53, 0x47, 0x13, 0x1A, 0xBD.toByte(), 0xA4.toByte(),
        0xF1.toByte(), 0x40, 0x19, 0x60, 0x0E, 0xED.toByte(), 0x68, 0x09,
        0x06, 0x5F, 0x4D, 0xCF.toByte(), 0x3D, 0x1A, 0xFE.toByte(), 0x20,
        0x77, 0xE4.toByte(), 0xD9.toByte(), 0xDA.toByte(), 0xF9.toByte(), 0xA4.toByte(), 0x2B, 0x76,
        0x1C, 0x71, 0xDB.toByte(), 0x00, 0xBC.toByte(), 0xFD.toByte(), 0x0C, 0x6C,
        0xA5.toByte(), 0x47, 0xF7.toByte(), 0xF6.toByte(), 0x00, 0x79, 0x4A, 0x11,
    )
}

private class MapCipher(private val key: ByteArray) : QmcCipher {
    override fun decrypt(data: ByteArray, index: Int, length: Int, fileOffset: Long) {
        val n = key.size
        for (i in 0 until length) {
            var off = fileOffset + i
            if (off > 0x7FFF) off %= 0x7FFF
            val idx = ((off * off + 71214) % n).toInt()
            val mask = rotate(key[idx].toInt() and 0xFF, idx and 7)
            data[index + i] = (data[index + i].toInt() xor mask).toByte()
        }
    }

    private fun rotate(value: Int, bits: Int): Int {
        val rot = (bits + 4) and 7
        return ((value shl rot) or (value ushr rot)) and 0xFF
    }
}

private class Rc4Cipher(private val key: ByteArray) : QmcCipher {
    private val n = key.size
    private val box: ByteArray
    private val hash: Long

    init {
        val s = ByteArray(n)
        for (i in 0 until n) s[i] = i.toByte()
        var j = 0
        for (i in 0 until n) {
            j = (j + (s[i].toInt() and 0xFF) + (key[i].toInt() and 0xFF)) % n
            val tmp = s[i]
            s[i] = s[j]
            s[j] = tmp
        }
        box = s
        var h = 1L
        for (b in key) {
            val v = b.toLong() and 0xFF
            if (v == 0L) continue
            val next = (h * v) and 0xFFFFFFFFL
            if (next == 0L || next <= h) break
            h = next
        }
        hash = h
    }

    override fun decrypt(data: ByteArray, index: Int, length: Int, fileOffset: Long) {
        var offset = fileOffset
        var toProcess = length
        var processed = 0
        fun mark(p: Int): Boolean {
            offset += p
            toProcess -= p
            processed += p
            return toProcess == 0
        }
        if (offset < FIRST) {
            var block = toProcess
            val room = (FIRST - offset).toInt()
            if (block > room) block = room
            for (i in 0 until block) {
                val skip = segmentSkip(offset + i)
                data[index + processed + i] =
                    (data[index + processed + i].toInt() xor (key[skip].toInt() and 0xFF)).toByte()
            }
            if (mark(block)) return
        }
        if (offset % SEGMENT != 0L) {
            var block = toProcess
            val room = (SEGMENT - offset % SEGMENT).toInt()
            if (block > room) block = room
            decryptSegment(data, index + processed, block, offset)
            if (mark(block)) return
        }
        while (toProcess > SEGMENT) {
            decryptSegment(data, index + processed, SEGMENT.toInt(), offset)
            mark(SEGMENT.toInt())
        }
        if (toProcess > 0) decryptSegment(data, index + processed, toProcess, offset)
    }

    private fun decryptSegment(data: ByteArray, index: Int, length: Int, offset: Long) {
        val b = box.copyOf()
        val skip = (offset % SEGMENT).toInt() + segmentSkip(offset / SEGMENT)
        var j = 0
        var k = 0
        var i = -skip
        while (i < length) {
            j = (j + 1) % n
            k = ((b[j].toInt() and 0xFF) + k) % n
            val tmp = b[j]
            b[j] = b[k]
            b[k] = tmp
            if (i >= 0) {
                val s = ((b[j].toInt() and 0xFF) + (b[k].toInt() and 0xFF)) % n
                data[index + i] = (data[index + i].toInt() xor (b[s].toInt() and 0xFF)).toByte()
            }
            i++
        }
    }

    private fun segmentSkip(id: Long): Int {
        val seed = key[(id % n).toInt()].toInt() and 0xFF
        if (seed == 0) return 0
        val prod = (id + 1) * seed.toLong()
        val idx = (hash.toDouble() / prod.toDouble() * 100.0).toLong()
        return (idx % n).toInt()
    }

    companion object {
        private const val SEGMENT = 5120L
        private const val FIRST = 128L
    }
}
