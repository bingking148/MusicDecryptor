package app.musicdecryptor.crypto

import kotlin.math.abs
import kotlin.math.tan

/**
 * QQ 音乐 ekey 使用的 TEA。
 *
 * 分组运算与 golang.org/x/crypto/tea 的 NewCipherWithRounds(key, 32) 一致：
 * 16 个双轮，sum 从 delta*16 递减。外层是 Tencent oi_symmetry_decrypt2。
 */
internal object QmcTea {
    private const val DELTA = 0x9E3779B9L
    private const val ROUNDS = 16

    fun simpleMakeKey(salt: Int, length: Int): ByteArray {
        val out = ByteArray(length)
        for (i in 0 until length) {
            val value = abs(tan(salt.toDouble() + i * 0.1)) * 100.0
            out[i] = value.toInt().toByte()
        }
        return out
    }

    fun decryptBlock(src: ByteArray, srcOff: Int, key: ByteArray, dst: ByteArray, dstOff: Int) {
        var v0 = be32(src, srcOff)
        var v1 = be32(src, srcOff + 4)
        val k0 = be32(key, 0)
        val k1 = be32(key, 4)
        val k2 = be32(key, 8)
        val k3 = be32(key, 12)
        var sum = (DELTA * ROUNDS).toInt()
        repeat(ROUNDS) {
            v1 -= (((v0 shl 4) + k2) xor (v0 + sum) xor ((v0 ushr 5) + k3))
            v0 -= (((v1 shl 4) + k0) xor (v1 + sum) xor ((v1 ushr 5) + k1))
            sum -= DELTA.toInt()
        }
        putBe32(dst, dstOff, v0)
        putBe32(dst, dstOff + 4, v1)
    }

    /**
     * oi_symmetry_decrypt2。明文布局：
     * PadLen(1) + Padding + Salt(2) + Body + Zero(7)。
     */
    fun decrypt(cipher: ByteArray, key: ByteArray): ByteArray {
        if (cipher.size % 8 != 0 || cipher.size < 16) {
            throw BadKeyException("密钥块长度不合法")
        }
        if (key.size != 16) throw BadKeyException("TEA 密钥长度不是 16 字节")
        val dest = ByteArray(8)
        decryptBlock(cipher, 0, key, dest, 0)
        val padLen = dest[0].toInt() and 0x7
        val outLen = cipher.size - 1 - padLen - 2 - 7
        if (outLen < 0) throw BadKeyException("密钥填充长度不合法")
        val out = ByteArray(outLen)
        val ivPrev = ByteArray(8)
        var inPos = 8
        var destIdx = 1 + padLen

        fun cryptBlock() {
            if (inPos + 8 > cipher.size) throw BadKeyException("密钥数据被截断")
            cipher.copyInto(ivPrev, 0, inPos - 8, inPos)
            for (j in 0 until 8) {
                dest[j] = (dest[j].toInt() xor cipher[inPos + j].toInt()).toByte()
            }
            decryptBlock(dest, 0, key, dest, 0)
            inPos += 8
            destIdx = 0
        }

        var salt = 1
        while (salt <= 2) {
            if (destIdx < 8) {
                destIdx++
                salt++
            } else {
                cryptBlock()
            }
        }
        var n = 0
        while (n < outLen) {
            if (destIdx < 8) {
                out[n] = (dest[destIdx].toInt() xor ivPrev[destIdx].toInt()).toByte()
                destIdx++
                n++
            } else {
                cryptBlock()
            }
        }
        var zero = 1
        while (zero <= 7) {
            if (destIdx < 8) {
                if (dest[destIdx] != ivPrev[destIdx]) throw BadKeyException("密钥校验失败")
                destIdx++
                zero++
            } else {
                cryptBlock()
            }
        }
        return out
    }

    private fun putBe32(dst: ByteArray, off: Int, value: Int) {
        dst[off] = (value ushr 24).toByte()
        dst[off + 1] = (value ushr 16).toByte()
        dst[off + 2] = (value ushr 8).toByte()
        dst[off + 3] = value.toByte()
    }
}

class BadKeyException(message: String) : Exception(message)
