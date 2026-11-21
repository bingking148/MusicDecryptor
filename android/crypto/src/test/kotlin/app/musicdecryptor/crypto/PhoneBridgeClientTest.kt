package app.musicdecryptor.crypto

import com.sun.net.httpserver.HttpServer
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.net.InetSocketAddress
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class PhoneBridgeClientTest {
    @Test
    fun rejectsUnsafeAddresses() {
        assertFailsWith<BridgeException> { PhoneBridgeClient.normalizeBaseUrl("ftp://192.168.1.2") }
        assertFailsWith<BridgeException> { PhoneBridgeClient.normalizeBaseUrl("http://user:pw@192.168.1.2") }
        assertFailsWith<BridgeException> { PhoneBridgeClient.normalizeBaseUrl("0.0.0.0:1") }
        assertEquals("http://192.168.1.8:47321", PhoneBridgeClient.normalizeBaseUrl("192.168.1.8:47321"))
    }

    @Test
    fun pairAndDecryptRoundTrip() {
        val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
        server.createContext("/v1/pair") { exchange ->
            val text = exchange.requestBody.readBytes().decodeToString()
            if (!text.contains("AB12-CD34")) {
                exchange.sendResponseHeaders(401, 0)
                exchange.close()
                return@createContext
            }
            val body = """{"token":"secret-token"}""".toByteArray()
            exchange.sendResponseHeaders(200, body.size.toLong())
            exchange.responseBody.use { it.write(body) }
        }
        server.createContext("/v1/decrypt") { exchange ->
            val auth = exchange.requestHeaders.getFirst("Authorization")
            val name = exchange.requestHeaders.getFirst("X-Filename")
            val payload = exchange.requestBody.readBytes()
            if (auth != "Bearer secret-token" || name != "song.mflac" || payload.decodeToString() != "cipher") {
                val err = """{"error":"拒绝"}""".toByteArray()
                exchange.sendResponseHeaders(401, err.size.toLong())
                exchange.responseBody.use { it.write(err) }
                return@createContext
            }
            val out = "fLaC-ok".toByteArray()
            exchange.responseHeaders.add("X-Audio-Ext", ".flac")
            exchange.responseHeaders.add("X-Output-Name", "song.flac")
            exchange.sendResponseHeaders(200, out.size.toLong())
            exchange.responseBody.use { it.write(out) }
        }
        server.start()
        try {
            val base = "http://127.0.0.1:${server.address.port}"
            val bad = assertFailsWith<BridgeException> { PhoneBridgeClient.pair(base, "NOPE-NOPE") }
            assertEquals(401, bad.http)
            val token = PhoneBridgeClient.pair(base, "AB12-CD34")
            assertEquals("secret-token", token)
            val output = ByteArrayOutputStream()
            val file = PhoneBridgeClient.decrypt(
                baseUrl = base,
                token = token,
                fileName = "song.mflac",
                body = ByteArrayInputStream("cipher".toByteArray()),
                length = 6,
                openOutput = { output },
            )
            assertEquals("fLaC-ok", output.toString(Charsets.UTF_8))
            assertEquals(".flac", file.extension)
            assertTrue(file.outputName.endsWith(".flac"))
        } finally {
            server.stop(0)
        }
    }

    @Test
    fun doesNotSendOversizedFile() {
        val error = assertFailsWith<BridgeException> {
            PhoneBridgeClient.decrypt(
                baseUrl = "http://127.0.0.1:9",
                token = "t",
                fileName = "a.mflac",
                body = ByteArrayInputStream(ByteArray(0)),
                length = PhoneBridgeClient.MAX_BYTES + 1,
                openOutput = { ByteArrayOutputStream() },
            )
        }
        assertEquals(413, error.http)
    }
}
