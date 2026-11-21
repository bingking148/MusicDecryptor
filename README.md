# 音乐解密器 Music Decryptor

原 `music-decryptor-master`（Python + Frida 命令行脚本）的 **Go 桌面应用版**。

调用本机正在运行的 **QQ 音乐客户端**，把 `.mflac` / `.mgg` 批量还原为通用音频格式（`.flac` / `.ogg` / `.mp3` 等）。

> **下载**：无需自己编译，直接到 [Releases](../../releases) 页面下载最新版 `MusicDecryptor-*.exe` 即可。

> 请仅用于处理你合法获得的音频文件（如个人备份、设备间迁移），请勿用于未授权的传播或分发。

## 功能

- 🖱️ 拖拽目录 / 文件即可开始，支持递归扫描子目录
- 🔍 解密后按文件头**自动识别真实格式**修正扩展名（可关闭）
- 📁 输出支持**保留目录结构**、跳过已存在文件、输出重名检测
- 📊 实时进度：整体进度条 + 单文件状态（待处理 / 解密中 / 成功 / 跳过 / 失败）+ 统计卡片
- 🔎 任务预扫描：开始前预览文件清单、总大小和跳过项，支持搜索、状态筛选与排序
- ↻ 失败项单独重试，成功产物与原文件可一键在资源管理器中定位
- 🕘 本机任务记录：保存最近 20 次任务，可快速恢复目录与选项
- 🧾 运行日志面板，支持按级别筛选、复制、导出与清空
- ⚙️ 设置页记忆默认输出目录、默认选项与完成后行为（`%APPDATA%\MusicDecryptorDesktop\settings.json`）
- 🌗 专业桌面风格暗色 / 浅色双主题，无边框窗口与细线图标
- 🔧 环境自检：侧边栏实时显示 QQ 音乐客户端运行状态与解密引擎状态
- 📦 引擎（frida-inject，两种架构）内置于应用中，解压即用，无需安装 Python / Frida

## 使用前提

1. Windows 10 / 11 x64
2. 本机安装了 QQ 音乐桌面客户端
3. **解密前先启动 QQ 音乐**（进入主界面即可，无需播放）

## 使用方法

1. 启动 `MusicDecryptor.exe`
2. 拖入或选择包含 `.mflac` / `.mgg` 的目录
3. 选择输出目录
4. 点击「开始解密」

## 工作原理

```
MusicDecryptor.exe (Go + Wails)
   │  1. 扫描目录，生成任务清单 jobs.json
   │  2. 在输出盘根建立 ASCII 暂存目录 + 目录联接(junction)
   │     （绕开 QQ 音乐 DLL 无法读取非 ASCII 路径的限制）
   │  3. 按 QQMusic.exe 位数选择内置的 frida-inject (x86/x64)
   ▼
frida-inject ──注入──> QQMusic.exe
   │  4. 脚本通过 QQMusicCommon.dll 导出的 EncAndDesMediaFile
   │     类逐个解密文件（客户端自带组件，不内置任何算法）
   │  5. 经 frida:stdout 通道实时回传进度
   ▼
应用把解密产物改名/搬运到输出目录（自动修正扩展名）
```

应用本体不含任何解密算法，解密完全由 QQ 音乐客户端自身的组件完成；
应用也不修改 QQ 音乐客户端。

## 开发 / 构建

```bash
# 前端为原生 HTML/CSS/JS，无需 npm
wails build            # 产物: build/bin/MusicDecryptor.exe
wails dev              # 开发热重载
```

- Go 后端：`main.go` / `app.go` / `engine/`
- 注入脚本模板：`engine/assets/hook_qq_music.js`（宿主会把任务清单路径填入 `__JOBS_PATH__`）
- frida-inject 二进制：`engine/assets/*.exe.xz`（随仓库提交，编译时 go:embed 打包进应用；
  首次运行解压到应用目录 `bin/`，也可手动替换 `bin/frida-inject-ia32.exe`
  或 `bin/frida-inject-x64.exe` 以升级引擎）

## 致谢

- [Frida](https://frida.re) — 注入引擎（frida-inject 二进制随应用分发）
- 原项目 music-decryptor-master（Python + Frida 脚本）— 本应用为其桌面化移植

## 许可说明

- 本仓库代码仅供学习与个人合法用途使用
- 应用本体不含任何音乐格式解密算法，解密行为完全依赖本机 QQ 音乐客户端自身组件
- frida-inject 二进制遵循 [Frida 的许可条款](https://github.com/frida/frida/blob/main/COPYING)
