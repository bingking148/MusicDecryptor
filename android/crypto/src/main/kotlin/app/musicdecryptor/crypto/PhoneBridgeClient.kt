package app.musicdecryptor.crypto

import java.io.InputStream
import java.io.OutputStream
import java.net.HttpURLConnection
import java.net.URI
import java.net.URL

/**
 * 手机主动连到用户已经开启的电脑接力。不保存配对令牌，不跟随重定向。
 * 未调用时不会产生任何网络请求。
 */
object PhoneBridgeClient {
    const val MAX_BYTES = 256L * 1024 * 1024

    fun normalizeBaseUrl(raw: String): String {
        var text = raw.trim().trimEnd('/')
        if (text.isEmpty()) throw BridgeException(0, "请填写电脑上显示的地址")
        if (!text.contains("://")) text = "http://$text"
        val uri = try {
            URI(text)
        } catch (e: Exception) {
            throw BridgeException(0, "地址格式不对")
        }
        val scheme = uri.scheme?.lowercase()
        if (scheme != "http" && scheme != "https") throw BridgeException(0, "只接受 http 或 https")
        if (!uri.userInfo.isNullOrBlank()) throw BridgeException(0, "地址里不能带账号密码")
        val host = uri.host ?: throw BridgeException(0, "地址里没有主机")
        if (host == "0.0.0.0") throw BridgeException(0, "请填写电脑的局域网地址，而不是 0.0.0.0")
        val port = if (uri.port > 0) uri.port else if (scheme == "https") 443 else 80
        return "$scheme://$host:$port"
    }

    fun pair(baseUrl: String, code: String, timeoutMs: Int = 8_000): String {
        val root = normalizeBaseUrl(baseUrl)
        val trimmed = code.trim()
        if (trimmed.length < 4) throw BridgeException(0, "请填写电脑上的配对码")
        val payload = """{"code":${jsonString(trimmed)}}""".toByteArray(Charsets.UTF_8)
        val conn = open("POST", "$root/v1/pair", timeoutMs, timeoutMs)
        conn.doOutput = true
        conn.setFixedLengthStreamingMode(payload.size)
        conn.setRequestProperty("Content-Type", "application/json; charset=utf-8")
        conn.outputStream.use { it.write(payload) }
        val (status, body) = readText(conn)
        if (status != 200) throw BridgeException(status, errorOf(body, status))
        val token = jsonField(body, "token")
        if (token.isBlank()) throw BridgeException(status, "电脑没有返回配对令牌")
        return token
    }

    fun decrypt(
        baseUrl: String,
        token: String,
        fileName: String,
        body: InputStream,
        length: Long,
        openOutput: (BridgeFile) -> OutputStream,
        onProgress: (Long) -> Unit = {},
        isCancelled: () -> Boolean = { false },
        onConnection: (HttpURLConnection) -> Unit = {},
        timeoutMs: Int = 15_000,
    ): BridgeFile {
        val root = normalizeBaseUrl(baseUrl)
        if (token.isBlank()) throw BridgeException(401, "还没有配对")
        if (length < 0 || length > MAX_BYTES) throw BridgeException(413, "文件超过 256MB，没有发送")
        val safeName = fileName.substringAfterLast('/').substringAfterLast('\\').ifBlank { "audio.mflac" }
        val conn = open("POST", "$root/v1/decrypt", timeoutMs, 0)
        onConnection(conn)
        conn.doOutput = true
        conn.setFixedLengthStreamingMode(length)
        conn.setRequestProperty("Authorization", "Bearer $token")
        conn.setRequestProperty("X-Filename", safeName)
        conn.outputStream.use { out ->
            val buf = ByteArray(64 * 1024)
            var sent = 0L
            while (sent < length) {
                if (isCancelled()) throw CancelledException()
                val n = body.read(buf, 0, minOf(buf.size.toLong(), length - sent).toInt())
                if (n < 0) break
                out.write(buf, 0, n)
                sent += n
                onProgress(sent)
            }
        }
        val status = conn.responseCode
        if (status != 200) {
            val text = (conn.errorStream ?: conn.inputStream)?.bufferedReader()?.use { it.readText() }.orEmpty()
            throw BridgeException(status, errorOf(text, status))
        }
        val ext = conn.getHeaderField("X-Audio-Ext").orEmpty().ifBlank { ".flac" }
        val outName = conn.getHeaderField("X-Output-Name").orEmpty()
        val meta = BridgeFile(outName.ifBlank { safeName.substringBeforeLast('.') + ext }, ext)
        openOutput(meta).use { output ->
            conn.inputStream.use { input ->
                val buf = ByteArray(64 * 1024)
                while (true) {
                    if (isCancelled()) throw CancelledException()
                    val n = input.read(buf)
                    if (n < 0) break
                    output.write(buf, 0, n)
                }
            }
        }
        return meta
    }

    private fun open(method: String, url: String, connectMs: Int, readMs: Int): HttpURLConnection {
        val conn = URL(url).openConnection() as HttpURLConnection
        conn.requestMethod = method
        conn.connectTimeout = connectMs
        conn.readTimeout = readMs
        conn.instanceFollowRedirects = false
        return conn
    }

    private fun readText(conn: HttpURLConnection): Pair<Int, String> {
        val status = conn.responseCode
        val stream = if (status in 200..299) conn.inputStream else conn.errorStream
        val text = stream?.bufferedReader()?.use { it.readText() }.orEmpty()
        return status to text
    }
}

data class BridgeFile(val outputName: String, val extension: String)

class BridgeException(val http: Int, message: String) : Exception(message)

private fun jsonString(value: String): String {
    val escaped = buildString {
        append('"')
        value.forEach { ch ->
            when (ch) {
                '\\', '"' -> append('\\').append(ch)
                '\n' -> append("\\n")
                '\r' -> append("\\r")
                else -> append(ch)
            }
        }
        append('"')
    }
    return escaped
}

private fun jsonField(json: String, name: String): String {
    val key = "\"$name\""
    val at = json.indexOf(key)
    if (at < 0) return ""
    val colon = json.indexOf(':', at + key.length)
    if (colon < 0) return ""
    val start = json.indexOf('"', colon + 1)
    if (start < 0) return ""
    val out = StringBuilder()
    var i = start + 1
    while (i < json.length) {
        val ch = json[i]
        if (ch == '\\' && i + 1 < json.length) {
            out.append(json[i + 1])
            i += 2
            continue
        }
        if (ch == '"') break
        out.append(ch)
        i++
    }
    return out.toString()
}

private fun errorOf(body: String, status: Int): String {
    val parsed = jsonField(body, "error")
    if (parsed.isNotBlank()) return parsed
    return when (status) {
        401 -> "配对失败或已失效"
        413 -> "文件太大，电脑拒绝接收"
        429 -> "配对尝试过多，请稍后再试"
        else -> "电脑返回了错误（$status）"
    }
}
