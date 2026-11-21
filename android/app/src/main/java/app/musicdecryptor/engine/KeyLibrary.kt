package app.musicdecryptor.engine

import android.content.Context
import android.database.sqlite.SQLiteDatabase
import android.net.Uri
import app.musicdecryptor.crypto.outputStem
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.withContext
import java.io.File

data class KeyState(
    val label: String = "",
    val count: Int = 0,
    val message: String = "还没有导入密钥库",
)

data class KeyEntry(val name: String, val ekey: String)

class KeyLibrary(private val context: Context) {
    private val file = File(context.filesDir, "keys/entries.tsv")
    private val entries = mutableListOf<KeyEntry>()
    private val stateFlow = MutableStateFlow(KeyState())
    val state: StateFlow<KeyState> = stateFlow

    init {
        load()
    }

    fun lookup(fileName: String, mediaMid: String?): String? {
        val base = baseName(fileName).lowercase()
        val stem = outputStem(fileName).lowercase()
        entries.firstOrNull {
            val name = baseName(it.name).lowercase()
            name == base || outputStem(it.name).lowercase() == stem
        }?.let { return it.ekey }
        if (!mediaMid.isNullOrBlank()) {
            val mid = mediaMid.lowercase()
            entries.firstOrNull { baseName(it.name).lowercase().contains(mid) }?.let { return it.ekey }
        }
        return null
    }

    suspend fun import(uri: Uri): String = withContext(Dispatchers.IO) {
        val name = queryName(context, uri) ?: "密钥文件"
        val bytes = context.contentResolver.openInputStream(uri)?.use { it.readBytes() }
            ?: return@withContext "读不到这个文件"
        if (bytes.size >= 16 && bytes.copyOfRange(0, 15).decodeToString() == "SQLite format 3") {
            val dbFile = File(context.cacheDir, "import-keys.db")
            dbFile.writeBytes(bytes)
            try {
                val parsed = readSqlite(dbFile)
                if (parsed.isEmpty()) return@withContext "数据库里没有 audio_file_ekey_table 或 EKeyFileInfo"
                replace(parsed, name)
                "已导入 ${parsed.size} 条密钥"
            } catch (e: Exception) {
                "打不开这个数据库：${e.message ?: "格式无法识别"}"
            } finally {
                dbFile.delete()
            }
        } else {
            val text = bytes.toString(Charsets.UTF_8)
            val parsed = readText(text)
            if (parsed.size == 1 && parsed[0].name.isBlank()) {
                return@withContext "BARE:" + parsed[0].ekey
            }
            if (parsed.isEmpty()) return@withContext "没有解析到密钥。文本格式是「文件名<Tab>ekey」，每行一条"
            replace(parsed, name)
            "已导入 ${parsed.size} 条密钥"
        }
    }

    fun clear() {
        entries.clear()
        file.delete()
        publish("已清除密钥库")
    }

    private fun replace(parsed: List<KeyEntry>, label: String) {
        entries.clear()
        entries += parsed
        file.parentFile?.mkdirs()
        file.writeText(entries.joinToString("\n") { it.name.replace("\t", " ") + "\t" + it.ekey })
        publish("已载入 ${entries.size} 条", label)
    }

    private fun load() {
        if (!file.exists()) return
        val parsed = readText(file.readText())
        entries.clear()
        entries += parsed
        publish("已载入 ${entries.size} 条", "本地密钥库")
    }

    private fun publish(message: String, label: String = stateFlow.value.label) {
        stateFlow.value = KeyState(label = label, count = entries.size, message = message)
    }

    private fun readSqlite(dbFile: File): List<KeyEntry> {
        val db = SQLiteDatabase.openDatabase(dbFile.absolutePath, null, SQLiteDatabase.OPEN_READONLY)
        db.use {
            val tables = mutableSetOf<String>()
            it.rawQuery("SELECT name FROM sqlite_master WHERE type='table'", null).use { cursor ->
                while (cursor.moveToNext()) tables += cursor.getString(0)
            }
            val sql = when {
                "audio_file_ekey_table" in tables -> "SELECT file_path, ekey FROM audio_file_ekey_table"
                "EKeyFileInfo" in tables -> "SELECT filePath, eKey FROM EKeyFileInfo"
                else -> return emptyList()
            }
            val out = mutableListOf<KeyEntry>()
            it.rawQuery(sql, null).use { cursor ->
                while (cursor.moveToNext()) {
                    val path = cursor.getString(0) ?: continue
                    val ekey = cursor.getString(1) ?: continue
                    if (path.isBlank() || ekey.isBlank()) continue
                    out += KeyEntry(baseName(path), ekey.trim())
                }
            }
            return out
        }
    }

    private fun readText(text: String): List<KeyEntry> {
        val out = mutableListOf<KeyEntry>()
        for (raw in text.lineSequence()) {
            val line = raw.trim()
            if (line.isEmpty() || line.startsWith("#")) continue
            val tab = line.indexOf('\t')
            val comma = line.indexOf(',')
            val splitAt = when {
                tab >= 0 && (comma < 0 || tab < comma) -> tab
                comma >= 0 -> comma
                else -> -1
            }
            if (splitAt < 0) {
                if (line.length > 20) out += KeyEntry("", line)
                continue
            }
            val name = line.substring(0, splitAt).trim()
            val ekey = line.substring(splitAt + 1).trim()
            if (ekey.isNotBlank()) out += KeyEntry(name, ekey)
        }
        return out
    }
}

internal fun baseName(path: String): String =
    path.substringAfterLast('/').substringAfterLast('\\')
