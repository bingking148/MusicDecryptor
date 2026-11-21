# 音乐解密 · Android

桌面版把脚本注入正在运行的 `QQMusic.exe`，调用 `QQMusicCommon.dll` 里的 `EncAndDesMediaFile`。手机上没有这个进程，也不能注入其他应用，所以 Android 版改为**在本机完成解密**：读取你选中的 `.mflac` / `.mgg`（以及同系列 `.qmc*`），用文件自带的 ekey，或你导入的密钥，还原成 `.flac` / `.ogg` / `.mp3` 等。

应用不联网，不登录 QQ 音乐，也不修改源文件。

请只处理你有权使用的音频（个人备份、换机）。不要用来传播未授权的文件。

## 能解密什么

| 文件 | 条件 |
| --- | --- |
| 尾部带 ekey（4 字节长度）或 `QTag` 的 `.mflac` / `.mgg` / `.qmc*` | 离线直接解，密钥在文件里 |
| 旧版没有密钥尾的 `.qmc0` 等 | 用静态掩码尝试，文件头必须能认出是音频 |
| 安卓 QQ 音乐缓存（尾部是 `STag`）或 `musicex` | 文件里没有密钥。导入 `player_process_db` 或明文 `MMKVStreamEncryptId`，或用电脑接力 |

密钥可以来自 SQLite（`audio_file_ekey_table` 或 `EKeyFileInfo`）、明文 MMKV（`MMKVStreamEncryptId`）、JSON，或每行 `文件名<Tab>ekey` 的文本。

安卓客户端把数据库放在应用私有目录：

```text
/data/data/com.tencent.qqmusic/databases/player_process_db
```

普通应用读不到这里。需要你自己用 root 或备份工具把库拷出来（建议先打开一次 QQ 音乐，让 `-wal` 写回主库），再在本应用的「密钥」里导入。导入后按文件名和 STag 里的 media id 自动配对。

## 又核对过的路径

对照 unlock-music、freenote、QMCDecoder 和 QQ 音乐自己的存放方式之后，结论没有变成「手机上有一条被漏掉的万能算法」：

| 做法 | 结果 |
| --- | --- |
| 文件尾 QTag / 内嵌 ekey / 旧版静态掩码 | 手机离线可解，不需要电脑 |
| STag、musicex | 尾部只有曲目编号或文件名，没有 ekey |
| `/data/data/com.tencent.qqmusic/databases/player_process_db` | 官方解锁说明和 2024–2025 的实机记录都指向这里；提取命令写的是 `adb shell su`，不是普通 adb |
| `files/mmkv/MMKVStreamEncryptId` | 同一私有目录里的另一份密钥。本应用现在能导入这个明文库，但拿不到文件本身 |
| 无线调试 / Shizuku | shell（uid 2000）读不了其他正式版应用的 `/data/data`。Shizuku 官方说明和 issue 都这么写。没有做成一个假装能读的按钮 |
| 旧版 QQ 音乐（约 11.6 之前）或简洁版下载 | 密钥写在文件里，选进来就能解。这是换客户端重新下载，不是本应用突破沙箱 |
| vivo 等厂商的应用数据备份 | 备份包里可能带出数据库，解压后用「密钥」导入。这是厂商备份，不是每台手机都有 |

## 为什么电脑可以、手机不行

电脑版并不自己算密钥。它把脚本注入**已经打开的 Windows QQ 音乐**，调用客户端自己的 `EncAndDesMediaFile`。客户端手里有这首歌的 ekey，所以 STag 文件也能解开。

安卓 QQ 音乐把同一把 ekey 放在应用私有目录：

```text
/data/data/com.tencent.qqmusic/databases/player_process_db
```

这是系统给那个应用单独留的目录。别的应用即使用户授权了「所有文件」或 SAF，也读不到 `/data/data`。这和点权限弹窗不是一回事，本应用不会用漏洞或 root 去打开它。`/Android/data/...` 在较新的系统上同样不保证能被第三方读到，而且密钥通常不在那里。

因此：

- **文件自带 ekey / QTag，或你导入了数据库、明文 MMKV、JSON**：手机可以离线解，不需要电脑。
- **只有 STag、没有密钥**：手机单独解不了。不要把它理解成「手机算不出格式」，缺的是密钥。
- **你有一台已经能解密的电脑**：在电脑的「偏好设置 → 手机接力」里主动开启，手机和电脑连同一个 Wi-Fi，在手机里填地址和配对码。电脑仍用原来的 QQ 音乐通道解密，再把结果送回手机。不开启时，手机不会监听，也不会发网。

接力只绑回环和私网地址（`127.0.0.1`、`10/8`、`172.16/12`、`192.168/16`），不监听公网网卡。配对码约 15 分钟有效，错 5 次会锁定。单文件上限 256MB。关掉接力后，旧配对立即失效。局域网传输没有额外加密，只适合你自己的网络。

## 做不到的部分

这些是桌面版才有的，手机上没有等价物：

- 注入 `QQMusic.exe` / 调用 `QQMusicCommon.dll`。桌面版本体不含算法，解密发生在客户端进程里；Android 版必须自带流密码实现，否则无法解密。
- 从 QQ 音乐服务器拉取 ekey。本应用没有网络代码。
- 在没有 root、没有用户导出数据库、也没有配对电脑时，解开只有 STag 的缓存。这是密钥不在文件里，并且放在其他应用的私有目录中。
- Mac 客户端的 `cex` 尾和 MMKV 密钥。这类文件会明确提示，而不是假装解开。

算法与公开的 QMCv2 实现一致：ekey 经 Base64 和 Tencent TEA（`oi_symmetry_decrypt2`，16 轮）还原；密钥不超过 300 字节时用 map，更长用分段 RC4（首段 128 字节，之后每 5120 字节）；没有密钥时才用静态掩码。对照向量由 `android/tools/qmc-oracle` 生成，TEA 分组使用 `golang.org/x/crypto/tea`。仓库里没有真实曲库样本，单元测试和「检测解密引擎」验证的是这条实现与对照向量一致，而不是某一首在线歌曲。

## 怎么用

1. 打开应用，点「选择文件」，选一个或多个 `.mflac` / `.mgg`。
2. 列表会标出密钥来源：内嵌密钥、密钥库、备用密钥、旧版静态密钥，或缺少密钥。
3. 默认写到系统「下载 / MusicDecryptor」。可在「输出设置」改成应用目录或自选文件夹。同名输出默认跳过；只有打开「覆盖同名输出」才会替换**输出**，源文件始终保留。
4. 点「解密」。通知栏显示进度，可以取消。失败项可以单独重试，或点「重试失败」。
5. 成功后能看到曲名、歌手和封面（文件里有标签时），并可以打开或分享。

下拉列表会按当前密钥库重新判断每一首。左滑可以把文件移出列表，不会删除磁盘上的文件。

## 构建

需要 JDK 17+、Android SDK（compileSdk / targetSdk 35，minSdk 26）。

### Android Studio

用 Android Studio 打开 `android/` 目录（不要打开仓库根目录），等 Gradle 同步完成，运行 `app`。

### 命令行

```bash
cd android
# 若还没有 local.properties：
echo "sdk.dir=$ANDROID_HOME" > local.properties

./gradlew :crypto:test :app:assembleDebug
```

调试包：

```text
android/app/build/outputs/apk/debug/app-debug.apk
```

安装：

```bash
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

`local.properties`、`.gradle/` 和 `build/` 不入库。

解密引擎的单元测试不依赖模拟器：

```bash
./gradlew :crypto:test
```

应用内「密钥 → 检测解密引擎」跑的是同一组向量。

## 工程结构

```text
android/
  app/          Compose 界面、SAF、批量任务、通知
  crypto/       纯 Kotlin 解密核心（JVM 单元测试）
  tools/qmc-oracle/   生成 crypto 测试向量的 Go 程序，不参与打包
```

界面是深色、偏唱片的排版：解密时唱片按进度转动，列表项跟随状态变色，支持下拉刷新和滑动移除。任务跑在应用自己的协程里，并用前台服务保住进程；离开页面不会把正在写的文件丢掉。

## English

The desktop app injects `QQMusic.exe` and calls `EncAndDesMediaFile`. That process does not exist on Android, so this app decrypts on device.

- Embedded ekey / `QTag`: offline.
- Legacy keyless `.qmc*`: static mask, accepted only if the plaintext header is audio.
- Android cache (`STag`): no key in the file. Import `player_process_db` (`audio_file_ekey_table`) or paste an ekey. The database lives in QQ Music's private data dir and is not readable without root or a backup.
- No network, no login, sources are never overwritten. Existing outputs are skipped unless you opt in.
- Mac `cex` / MMKV keys are not supported.

```bash
cd android && ./gradlew :crypto:test :app:assembleDebug
```

Debug APK: `android/app/build/outputs/apk/debug/app-debug.apk`. minSdk 26, targetSdk 35.
