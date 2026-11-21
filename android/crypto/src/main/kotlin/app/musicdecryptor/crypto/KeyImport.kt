package app.musicdecryptor.crypto

/**
 * 读取 QQ 音乐明文 MMKV（常见文件名 MMKVStreamEncryptId）。
 * 布局与 unlock-music 的解析器一致：小端长度、跳过一个 varint，然后是
 * key 字符串和「容器长度 + 字符串」的 value。后写覆盖先写，空 value 表示删除。
 * 加密的 AES-CFB vault 解不开，会返回 null。
 */
fun parseMmkvEkeys(data: ByteArray): List<Pair<String, String>>? {
    if (data.size < 8) return null
    val payloadLength = le32(data, 0)
    if (payloadLength <= 0 || payloadLength > data.size - 4) return null
    val end = 4 + payloadLength
    var offset = 4
    offset = skipVarint(data, offset, end) ?: return null
    val map = linkedMapOf<String, String>()
    while (offset < end) {
        if (data[offset].toInt() == 0) break
        val key = readMmkvString(data, offset, end) ?: return if (map.isEmpty()) null else map.toList()
        offset = key.second
        if (offset >= end) break
        val value = readMmkvVariant(data, offset, end) ?: return if (map.isEmpty()) null else map.toList()
        offset = value.third
        val text = value.second
        if (text == null) map.remove(key.first) else map[key.first] = text
    }
    return map.entries.map { it.key to it.value }.ifEmpty { null }
}

fun parseJsonEkeys(text: String): List<Pair<String, String>>? {
    val trimmed = text.trim()
    if (!trimmed.startsWith("{")) return null
    val found = Regex(""""((?:\\.|[^"\\])*)"\s*:\s*"((?:\\.|[^"\\])*)"""").findAll(trimmed).map { match ->
        unescapeJson(match.groupValues[1]) to unescapeJson(match.groupValues[2])
    }.filter { it.first.isNotBlank() && it.second.isNotBlank() }.toList()
    return found.ifEmpty { null }
}

private fun skipVarint(data: ByteArray, start: Int, end: Int): Int? {
    var offset = start
    var guard = 0
    while (offset < end && guard++ < 10) {
        val b = data[offset].toInt() and 0xFF
        offset++
        if (b and 0x80 == 0) return offset
    }
    return null
}

private fun readVarint(data: ByteArray, start: Int, end: Int): Pair<Int, Int>? {
    var offset = start
    var value = 0
    var shift = 0
    var guard = 0
    while (offset < end && guard++ < 10) {
        val b = data[offset].toInt() and 0xFF
        offset++
        value = value or ((b and 0x7F) shl shift)
        if (b and 0x80 == 0) return value to offset
        shift += 7
        if (shift > 28) return null
    }
    return null
}

private fun readMmkvString(data: ByteArray, start: Int, end: Int): Pair<String, Int>? {
    val (len, offset) = readVarint(data, start, end) ?: return null
    if (len < 0 || offset + len > end) return null
    val text = data.copyOfRange(offset, offset + len).toString(Charsets.UTF_8)
    return text to offset + len
}

private fun readMmkvVariant(data: ByteArray, start: Int, end: Int): Triple<Int, String?, Int>? {
    val (container, offset) = readVarint(data, start, end) ?: return null
    if (container == 0) return Triple(0, null, offset)
    if (container < 0 || offset + container > end) return null
    val inner = readMmkvString(data, offset, offset + container) ?: return null
    if (inner.second != offset + container) return null
    return Triple(container, inner.first, offset + container)
}

private fun unescapeJson(value: String): String {
    val out = StringBuilder()
    var i = 0
    while (i < value.length) {
        val ch = value[i]
        if (ch == '\\' && i + 1 < value.length) {
            out.append(value[i + 1])
            i += 2
        } else {
            out.append(ch)
            i++
        }
    }
    return out.toString()
}
