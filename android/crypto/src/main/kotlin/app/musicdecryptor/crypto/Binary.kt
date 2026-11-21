package app.musicdecryptor.crypto

import java.io.EOFException

/** Random-access view of an encrypted or decrypted file. */
interface ByteSource {
    val size: Long
    fun read(position: Long, dst: ByteArray, offset: Int, len: Int): Int
}

fun ByteSource.readFully(position: Long, len: Int): ByteArray {
    if (len < 0 || position < 0 || position + len > size) {
        throw EOFException("读取越界 position=$position len=$len size=$size")
    }
    val dst = ByteArray(len)
    var got = 0
    while (got < len) {
        val n = read(position + got, dst, got, len - got)
        if (n <= 0) throw EOFException("文件在 ${position + got} 处被截断")
        got += n
    }
    return dst
}

class ArraySource(private val data: ByteArray) : ByteSource {
    override val size: Long get() = data.size.toLong()
    override fun read(position: Long, dst: ByteArray, offset: Int, len: Int): Int {
        if (position >= data.size || len <= 0) return -1
        val start = position.toInt()
        val n = minOf(len, data.size - start)
        data.copyInto(dst, offset, start, start + n)
        return n
    }
}

internal fun be32(b: ByteArray, o: Int): Int {
    return ((b[o].toInt() and 0xFF) shl 24) or
        ((b[o + 1].toInt() and 0xFF) shl 16) or
        ((b[o + 2].toInt() and 0xFF) shl 8) or
        (b[o + 3].toInt() and 0xFF)
}

internal fun le32(b: ByteArray, o: Int): Int {
    return (b[o].toInt() and 0xFF) or
        ((b[o + 1].toInt() and 0xFF) shl 8) or
        ((b[o + 2].toInt() and 0xFF) shl 16) or
        ((b[o + 3].toInt() and 0xFF) shl 24)
}

internal fun be32Long(b: ByteArray, o: Int): Long = be32(b, o).toLong() and 0xFFFFFFFFL

internal fun hexToBytes(hex: String): ByteArray {
    val out = ByteArray(hex.length / 2)
    for (i in out.indices) {
        out[i] = hex.substring(i * 2, i * 2 + 2).toInt(16).toByte()
    }
    return out
}

internal fun ByteArray.toHex(): String = joinToString("") { "%02x".format(it.toInt() and 0xFF) }
