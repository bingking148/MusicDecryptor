package app.musicdecryptor.crypto

import java.util.Base64

/**
 * 把文件里或数据库里的 ekey 还原成流密码密钥。
 * v1：base64 → 前 8 字节与 simpleMakeKey(106) 交错成 TEA 密钥 → 解密剩余部分。
 * v2：外层还有 "QQMusic EncV2,Key:" 和两轮固定密钥的 TEA。
 */
fun deriveEKey(ekey: String): ByteArray {
    val cleaned = ekey.trim().replace("\n", "").replace("\r", "").replace(" ", "")
    if (cleaned.isEmpty()) throw BadKeyException("ekey 是空的")
    var raw = decodeB64(cleaned)
    val prefix = "QQMusic EncV2,Key:".encodeToByteArray()
    if (raw.size >= prefix.size && raw.copyOfRange(0, prefix.size).contentEquals(prefix)) {
        var body = raw.copyOfRange(prefix.size, raw.size)
        body = QmcTea.decrypt(body, V2_KEY_1)
        body = QmcTea.decrypt(body, V2_KEY_2)
        var end = body.size
        while (end > 0 && body[end - 1] == 0.toByte()) end--
        raw = decodeB64(body.copyOfRange(0, end).toString(Charsets.US_ASCII))
    }
    return deriveV1(raw)
}

private fun deriveV1(raw: ByteArray): ByteArray {
    if (raw.size < 16) throw BadKeyException("ekey 太短")
    val simple = QmcTea.simpleMakeKey(106, 8)
    val teaKey = ByteArray(16)
    for (i in 0 until 8) {
        teaKey[i shl 1] = simple[i]
        teaKey[(i shl 1) + 1] = raw[i]
    }
    val body = QmcTea.decrypt(raw.copyOfRange(8, raw.size), teaKey)
    return raw.copyOfRange(0, 8) + body
}

private fun decodeB64(text: String): ByteArray {
    val pad = (4 - text.length % 4) % 4
    val normalized = text + "=".repeat(pad)
    try {
        return Base64.getDecoder().decode(normalized)
    } catch (e: IllegalArgumentException) {
        throw BadKeyException("ekey 不是有效的 Base64")
    }
}

private val V2_KEY_1 = byteArrayOf(
    0x33, 0x38, 0x36, 0x5A, 0x4A, 0x59, 0x21, 0x40,
    0x23, 0x2A, 0x24, 0x25, 0x5E, 0x26, 0x29, 0x28,
)

private val V2_KEY_2 = byteArrayOf(
    0x2A, 0x2A, 0x23, 0x21, 0x28, 0x23, 0x24, 0x25,
    0x26, 0x5E, 0x61, 0x31, 0x63, 0x5A, 0x2C, 0x54,
)
