// QQ 音乐批量解密脚本（由 Go 宿主写入暂存目录后，通过 frida-inject 注入 QQMusic.exe）。
// 移植自 music-decryptor-master 项目的 hook_qq_music.js：
// 通过 QQMusicCommon.dll 导出的 EncAndDesMediaFile 类，用客户端自身完成 .mflac/.mgg 解密。
//
// 宿主与脚本的协议（stdout send 消息）：
//   { t: "start", id }                    开始解密某个文件
//   { t: "done", id, size }               解密成功，产物已写到 job.tmp
//   { t: "fail", id, err }                解密失败
//   { t: "fatal", err }                   脚本级致命错误
//   { t: "finished" }                     全部任务完成

'use strict';

const TARGET_DLL = 'QQMusicCommon.dll';
const JOBS_PATH = '__JOBS_PATH__'; // 由宿主替换为 ASCII 暂存目录下的 jobs.json

// ---------- 基础工具 ----------

// 通过 frida:stdout 通道发送进度事件（inject.vala 对该格式会立即 flush 到 stdout）。
// frida:stdout 不追加换行，需要自带 \n 供宿主按行解析。
function emit(obj) {
  send(['frida:stdout', JSON.stringify(obj) + '\n']);
}


function waitForModule(name, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      return Process.getModuleByName(name);
    } catch (e) {
      if (Date.now() > deadline) {
        throw new Error('等待模块 ' + name + ' 加载超时（请确认已进入 QQ 音乐主界面后重试）');
      }
      Thread.sleep(0.25);
    }
  }
}

function isASCII(s) {
  for (let i = 0; i < s.length; i++) {
    if (s.charCodeAt(i) > 127) return false;
  }
  return true;
}

// frida File 静态方法在不同版本间有差异，这里统一走 kernel32 原生读取，稳妥可靠。
function readAllTextNative(path) {
  const k32 = Process.getModuleByName('kernel32.dll');
  const CreateFileW = new NativeFunction(
    k32.getExportByName('CreateFileW'),
    'pointer', ['pointer', 'uint32', 'uint32', 'pointer', 'uint32', 'uint32', 'pointer'],
    'stdcall'
  );
  const GetFileSize = new NativeFunction(
    k32.getExportByName('GetFileSize'),
    'uint32', ['pointer', 'pointer'],
    'stdcall'
  );
  const ReadFile = new NativeFunction(
    k32.getExportByName('ReadFile'),
    'int32', ['pointer', 'pointer', 'uint32', 'pointer', 'pointer'],
    'stdcall'
  );
  const CloseHandle = new NativeFunction(
    k32.getExportByName('CloseHandle'),
    'int32', ['pointer'],
    'stdcall'
  );

  const GENERIC_READ = 0x80000000;
  const FILE_SHARE_READ = 0x1;
  const OPEN_EXISTING = 3;

  const h = CreateFileW(
    Memory.allocUtf16String(path),
    GENERIC_READ,
    FILE_SHARE_READ,
    ptr(0),
    OPEN_EXISTING,
    0,
    ptr(0)
  );
  if (h.isNull() || h.equals(ptr(-1))) {
    throw new Error('CreateFileW 失败: ' + path);
  }

  const size = GetFileSize(h, ptr(0));
  const buf = Memory.alloc(size + 1);
  const bytesRead = Memory.alloc(4);
  let got = 0;
  while (got < size) {
    const chunk = Math.min(size - got, 1 << 20);
    if (!ReadFile(h, buf.add(got), chunk, bytesRead, ptr(0))) {
      CloseHandle(h);
      throw new Error('ReadFile 失败');
    }
    const n = bytesRead.readU32();
    if (n === 0) break;
    got += n;
  }
  CloseHandle(h);
  return buf.readUtf8String(got);
}

function readJobs(path) {
  try {
    if (typeof File !== 'undefined' && typeof File.readAllText === 'function') {
      return JSON.parse(File.readAllText(path));
    }
  } catch (e) {
    // 继续使用原生实现
  }
  return JSON.parse(readAllTextNative(path));
}

function writeBytes(path, data) {
  const f = new File(path, 'wb');
  f.write(data);
  f.flush();
  f.close();
}

// ---------- 主流程 ----------

function main() {
  if (!isASCII(JOBS_PATH)) {
    emit({ t: 'fatal', err: '任务文件路径不是 ASCII，无法继续（内部错误）' });
    return;
  }

  const mod = waitForModule(TARGET_DLL, 20000);
  const findExport = (name) => mod.getExportByName(name);

  // EncAndDesMediaFile 的构造/析构/打开/取大小/读取（32 位 thiscall）
  const EncAndDesMediaFileConstructor = new NativeFunction(
    findExport('??0EncAndDesMediaFile@@QAE@XZ'), 'pointer', ['pointer'], 'thiscall'
  );
  const EncAndDesMediaFileDestructor = new NativeFunction(
    findExport('??1EncAndDesMediaFile@@QAE@XZ'), 'void', ['pointer'], 'thiscall'
  );
  const EncAndDesMediaFileOpen = new NativeFunction(
    findExport('?Open@EncAndDesMediaFile@@QAE_NPB_W_N1@Z'), 'bool',
    ['pointer', 'pointer', 'bool', 'bool'], 'thiscall'
  );
  const EncAndDesMediaFileGetSize = new NativeFunction(
    findExport('?GetSize@EncAndDesMediaFile@@QAEKXZ'), 'uint32', ['pointer'], 'thiscall'
  );
  const EncAndDesMediaFileRead = new NativeFunction(
    findExport('?Read@EncAndDesMediaFile@@QAEKPAEK_J@Z'), 'uint32',
    ['pointer', 'pointer', 'uint32', 'uint64'], 'thiscall'
  );

  const jobs = readJobs(JOBS_PATH);
  if (!Array.isArray(jobs) || jobs.length === 0) {
    emit({ t: 'fatal', err: '任务清单为空' });
    return;
  }

  for (const job of jobs) {
    try {
      emit({ t: 'start', id: job.id });

      const obj = Memory.alloc(0x28);
      EncAndDesMediaFileConstructor(obj);

      let ok = false;
      let got = 0;
      let data = null;
      try {
        const fileNameUtf16 = Memory.allocUtf16String(job.src);
        ok = EncAndDesMediaFileOpen(obj, fileNameUtf16, 1, 0);
        if (ok) {
          const fileSize = EncAndDesMediaFileGetSize(obj);
          if (fileSize > 0) {
            const buffer = Memory.alloc(fileSize);
            got = EncAndDesMediaFileRead(obj, buffer, fileSize, 0);
            if (got > 0) {
              data = buffer.readByteArray(got);
            }
          }
        }
      } finally {
        EncAndDesMediaFileDestructor(obj);
      }

      if (!ok) {
        emit({ t: 'fail', id: job.id, err: '打开加密文件失败（不是有效的加密格式或文件不可读）' });
        continue;
      }
      if (data === null) {
        emit({ t: 'fail', id: job.id, err: '读取内容为空' });
        continue;
      }

      writeBytes(job.tmp, data);
      emit({ t: 'done', id: job.id, size: got });
    } catch (e) {
      send({ t: 'fail', id: job.id, err: String((e && e.message) || e) });
    }
  }

  emit({ t: 'finished' });
}

try {
  main();
} catch (e) {
  send({ t: 'fatal', err: String((e && e.stack) || e) });
}
