package app.musicdecryptor.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.musicdecryptor.engine.KeyState
import app.musicdecryptor.engine.Track
import app.musicdecryptor.engine.TrackStatus
import app.musicdecryptor.engine.UserSettings
import app.musicdecryptor.engine.outputLabel
import app.musicdecryptor.theme.Danger
import app.musicdecryptor.theme.Ink
import app.musicdecryptor.theme.Lime
import app.musicdecryptor.theme.Line
import app.musicdecryptor.theme.Success
import app.musicdecryptor.theme.Surface2
import app.musicdecryptor.theme.Text
import app.musicdecryptor.theme.TextDim
import app.musicdecryptor.theme.TextFaint

@Composable
fun OutputSheet(
    settings: UserSettings,
    onMode: (String) -> Unit,
    onPickTree: () -> Unit,
    onOverwrite: (Boolean) -> Unit,
    onAutoExt: (Boolean) -> Unit,
) {
    Column(Modifier.padding(horizontal = 20.dp).padding(bottom = 28.dp)) {
        Text("输出", color = Text, fontSize = 22.sp, fontWeight = FontWeight.Medium)
        Text("源文件始终保留。只有打开「覆盖同名输出」时，才会替换输出目录里的同名结果。", color = TextDim, fontSize = 13.sp)
        Spacer(Modifier.height(16.dp))
        ModeRow("下载 / MusicDecryptor", "出现在系统「下载」里，不用额外授权", settings.outputMode == UserSettings.MODE_DOWNLOADS) {
            onMode(UserSettings.MODE_DOWNLOADS)
        }
        ModeRow("应用音乐目录", "不需要权限，适合先解密再分享出去", settings.outputMode == UserSettings.MODE_APP) {
            onMode(UserSettings.MODE_APP)
        }
        ModeRow("自选文件夹", if (settings.treeUri.isBlank()) "用系统选择器指定一个目录" else "已选择文件夹", settings.outputMode == UserSettings.MODE_TREE, onPickTree)
        Spacer(Modifier.height(8.dp))
        Text("当前：${outputLabel(settings)}", color = TextFaint, fontSize = 12.sp)
        Spacer(Modifier.height(12.dp))
        ToggleRow("覆盖同名输出", "默认跳过已存在的文件", settings.overwrite, onOverwrite)
        ToggleRow("按文件头修正扩展名", "把结果认成真实的 flac / ogg / mp3", settings.autoExt, onAutoExt)
    }
}

@Composable
fun KeySheet(
    keys: KeyState,
    manual: String,
    selfTest: String?,
    onImport: () -> Unit,
    onSaveEkey: (String) -> Unit,
    onClear: () -> Unit,
    onSelfTest: () -> Unit,
) {
    var draft by remember(manual) { mutableStateOf(manual) }
    Column(
        Modifier
            .padding(horizontal = 20.dp)
            .padding(bottom = 28.dp)
            .verticalScroll(rememberScrollState()),
    ) {
        Text("密钥", color = Text, fontSize = 22.sp, fontWeight = FontWeight.Medium)
        Spacer(Modifier.height(6.dp))
        Text(
            "带 QTag 或尾部 ekey 的文件可以直接解。STag / musicex 的密钥不在文件里，而在 QQ 音乐私有目录，普通授权和 Shizuku 都读不到。你可以导入自己导出的 player_process_db 或 MMKVStreamEncryptId，也可以用电脑接力。",
            color = TextDim,
            fontSize = 13.sp,
        )
        Spacer(Modifier.height(14.dp))
        Text(keys.message, color = if (keys.count > 0) Success else TextFaint, fontSize = 13.sp)
        if (keys.label.isNotBlank()) {
            Text(keys.label, color = TextFaint, fontSize = 12.sp)
        }
        Spacer(Modifier.height(12.dp))
        PrimaryButton("导入密钥库或文本", true, onImport, Modifier.fillMaxWidth())
        Spacer(Modifier.height(8.dp))
        GhostButton("清除已导入的密钥", onClear, Modifier.fillMaxWidth())
        Spacer(Modifier.height(18.dp))
        Text("备用 ekey", color = Text, fontSize = 14.sp, fontWeight = FontWeight.Medium)
        Text("对不上文件名时，用这一条尝试所有缺少密钥的文件。", color = TextFaint, fontSize = 12.sp)
        Spacer(Modifier.height(8.dp))
        TextField(
            value = draft,
            onValueChange = { draft = it },
            modifier = Modifier.fillMaxWidth(),
            placeholder = { Text("粘贴 ekey", color = TextFaint) },
            minLines = 2,
            colors = TextFieldDefaults.colors(
                focusedContainerColor = Surface2,
                unfocusedContainerColor = Surface2,
                focusedIndicatorColor = Color.Transparent,
                unfocusedIndicatorColor = Color.Transparent,
                focusedTextColor = Text,
                unfocusedTextColor = Text,
                cursorColor = Lime,
            ),
            shape = RoundedCornerShape(14.dp),
        )
        Spacer(Modifier.height(8.dp))
        PrimaryButton("保存备用密钥", true, { onSaveEkey(draft) }, Modifier.fillMaxWidth())
        Spacer(Modifier.height(18.dp))
        GhostButton("检测解密引擎", onSelfTest, Modifier.fillMaxWidth())
        if (!selfTest.isNullOrBlank()) {
            Spacer(Modifier.height(8.dp))
            Text(selfTest, color = if (selfTest.contains("通过")) Success else Danger, fontSize = 13.sp)
        }
    }
}

@Composable
fun ComputerSheet(
    host: String,
    busy: Boolean,
    onStart: (host: String, code: String) -> Unit,
) {
    var address by remember(host) { mutableStateOf(host) }
    var code by remember { mutableStateOf("") }
    Column(
        Modifier
            .padding(horizontal = 20.dp)
            .padding(bottom = 28.dp)
            .verticalScroll(rememberScrollState()),
    ) {
        Text("用电脑解开", color = Text, fontSize = 22.sp, fontWeight = FontWeight.Medium)
        Spacer(Modifier.height(6.dp))
        Text(
            "没有内嵌密钥时，手机不能自己变成 QQ 音乐。无线调试和 Shizuku 只提供 shell 权限，进不了 /data/data。若电脑上的音乐解密和 QQ 音乐都开着，并在设置里点了「开启接力」，可以把这些文件送到那台电脑。",
            color = TextDim,
            fontSize = 13.sp,
        )
        Spacer(Modifier.height(8.dp))
        Text("不点连接时，应用不会联网。配对码不要发到公共网络。", color = TextFaint, fontSize = 12.sp)
        Spacer(Modifier.height(14.dp))
        Text("电脑地址", color = Text, fontSize = 14.sp, fontWeight = FontWeight.Medium)
        TextField(
            value = address,
            onValueChange = { address = it },
            modifier = Modifier.fillMaxWidth(),
            placeholder = { Text("192.168.1.8:47321", color = TextFaint) },
            singleLine = true,
            colors = bridgeFieldColors(),
            shape = RoundedCornerShape(14.dp),
        )
        Spacer(Modifier.height(10.dp))
        Text("配对码", color = Text, fontSize = 14.sp, fontWeight = FontWeight.Medium)
        TextField(
            value = code,
            onValueChange = { code = it },
            modifier = Modifier.fillMaxWidth(),
            placeholder = { Text("电脑上显示的 8 位码", color = TextFaint) },
            singleLine = true,
            colors = bridgeFieldColors(),
            shape = RoundedCornerShape(14.dp),
        )
        Spacer(Modifier.height(12.dp))
        PrimaryButton(
            if (busy) "正在连接" else "连接并解密缺密钥的文件",
            enabled = !busy && address.isNotBlank() && code.isNotBlank(),
            onClick = { onStart(address.trim(), code.trim()) },
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

@Composable
private fun bridgeFieldColors() = TextFieldDefaults.colors(
    focusedContainerColor = Surface2,
    unfocusedContainerColor = Surface2,
    focusedIndicatorColor = Color.Transparent,
    unfocusedIndicatorColor = Color.Transparent,
    focusedTextColor = Text,
    unfocusedTextColor = Text,
    cursorColor = Lime,
)

@Composable
fun DetailSheet(
    track: Track,
    onShare: () -> Unit,
    onOpen: () -> Unit,
    onRetry: () -> Unit,
) {
    Column(Modifier.padding(horizontal = 20.dp).padding(bottom = 28.dp)) {
        Text(track.title ?: track.name, color = Text, fontSize = 22.sp, fontWeight = FontWeight.Medium)
        val artist = listOfNotNull(track.artist, track.album).joinToString("  ·  ")
        if (artist.isNotBlank()) {
            Text(artist, color = TextDim, fontSize = 14.sp)
        }
        Spacer(Modifier.height(8.dp))
        Text("${formatBytes(track.size)}  ·  ${track.planLabel}", color = TextFaint, fontSize = 12.sp)
        if (track.outputName.isNotBlank()) {
            Text("输出  ${track.outputName}", color = TextDim, fontSize = 13.sp)
        }
        if (track.message.isNotBlank()) {
            Spacer(Modifier.height(8.dp))
            Text(track.message, color = statusColor(track.status), fontSize = 13.sp)
        }
        Spacer(Modifier.height(16.dp))
        if (track.status == TrackStatus.SUCCESS) {
            PrimaryButton("打开", true, onOpen, Modifier.fillMaxWidth())
            Spacer(Modifier.height(8.dp))
            GhostButton("分享", onShare, Modifier.fillMaxWidth())
        }
        if (track.status == TrackStatus.FAILED) {
            PrimaryButton("重试这一首", true, onRetry, Modifier.fillMaxWidth())
        }
    }
}

@Composable
private fun ModeRow(title: String, subtitle: String, selected: Boolean, onClick: () -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp)
            .background(if (selected) Surface2 else Color.Transparent, RoundedCornerShape(14.dp))
            .border(1.dp, if (selected) Lime else Line, RoundedCornerShape(14.dp))
            .clickable(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(title, color = Text, fontSize = 15.sp, fontWeight = FontWeight.Medium)
            Text(subtitle, color = TextFaint, fontSize = 12.sp)
        }
        if (selected) Text("使用中", color = Lime, fontSize = 12.sp)
    }
}

@Composable
private fun ToggleRow(title: String, subtitle: String, checked: Boolean, onChecked: (Boolean) -> Unit) {
    Row(Modifier.fillMaxWidth().padding(vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(title, color = Text, fontSize = 15.sp)
            Text(subtitle, color = TextFaint, fontSize = 12.sp)
        }
        Switch(
            checked = checked,
            onCheckedChange = onChecked,
            colors = SwitchDefaults.colors(
                checkedTrackColor = Lime,
                checkedThumbColor = Ink,
                uncheckedTrackColor = Surface2,
                uncheckedBorderColor = Line,
            ),
        )
    }
}
