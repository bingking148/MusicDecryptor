package app.musicdecryptor.crypto

private val encryptedSuffix = Regex(
    """(?i)(\.(?:mflac|mgg)\d*|\.qmcflac|\.qmcogg|\.qmc\d+|\.flac|\.ogg|\.mp3|\.m4a|\.wav)$""",
)

private val supportedToken = Regex("""(?i)\.(mflac|mgg)\d*""")

fun isQmcFileName(fileName: String): Boolean {
    val lower = fileName.lowercase()
    if (supportedToken.containsMatchIn(lower)) return true
    return listOf(".qmc0", ".qmc2", ".qmc3", ".qmc4", ".qmc6", ".qmc8", ".qmcflac", ".qmcogg")
        .any { lower.endsWith(it) }
}

/** 去掉加密后缀和可能套在外面的音频后缀，得到输出主文件名。 */
fun outputStem(fileName: String): String {
    var name = fileName.trim().substringAfterLast('/').substringAfterLast('\\')
    repeat(6) {
        val next = encryptedSuffix.replace(name, "")
        if (next == name) return sanitizeStem(name)
        name = next
    }
    return sanitizeStem(name)
}

fun preferredExtension(fileName: String): String {
    val lower = fileName.lowercase()
    return when {
        ".mgg" in lower || ".qmcogg" in lower || lower.endsWith(".qmc6") -> ".ogg"
        lower.endsWith(".qmc0") || lower.endsWith(".qmc3") -> ".mp3"
        lower.endsWith(".qmc2") || lower.endsWith(".qmc4") || lower.endsWith(".qmc8") -> ".m4a"
        else -> ".flac"
    }
}

private fun sanitizeStem(name: String): String {
    val cleaned = name.replace(Regex("""[\\/:*?"<>|]"""), "_").trim().trim('.')
    return cleaned.ifBlank { "audio" }
}
