package app.musicdecryptor

import android.app.Application
import app.musicdecryptor.engine.DecryptEngine
import app.musicdecryptor.engine.KeyLibrary
import app.musicdecryptor.engine.SettingsStore

class MusicApp : Application() {
    lateinit var settings: SettingsStore
        private set
    lateinit var keys: KeyLibrary
        private set

    override fun onCreate() {
        super.onCreate()
        settings = SettingsStore(this)
        keys = KeyLibrary(this)
        DecryptEngine.attach(this, settings, keys)
    }
}
