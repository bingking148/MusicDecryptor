package app.musicdecryptor.engine

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

private val Context.settingsDataStore by preferencesDataStore("music_decryptor")

class SettingsStore(context: Context) {
    private val store = context.applicationContext.settingsDataStore

    val flow: Flow<UserSettings> = store.data.map { prefs ->
        UserSettings(
            outputMode = prefs[KEY_MODE] ?: UserSettings.MODE_DOWNLOADS,
            treeUri = prefs[KEY_TREE] ?: "",
            overwrite = prefs[KEY_OVERWRITE] ?: false,
            autoExt = prefs[KEY_AUTO_EXT] ?: true,
            manualEkey = prefs[KEY_EKEY] ?: "",
        )
    }

    suspend fun update(block: (UserSettings) -> UserSettings) {
        store.edit { prefs ->
            val current = UserSettings(
                outputMode = prefs[KEY_MODE] ?: UserSettings.MODE_DOWNLOADS,
                treeUri = prefs[KEY_TREE] ?: "",
                overwrite = prefs[KEY_OVERWRITE] ?: false,
                autoExt = prefs[KEY_AUTO_EXT] ?: true,
                manualEkey = prefs[KEY_EKEY] ?: "",
            )
            val next = block(current)
            prefs[KEY_MODE] = next.outputMode
            prefs[KEY_TREE] = next.treeUri
            prefs[KEY_OVERWRITE] = next.overwrite
            prefs[KEY_AUTO_EXT] = next.autoExt
            prefs[KEY_EKEY] = next.manualEkey
        }
    }

    private companion object {
        val KEY_MODE = stringPreferencesKey("output_mode")
        val KEY_TREE = stringPreferencesKey("tree_uri")
        val KEY_OVERWRITE = booleanPreferencesKey("overwrite")
        val KEY_AUTO_EXT = booleanPreferencesKey("auto_ext")
        val KEY_EKEY = stringPreferencesKey("manual_ekey")
    }
}
