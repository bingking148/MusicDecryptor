package app.musicdecryptor.crypto

import java.security.MessageDigest

/**
 * 用构建期生成的向量自检整条解密链。应用内「检测解密引擎」和单元测试都走这里。
 * 向量来自 android/tools/qmc-oracle：TEA 分组使用 golang.org/x/crypto/tea。
 */
object QmcSelfTest {
    fun run(): List<String> {
        val json = QmcSelfTest::class.java.getResourceAsStream("/qmc-vectors.json")
            ?.bufferedReader()?.use { it.readText() }
            ?: return listOf("缺少内置测试向量")
        val v = parseStringMap(json)
        val failures = mutableListOf<String>()
        fun check(name: String, ok: Boolean) {
            if (!ok) failures += name
        }
        try {
            val simple = QmcTea.simpleMakeKey(106, 8)
            check("simpleKey", simple.toHex() == v.getValue("simpleKey"))

            val teaKey = hexToBytes(v.getValue("teaKey"))
            val teaCipher = hexToBytes(v.getValue("teaCipher"))
            val plain = ByteArray(8)
            QmcTea.decryptBlock(teaCipher, 0, teaKey, plain, 0)
            check("teaBlock", plain.toHex() == v.getValue("teaPlain"))

            val body = QmcTea.decrypt(hexToBytes(v.getValue("teaBodyCipher")), teaKey)
            check("teaBody", body.toHex() == v.getValue("teaBody"))

            val mapKey = hexToBytes(v.getValue("mapKey"))
            check("deriveMap", deriveEKey(v.getValue("mapEkey")).contentEquals(mapKey))
            check("deriveV2", deriveEKey(v.getValue("v2Ekey")).contentEquals(mapKey))
            val rc4Key = hexToBytes(v.getValue("rc4Key"))
            check("deriveRc4", deriveEKey(v.getValue("rc4Ekey")).contentEquals(rc4Key))

            val audio = hexToBytes(v.getValue("mapAudio"))
            val enc0 = hexToBytes(v.getValue("mapEncrypted"))
            qmcCipher(mapKey).decrypt(enc0, 0, enc0.size, 0)
            check("map0", enc0.contentEquals(audio))
            val enc9 = hexToBytes(v.getValue("mapEncrypted9000"))
            qmcCipher(mapKey).decrypt(enc9, 0, enc9.size, 9000)
            check("map9000", enc9.contentEquals(audio))

            val plainRc4 = "OggS-rc4-segment-check".repeat(300).encodeToByteArray()
            check("rc4PlainSha", sha256(plainRc4) == v.getValue("rc4PlainSha"))
            val rc4Buf = plainRc4.copyOf()
            qmcCipher(rc4Key).decrypt(rc4Buf, 0, rc4Buf.size, 0)
            check("rc4Sha", sha256(rc4Buf) == v.getValue("rc4CipherSha"))
            check("rc4Head", rc4Buf.copyOfRange(0, 64).toHex() == v.getValue("rc4CipherHead"))
            val window = plainRc4.copyOfRange(100, 5300)
            qmcCipher(rc4Key).decrypt(window, 0, window.size, 100)
            check("rc4Window", window.copyOfRange(0, 32).toHex() == v.getValue("rc4Window"))

            val staticCipher = hexToBytes(v.getValue("staticCipher"))
            qmcCipher(ByteArray(0)).decrypt(staticCipher, 0, staticCipher.size, 0)
            check("static", staticCipher.toHex() == v.getValue("staticPlain"))

            val embedded = ArraySource(hexToBytes(v.getValue("embeddedFile")))
            val probe = probeQmc(embedded)
            check("probeEmbeddedKind", probe.kind == FooterKind.EMBEDDED)
            check("probeEmbeddedKey", probe.embeddedEKey == v.getValue("mapEkey"))
            check("probeEmbeddedLen", probe.audioLength == audio.size.toLong())
            val out = java.io.ByteArrayOutputStream()
            decryptAudio(embedded, out, qmcCipher(deriveEKey(probe.embeddedEKey!!)), probe.audioLength)
            check("embeddedRoundtrip", out.toByteArray().contentEquals(audio))
            check("embeddedSniff", sniffAudio(out.toByteArray()) == AudioKind.FLAC)

            val qtag = probeQmc(ArraySource(hexToBytes(v.getValue("qtagFile"))))
            check("qtag", qtag.kind == FooterKind.QTAG && qtag.embeddedEKey == v.getValue("mapEkey"))

            val stag = probeQmc(ArraySource(hexToBytes(v.getValue("stagFile"))))
            check(
                "stag",
                stag.kind == FooterKind.STAG &&
                    stag.stag?.mediaMid == "AbC123mid" &&
                    stag.stag.resourceId == "12345" &&
                    stag.audioLength == audio.size.toLong(),
            )

            val staticFile = ArraySource(hexToBytes(v.getValue("staticCipher")))
            // staticCipher in the vector is the encrypted bytes; probing that file should not
            // treat the tail as an ekey.
            val staticProbe = probeQmc(staticFile)
            check("staticProbe", staticProbe.kind == FooterKind.STATIC && staticProbe.audioLength == staticFile.size)

            check("nameMflac", outputStem("夜航.mflac") == "夜航" && preferredExtension("夜航.mflac") == ".flac")
            check("nameMgg", outputStem("a.mgg2.ogg") == "a" && preferredExtension("a.mgg2.ogg") == ".ogg")
            check("supported", isQmcFileName("x.mflac0") && !isQmcFileName("x.mp3"))
        } catch (e: Exception) {
            failures += "exception: ${e.javaClass.simpleName}: ${e.message}"
        }
        return failures
    }

    private fun sha256(data: ByteArray): String {
        return MessageDigest.getInstance("SHA-256").digest(data).toHex()
    }
}

internal fun parseStringMap(json: String): Map<String, String> {
    val out = linkedMapOf<String, String>()
    val re = Regex(""""([^"]+)"\s*:\s*"([^"\\]*)"""")
    for (m in re.findAll(json)) {
        out[m.groupValues[1]] = m.groupValues[2]
    }
    return out
}
