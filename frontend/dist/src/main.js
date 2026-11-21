// Music Decryptor · desktop state, Wails bridge and task views
"use strict";

const rt = window.runtime;
const app = window.go.main.App;
const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => Array.from(document.querySelectorAll(selector));

const STATUS_LABELS = { pending: "待处理", running: "解密中", success: "已完成", skipped: "已跳过", failed: "失败" };
const PAGE_LABELS = { home: "工作台", history: "任务记录", settings: "偏好设置", about: "关于与诊断" };
const HISTORY_KEY = "musicDecryptor.taskHistory.v1";
const RECENT_KEY = "musicDecryptor.recentPaths.v1";
const MAX_RENDERED_FILES = 400;

const state = {
  items: new Map(),
  order: [],
  running: false,
  scanning: false,
  env: null,
  theme: "dark",
  settings: null,
  currentOptions: null,
  timer: null,
  elapsedStart: 0,
  logs: [],
  history: [],
  recent: { input: [], output: [] },
  fileRenderFrame: 0,
  logRenderFrame: 0,
  historySaved: false,
  clearHistoryArmed: false,
  update: null,
  updateDismissed: false,
  updating: false,
};

window.addEventListener("DOMContentLoaded", async () => {
  bindTitlebar();
  bindNavigation();
  bindDropzone();
  bindControls();
  bindEngineEvents();
  loadLocalData();
  renderHistory();
  renderRecentPaths();
  await loadSettings();
  await refreshEnvironment();
  window.setInterval(refreshEnvironment, 6000);
  checkForUpdate(false);
});

// ---------- Window and navigation ----------
function bindTitlebar() {
  $("#btnMin").addEventListener("click", () => rt.WindowMinimise());
  $("#btnMax").addEventListener("click", () => rt.WindowToggleMaximise());
  $("#btnClose").addEventListener("click", () => rt.Quit());
  $("#btnTheme").addEventListener("click", () => setTheme(state.theme === "dark" ? "light" : "dark", true));
}

function bindNavigation() {
  $$(".nav-item").forEach((button) => button.addEventListener("click", () => navigate(button.dataset.page)));
}

function navigate(page) {
  $$(".nav-item").forEach((button) => button.classList.toggle("active", button.dataset.page === page));
  $$(".page").forEach((section) => section.classList.toggle("active", section.id === `page-${page}`));
  $("#titlebarPage").textContent = PAGE_LABELS[page] || "工作台";
  $(".main").scrollTop = 0;
  if (page === "history") renderHistory();
  if (page === "about") refreshEnvironment();
}

// ---------- Controls ----------
function bindControls() {
  $("#btnPickInput").addEventListener("click", pickInput);
  $("#btnDropBrowse").addEventListener("click", (event) => { event.stopPropagation(); pickInput(); });
  $("#btnPickOutput").addEventListener("click", pickOutput);
  $("#btnSetOutput").addEventListener("click", pickDefaultOutput);
  $("#btnOpenOutput").addEventListener("click", openOutputDirectory);
  $("#btnScan").addEventListener("click", scanFiles);
  $("#btnStart").addEventListener("click", () => startDecrypt());
  $("#btnCancel").addEventListener("click", async () => {
    $("#btnCancel").disabled = true;
    await app.CancelDecrypt();
    $("#taskStateText").textContent = "正在停止，等待当前文件处理结束";
  });
  $("#btnRetryFailed").addEventListener("click", retryFailed);

  $("#fileSearch").addEventListener("input", scheduleFileRender);
  $("#statusFilter").addEventListener("change", scheduleFileRender);
  $("#sortFiles").addEventListener("change", scheduleFileRender);
  $("#fileList").addEventListener("click", onFileAction);

  $("#logToggle").addEventListener("click", (event) => {
    if (event.target.closest(".log-actions")) return;
    toggleLogPanel();
  });
  $("#btnLogChevron").addEventListener("click", toggleLogPanel);
  $("#logFilter").addEventListener("change", renderLogs);
  $("#btnCopyLog").addEventListener("click", copyLogs);
  $("#btnExportLog").addEventListener("click", exportLogs);
  $("#btnClearLog").addEventListener("click", clearLogs);

  $("#btnRefreshEnv").addEventListener("click", refreshEnvironment);
  $("#btnAboutRefresh").addEventListener("click", refreshEnvironment);
  $("#btnCopyDiagnostics").addEventListener("click", copyDiagnostics);
  $("#btnCheckUpdate").addEventListener("click", () => checkForUpdate(true));
  $("#btnUpdateNow").addEventListener("click", installUpdate);
  $("#btnUpdateLater").addEventListener("click", dismissUpdate);
  $("#btnSaveSettings").addEventListener("click", () => saveSettings(true));
  $$("[data-theme-choice]").forEach((button) => button.addEventListener("click", () => setTheme(button.dataset.themeChoice, false)));

  $("#btnClearHistory").addEventListener("click", clearHistory);
  $("#historyList").addEventListener("click", onHistoryAction);
  $("#inpInput").addEventListener("change", () => addRecentPath("input", cleanPathInput($("#inpInput"))));
  $("#inpOutput").addEventListener("change", () => addRecentPath("output", cleanPathInput($("#inpOutput"))));
}

function bindDropzone() {
  const zone = $("#dropzone");
  zone.addEventListener("click", (event) => { if (!event.target.closest("button")) pickInput(); });
  zone.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") { event.preventDefault(); pickInput(); }
  });
  ["dragenter", "dragover"].forEach((name) => zone.addEventListener(name, () => zone.classList.add("drag-over")));
  ["dragleave", "drop"].forEach((name) => zone.addEventListener(name, () => zone.classList.remove("drag-over")));
  rt.OnFileDrop(async (_x, _y, paths) => {
    zone.classList.remove("drag-over");
    if (!paths || !paths.length || state.running) return;
    try {
      const directory = await app.ResolveDropPath(paths[0]);
      if (!directory) return toast("warn", "无法读取拖入内容", "请改为使用“选择文件夹”。");
      $("#inpInput").value = directory;
      addRecentPath("input", directory);
      toast("info", "已载入来源目录", paths.length > 1 ? `已接收 ${paths.length} 个拖入项，使用首个项目所在目录。` : directory);
    } catch (error) {
      toast("bad", "拖入失败", errorText(error));
    }
  });
}

async function pickInput() {
  if (state.running) return;
  const path = await rt.OpenDirectoryDialog({ Title: "选择包含 MFLAC / MGG 文件的目录", CanCreateDirectories: false });
  if (path) {
    $("#inpInput").value = path;
    if (!$("#inpOutput").value && state.settings?.outputDir) $("#inpOutput").value = state.settings.outputDir;
    addRecentPath("input", path);
  }
}

async function pickOutput() {
  const path = await rt.OpenDirectoryDialog({ Title: "选择输出目录", CanCreateDirectories: true });
  if (path) { $("#inpOutput").value = path; addRecentPath("output", path); }
}

async function pickDefaultOutput() {
  const path = await rt.OpenDirectoryDialog({ Title: "选择默认输出目录", CanCreateDirectories: true });
  if (path) $("#setOutput").value = path;
}

function openOutputDirectory() {
  const path = cleanPathInput($("#inpOutput"));
  if (!path) return toast("warn", "尚未设置输出目录", "请先选择转换结果的保存位置。");
  app.OpenDirectory(path);
}

// ---------- Settings ----------
async function loadSettings() {
  try {
    const settings = await app.GetSettings();
    if (!settings) return;
    state.settings = settings;
    setTheme(settings.theme || "dark", false);
    const outputDir = normalizeUserPath(settings.outputDir || "");
    $("#setOutput").value = outputDir;
    $("#setRecursive").checked = settings.recursive ?? true;
    $("#setSkipExisting").checked = settings.skipExisting ?? true;
    $("#setKeepStructure").checked = settings.keepStructure ?? true;
    $("#setAutoFixExt").checked = settings.autoFixExt ?? true;
    $("#setOpenOnComplete").checked = settings.openOnComplete ?? false;
    $("#optRecursive").checked = settings.recursive ?? true;
    $("#optSkipExisting").checked = settings.skipExisting ?? true;
    $("#optKeepStructure").checked = settings.keepStructure ?? true;
    $("#optAutoFixExt").checked = settings.autoFixExt ?? true;
    $("#inpOutput").value = outputDir;
  } catch (error) {
    toast("bad", "设置读取失败", errorText(error));
  }
}

async function saveSettings(showToast) {
  const outputDir = cleanPathInput($("#setOutput"));
  const payload = {
    outputDir,
    recursive: $("#setRecursive").checked,
    skipExisting: $("#setSkipExisting").checked,
    keepStructure: $("#setKeepStructure").checked,
    autoFixExt: $("#setAutoFixExt").checked,
    openOnComplete: $("#setOpenOnComplete").checked,
    theme: state.theme,
  };
  try {
    state.settings = await app.SaveSettings(payload);
    if (!state.running) {
      $("#optRecursive").checked = payload.recursive;
      $("#optSkipExisting").checked = payload.skipExisting;
      $("#optKeepStructure").checked = payload.keepStructure;
      $("#optAutoFixExt").checked = payload.autoFixExt;
      if (!normalizeUserPath($("#inpOutput").value)) $("#inpOutput").value = payload.outputDir;
    }
    if (showToast) toast("ok", "设置已保存", "新任务将使用这些默认选项。");
  } catch (error) {
    toast("bad", "设置保存失败", errorText(error));
  }
}

function setTheme(theme, persist) {
  state.theme = theme === "light" ? "light" : "dark";
  document.documentElement.dataset.theme = state.theme;
  $("#themeIcon").setAttribute("href", state.theme === "dark" ? "#i-moon" : "#i-sun");
  $$("[data-theme-choice]").forEach((button) => button.classList.toggle("active", button.dataset.themeChoice === state.theme));
  if (persist && state.settings) saveSettings(false);
}

// ---------- Environment ----------
async function refreshEnvironment() {
  try {
    const env = await app.CheckEnvironment();
    state.env = env;
    setStatusDot($("#dotQQ"), env.qqMusicRunning ? "on" : "off");
    setStatusDot($("#dotEngine"), env.engineReady ? "on" : "busy");
    $("#envQQText").textContent = env.qqMusicRunning ? `运行中 · ${env.qqMusicArch || "架构未知"}` : "未运行";
    $("#envEngineText").textContent = env.engineReady ? `就绪 · ${env.engineArch || ""} · ${env.engineVersion || ""}` : (env.note || "准备中");
    const ready = env.qqMusicRunning && env.engineReady;
    setStatusDot($("#connectionPill .status-dot"), ready ? "on" : (env.engineReady ? "off" : "busy"));
    $("#connectionText").textContent = ready ? "环境就绪" : (!env.engineReady ? "引擎准备中" : "请启动 QQ 音乐");
    $("#diagQQ").textContent = env.qqMusicRunning ? "正在运行" : "未运行";
    $("#diagQQArch").textContent = env.qqMusicArch || "—";
    $("#diagEngine").textContent = env.engineReady ? "已就绪" : (env.note || "准备中");
    $("#diagEngineVersion").textContent = env.engineVersion ? `${env.engineVersion} · ${env.engineArch || ""}` : "—";
    $("#diagQQPath").textContent = env.qqMusicPath || "—";
    $("#diagQQPath").title = env.qqMusicPath || "";
    applyAppVersion(env.appVersion);
  } catch (error) {
    console.warn("Environment check failed", error);
  }
}

function applyAppVersion(version) {
  const text = version || "未知";
  const subtitle = $("#aboutSubtitle");
  if (subtitle) subtitle.textContent = `Music Decryptor ${text} · Go + Wails`;
  const diag = $("#diagAppVersion");
  if (diag) diag.textContent = text;
}

function renderUpdateBanner(info) {
  const available = !!info?.available;
  $("#updateBanner").hidden = false;
  $("#btnUpdateNow").hidden = !available;
  $("#btnUpdateNow").disabled = false;
  $("#btnUpdateNow").textContent = "立即更新";
  $("#btnUpdateLater").disabled = false;
  $("#btnUpdateLater").textContent = available ? "稍后" : "知道了";
  $("#updateTitle").textContent = `发现新版本 ${info?.latestVersion || ""}`.trim();
  $("#updateText").textContent = info?.message || (available ? "可以在应用内下载并安装，完成后会自动重启。" : "暂时无法安装这个版本。");
}

async function checkForUpdate(manual) {
  if (state.updating) return;
  try {
    const info = await app.CheckForUpdate();
    state.update = info || null;
    if (info?.currentVersion) applyAppVersion(info.currentVersion);
    if ((info?.available || info?.assetMissing) && (manual || !state.updateDismissed)) {
      renderUpdateBanner(info);
      return;
    }
    if (!state.updating) $("#updateBanner").hidden = true;
    if (manual) toast("ok", "已是最新版本", `当前版本 ${info?.currentVersion || "未知"}。`);
  } catch (error) {
    if (manual) toast("bad", "检查更新失败", errorText(error));
  }
}

async function installUpdate() {
  if (state.updating || !state.update?.available) return;
  if (state.running || state.scanning) {
    toast("warn", "请先停止当前任务", "更新会重启应用，进行中的解密会中断。");
    return;
  }
  state.updating = true;
  $("#btnUpdateNow").disabled = true;
  $("#btnUpdateLater").disabled = true;
  $("#updateTitle").textContent = "正在下载更新";
  $("#updateText").textContent = "正在连接 GitHub，请稍候。";
  try {
    await app.InstallUpdate();
    $("#updateTitle").textContent = "正在安装更新";
    $("#updateText").textContent = "程序即将重启，请稍候。";
  } catch (error) {
    state.updating = false;
    $("#btnUpdateNow").hidden = false;
    $("#btnUpdateNow").disabled = false;
    $("#btnUpdateNow").textContent = "重试";
    $("#btnUpdateLater").disabled = false;
    $("#btnUpdateLater").textContent = "关闭";
    $("#updateTitle").textContent = "更新没有完成";
    $("#updateText").textContent = errorText(error);
  }
}

function dismissUpdate() {
  if (state.updating) return;
  state.updateDismissed = true;
  $("#updateBanner").hidden = true;
}

function setStatusDot(element, className) {
  if (element) element.className = `status-dot ${className}`;
}

async function copyDiagnostics() {
  const env = state.env || await app.CheckEnvironment();
  const text = [
    `Music Decryptor ${env.appVersion || "未知"}`,
    `QQ Music: ${env.qqMusicRunning ? "Running" : "Not running"}`,
    `QQ Music architecture: ${env.qqMusicArch || "Unknown"}`,
    `QQ Music path: ${env.qqMusicPath || "Unknown"}`,
    `Engine ready: ${env.engineReady}`,
    `Engine: ${env.engineVersion || "Unknown"} ${env.engineArch || ""}`,
    `Note: ${env.note || ""}`,
  ].join("\n");
  await app.CopyText(text);
  toast("ok", "诊断信息已复制", "可粘贴到问题反馈中。");
}

// ---------- Scan and decrypt ----------
function collectOptions() {
  return {
    inputDir: cleanPathInput($("#inpInput")),
    outputDir: cleanPathInput($("#inpOutput")),
    recursive: $("#optRecursive").checked,
    skipExisting: $("#optSkipExisting").checked,
    keepStructure: $("#optKeepStructure").checked,
    autoFixExt: $("#optAutoFixExt").checked,
  };
}

function validateOptions(options) {
  if (!options.inputDir) { toast("warn", "请选择来源目录", "来源目录中应包含 .mflac 或 .mgg 文件。"); return false; }
  if (!options.outputDir) { toast("warn", "请选择输出目录", "转换结果需要明确的保存位置。"); return false; }
  if (normalizePath(options.inputDir) === normalizePath(options.outputDir)) {
    toast("warn", "输出目录需要与来源目录分开", "请选择另一个目录，避免原始文件与转换结果混在一起。"); return false;
  }
  return true;
}

async function scanFiles() {
  if (state.running || state.scanning) return;
  const options = collectOptions();
  if (!validateOptions(options)) return;
  setScanning(true);
  try {
    const items = await app.ScanFiles(options);
    setItems(items || []);
    const counts = countItems();
    renderStats({ total: counts.total, success: 0, skipped: counts.skipped, failed: 0 });
    setProgress(counts.total ? Math.round(counts.skipped / counts.total * 100) : 0, `扫描到 ${counts.total} 个文件 · ${counts.pending} 个待处理`);
    $("#taskStateTitle").textContent = counts.total ? "扫描完成" : "未发现目标文件";
    $("#taskStateText").textContent = counts.total ? `${counts.pending} 个可处理，${counts.skipped} 个将跳过` : "请确认目录中存在 .mflac / .mgg 文件";
    state.currentOptions = options;
    addRecentPath("input", options.inputDir);
    addRecentPath("output", options.outputDir);
    toast(counts.total ? "ok" : "warn", counts.total ? "扫描完成" : "没有找到音频", counts.total ? `${counts.total} 个文件，共 ${fmtSize(totalBytes())}。` : "仅支持 .mflac 与 .mgg 文件。");
  } catch (error) {
    toast("bad", "扫描失败", errorText(error));
  } finally {
    setScanning(false);
  }
}

function setScanning(value) {
  state.scanning = value;
  $("#btnScan").disabled = value || state.running;
  $("#btnScan span").textContent = value ? "正在扫描" : "扫描文件";
  $("#btnStart").disabled = value || state.running;
}

async function startDecrypt(includeIDs = []) {
  if (state.running || state.scanning) return;
  const options = collectOptions();
  if (!validateOptions(options)) return;
  if (includeIDs.length) options.includeIDs = includeIDs;
  if (state.env && !state.env.qqMusicRunning) {
    return toast("bad", "QQ 音乐尚未运行", "请先启动 QQ 音乐桌面客户端，再开始解密。");
  }
  if (state.env && !state.env.engineReady) {
    return toast("warn", "解密引擎尚未就绪", "请稍等片刻，或点击运行环境旁的刷新按钮。");
  }

  state.currentOptions = options;
  state.historySaved = false;
  resetTaskUI();
  setRunning(true);
  addRecentPath("input", options.inputDir);
  addRecentPath("output", options.outputDir);
  try {
    await app.StartDecrypt(options);
  } catch (error) {
    setRunning(false);
    toast("bad", "任务启动失败", errorText(error));
  }
}

function retryFailed() {
  const failedIDs = allItems().filter((item) => item.status === "failed").map((item) => item.id);
  if (!failedIDs.length) return;
  startDecrypt(failedIDs);
}

function resetTaskUI() {
  state.items.clear();
  state.order = [];
  state.logs = [];
  app.ClearLog();
  renderFiles();
  renderLogs();
  renderStats({ total: 0, success: 0, skipped: 0, failed: 0 });
  setProgress(0, "正在准备任务");
  $("#stElapsed").innerHTML = '0.0<span>s</span>';
  $("#taskStateTitle").textContent = "正在准备";
  $("#taskStateText").textContent = "扫描文件并初始化本地解密引擎";
}

function setRunning(value) {
  state.running = value;
  $("#btnStart").disabled = value || state.scanning;
  $("#btnScan").disabled = value || state.scanning;
  $("#btnCancel").disabled = !value;
  $("#btnRetryFailed").disabled = value || !allItems().some((item) => item.status === "failed");
  $(".progress-panel").classList.toggle("running", value);
  if (value) {
    state.elapsedStart = Date.now();
    $("#taskStateTitle").textContent = "正在解密";
    $("#taskStateText").textContent = "请保持 QQ 音乐客户端运行";
    window.clearInterval(state.timer);
    state.timer = window.setInterval(() => setElapsed((Date.now() - state.elapsedStart) / 1000), 100);
  } else {
    window.clearInterval(state.timer);
    state.timer = null;
    $("#btnRetryFailed").disabled = !allItems().some((item) => item.status === "failed");
  }
}

// ---------- Engine events ----------
function bindEngineEvents() {
  rt.EventsOn("engine:event", (event) => {
    if (!event) return;
    switch (event.kind) {
      case "files": onFiles(event.data); break;
      case "file": onFile(event.data); break;
      case "progress": onProgress(event.data); break;
      case "summary": onSummary(event.data); break;
      case "log": onLog(event.data); break;
      case "error": onEngineError(event.data); break;
    }
  });
  rt.EventsOn("update:progress", (payload) => {
    if (!state.updating || !payload) return;
    const received = Number(payload.received || 0);
    const total = Number(payload.total || 0);
    $("#updateTitle").textContent = "正在下载更新";
    $("#updateText").textContent = total ? `已下载 ${fmtSize(received)} / ${fmtSize(total)}` : `已下载 ${fmtSize(received)}`;
  });
}

function onFiles(data) {
  const items = Array.isArray(data) ? data : (data?.items || []);
  setItems(items);
}

function setItems(items) {
  state.items.clear();
  state.order = [];
  items.forEach((item) => { state.items.set(item.id, item); state.order.push(item.id); });
  renderFiles();
}

function onFile(item) {
  if (!item || !item.id) return;
  if (!state.items.has(item.id)) state.order.push(item.id);
  state.items.set(item.id, item);
  if (!state.running) $("#btnRetryFailed").disabled = !allItems().some((entry) => entry.status === "failed");
  scheduleFileRender();
}

function onProgress(progress) {
  if (!progress) return;
  const done = progress.done ?? ((progress.success || 0) + (progress.skipped || 0) + (progress.failed || 0));
  const total = progress.total || 0;
  renderStats(progress);
  setElapsed((progress.elapsedMs || 0) / 1000);
  setProgress(total ? Math.round(done / total * 100) : 0, `${done} / ${total} 个文件已处理`);
}

function onSummary(summary) {
  setRunning(false);
  const data = summary || {};
  const total = data.total ?? allItems().length;
  const done = (data.success || 0) + (data.skipped || 0) + (data.failed || 0);
  renderStats(data);
  setElapsed((data.elapsedMs || 0) / 1000);
  setProgress(total ? Math.round(done / total * 100) : 0, data.cancelled ? `任务已停止 · ${done} / ${total}` : `任务结束 · ${done} / ${total}`);
  $("#taskStateTitle").textContent = data.cancelled ? "任务已停止" : (data.failed ? "任务完成，存在失败项" : "任务完成");
  $("#taskStateText").textContent = data.failed ? "可筛选失败文件，并仅重试失败项" : "所有可处理文件均已处理";
  if (data.cancelled) {
    toast("warn", "任务已停止", `成功 ${data.success || 0} · 跳过 ${data.skipped || 0} · 失败 ${data.failed || 0}`);
  } else if (data.failed) {
    toast("bad", "部分文件处理失败", `成功 ${data.success || 0} · 失败 ${data.failed}，可查看日志或仅重试失败项。`);
  } else {
    toast("ok", "解密完成", `成功 ${data.success || 0} · 跳过 ${data.skipped || 0} · 失败 0`);
  }
  recordHistory(data);
  if (!data.cancelled && !data.failed && state.settings?.openOnComplete && (data.success || 0) > 0) {
    app.OpenDirectory(normalizeUserPath(state.currentOptions?.outputDir || $("#inpOutput").value));
  }
}

function onEngineError(data) {
  setRunning(false);
  $("#taskStateTitle").textContent = "任务中断";
  $("#taskStateText").textContent = data?.message || "请查看运行日志";
  toast("bad", "任务执行出错", data?.message || "未知错误");
}

// ---------- File table ----------
function allItems() {
  return state.order.map((id) => state.items.get(id)).filter(Boolean);
}

function countItems() {
  const counts = { total: 0, pending: 0, running: 0, success: 0, skipped: 0, failed: 0 };
  allItems().forEach((item) => { counts.total++; counts[item.status] = (counts[item.status] || 0) + 1; });
  return counts;
}

function totalBytes() {
  return allItems().reduce((sum, item) => sum + (Number(item.size) || 0), 0);
}

function scheduleFileRender() {
  if (state.fileRenderFrame) return;
  state.fileRenderFrame = requestAnimationFrame(() => { state.fileRenderFrame = 0; renderFiles(); });
}

function renderFiles() {
  const list = $("#fileList");
  const all = allItems();
  const query = $("#fileSearch").value.trim().toLocaleLowerCase();
  const filter = $("#statusFilter").value;
  let items = all.filter((item) => {
    const matchesQuery = !query || `${item.name} ${item.relPath}`.toLocaleLowerCase().includes(query);
    return matchesQuery && (filter === "all" || item.status === filter);
  });
  const sort = $("#sortFiles").value;
  const statusOrder = { running: 0, failed: 1, pending: 2, skipped: 3, success: 4 };
  items = items.slice().sort((a, b) => {
    if (sort === "name") return a.name.localeCompare(b.name, "zh-CN");
    if (sort === "size") return (b.size || 0) - (a.size || 0);
    if (sort === "status") return (statusOrder[a.status] ?? 9) - (statusOrder[b.status] ?? 9) || a.relPath.localeCompare(b.relPath);
    return a.relPath.localeCompare(b.relPath, "zh-CN");
  });

  $("#listCount").textContent = String(all.length);
  $("#fileSummary").innerHTML = all.length
    ? `${items.length} / ${all.length} 个文件 · ${fmtSize(totalBytes())}${items.length > MAX_RENDERED_FILES ? ' <span class="list-limit-note">仅显示前 400 项，请筛选缩小范围</span>' : ""}`
    : "等待扫描";
  if (!state.running) $("#btnRetryFailed").disabled = !all.some((item) => item.status === "failed");

  if (!items.length) {
    const isFiltered = all.length > 0;
    list.innerHTML = `<div class="empty-state"><div class="empty-icon"><svg class="icon"><use href="#i-${isFiltered ? "search" : "music"}"/></svg></div><strong>${isFiltered ? "没有匹配的文件" : "还没有文件"}</strong><p>${isFiltered ? "尝试更换关键词或状态筛选" : "选择来源目录，点击“扫描文件”预览任务清单"}</p></div>`;
    return;
  }
  list.innerHTML = items.slice(0, MAX_RENDERED_FILES).map(rowHTML).join("");
}

function rowHTML(item) {
  const directory = parentPath(item.relPath);
  const detail = item.message || `${extension(item.name).replace(".", "").toUpperCase()} · ${statusDescription(item.status)}`;
  const outputAction = item.status === "success" && item.outPath
    ? `<button class="icon-btn" data-action="open-output" data-fid="${esc(item.id)}" title="打开输出文件"><svg class="icon"><use href="#i-open"/></svg></button>` : "";
  return `<div class="file-row" data-fid="${esc(item.id)}">
    <div class="file-name"><span class="file-type-icon"><svg class="icon"><use href="#i-music"/></svg></span><div class="file-name-copy"><strong title="${esc(item.name)}">${esc(item.name)}</strong><span title="${esc(detail)}">${esc(detail)}</span></div></div>
    <span class="file-size">${fmtSize(item.size)}</span>
    <span class="file-path" title="${esc(item.relPath)}">${esc(directory)}</span>
    <span class="status-chip ${esc(item.status)}">${STATUS_LABELS[item.status] || esc(item.status)}</span>
    <div class="row-actions">${outputAction}<button class="icon-btn" data-action="reveal-source" data-fid="${esc(item.id)}" title="在资源管理器中显示原文件"><svg class="icon"><use href="#i-folder"/></svg></button></div>
  </div>`;
}

function onFileAction(event) {
  const button = event.target.closest("button[data-action]");
  if (!button) return;
  const item = state.items.get(button.dataset.fid);
  if (!item) return;
  if (button.dataset.action === "open-output") app.OpenPath(item.outPath);
  if (button.dataset.action === "reveal-source") app.OpenPath(item.srcPath);
}

function statusDescription(status) {
  return { pending: "等待处理", running: "正在处理", success: "转换结果已保存", skipped: "未重复处理", failed: "处理未完成" }[status] || "";
}

// ---------- Progress ----------
function renderStats(progress) {
  $("#stTotal").textContent = progress.total ?? 0;
  $("#stSuccess").textContent = progress.success ?? 0;
  $("#stSkipped").textContent = progress.skipped ?? 0;
  $("#stFailed").textContent = progress.failed ?? 0;
}

function setProgress(percent, text) {
  const value = Math.max(0, Math.min(100, Number(percent) || 0));
  $("#overallFill").style.width = `${value}%`;
  $("#overallPercent").textContent = `${value}%`;
  $("#overallText").textContent = text;
}

function setElapsed(seconds) {
  $("#stElapsed").innerHTML = `${Math.max(0, seconds).toFixed(1)}<span>s</span>`;
}

// ---------- Logs ----------
function onLog(data) {
  state.logs.push({ level: data?.level || "info", message: data?.message || "", time: new Date() });
  if (state.logs.length > 1200) state.logs.splice(0, state.logs.length - 1200);
  $("#logCount").textContent = String(state.logs.length);
  if (state.logRenderFrame) return;
  state.logRenderFrame = requestAnimationFrame(() => { state.logRenderFrame = 0; renderLogs(); });
}

function filteredLogs() {
  const filter = $("#logFilter").value;
  return state.logs.filter((line) => filter === "all" || line.level === filter);
}

function renderLogs() {
  const logs = filteredLogs();
  const body = $("#logBody");
  $("#logCount").textContent = String(state.logs.length);
  if (!logs.length) {
    body.innerHTML = `<div class="log-empty">${state.logs.length ? "当前级别没有日志" : "任务运行时的详细信息会显示在这里"}</div>`;
    return;
  }
  body.innerHTML = logs.slice(-800).map((line) => `<div class="log-line lv-${esc(line.level)}"><span class="log-time">${formatClock(line.time)}</span><span class="log-level">${esc(line.level)}</span><span class="log-message">${esc(line.message)}</span></div>`).join("");
  if ($("#logAutoScroll").checked) body.scrollTop = body.scrollHeight;
}

function toggleLogPanel() {
  $("#logCard").classList.toggle("collapsed");
}

async function copyLogs() {
  const text = filteredLogs().map((line) => `${formatClock(line.time)} [${line.level}] ${line.message}`).join("\n");
  if (!text) return toast("warn", "没有可复制的日志", "运行任务后再试。");
  await app.CopyText(text);
  toast("ok", "日志已复制", `已复制 ${filteredLogs().length} 条记录。`);
}

async function exportLogs() {
  try {
    const path = await app.ExportLog();
    if (path) toast("ok", "日志已导出", path);
  } catch (error) {
    toast("bad", "日志导出失败", errorText(error));
  }
}

async function clearLogs() {
  state.logs = [];
  await app.ClearLog();
  renderLogs();
}

// ---------- History and recent paths ----------
function loadLocalData() {
  try { state.history = JSON.parse(localStorage.getItem(HISTORY_KEY) || "[]"); } catch { state.history = []; }
  try { state.recent = { input: [], output: [], ...JSON.parse(localStorage.getItem(RECENT_KEY) || "{}") }; } catch { state.recent = { input: [], output: [] }; }
}

function recordHistory(summary) {
  if (state.historySaved || !state.currentOptions) return;
  state.historySaved = true;
  const entry = {
    id: `${Date.now()}-${Math.random().toString(16).slice(2, 7)}`,
    timestamp: new Date().toISOString(),
    options: { ...state.currentOptions, includeIDs: undefined },
    total: summary.total ?? allItems().length,
    success: summary.success || 0,
    skipped: summary.skipped || 0,
    failed: summary.failed || 0,
    cancelled: !!summary.cancelled,
    elapsedMs: summary.elapsedMs || Math.max(0, Date.now() - state.elapsedStart),
  };
  state.history.unshift(entry);
  state.history = state.history.slice(0, 20);
  persistHistory();
  renderHistory();
}

function persistHistory() {
  try { localStorage.setItem(HISTORY_KEY, JSON.stringify(state.history)); } catch (error) { console.warn(error); }
}

function renderHistory() {
  const entries = state.history || [];
  $("#historyTasks").textContent = String(entries.length);
  $("#historySuccess").textContent = String(entries.reduce((sum, entry) => sum + (entry.success || 0), 0));
  $("#historyTime").textContent = fmtDuration(entries.reduce((sum, entry) => sum + (entry.elapsedMs || 0), 0));
  $("#historyCount").textContent = String(entries.length);
  $("#historyCount").hidden = !entries.length;
  $("#btnClearHistory").disabled = !entries.length;
  if (!entries.length) {
    $("#historyList").innerHTML = '<div class="history-empty"><div class="empty-icon"><svg class="icon"><use href="#i-history"/></svg></div><strong>暂无任务记录</strong><p>完成一次解密后，可从这里快速恢复任务配置</p></div>';
    return;
  }
  $("#historyList").innerHTML = entries.map((entry) => {
    const date = new Date(entry.timestamp);
    return `<div class="history-row">
      <div class="history-time"><strong>${esc(date.toLocaleDateString("zh-CN"))}</strong><span>${esc(date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }))}${entry.cancelled ? " · 已停止" : ""}</span></div>
      <div class="history-paths"><span title="${esc(entry.options.inputDir)}">来源：${esc(entry.options.inputDir)}</span><span title="${esc(entry.options.outputDir)}">输出：${esc(entry.options.outputDir)}</span></div>
      <div class="history-result"><span>共 <b>${entry.total || 0}</b></span><span class="ok">成功 <b>${entry.success || 0}</b></span><span>跳过 <b>${entry.skipped || 0}</b></span><span class="bad">失败 <b>${entry.failed || 0}</b></span></div>
      <span class="history-duration">${fmtDuration(entry.elapsedMs || 0)}</span>
      <div class="history-actions"><button class="button secondary compact" data-history-action="restore" data-history-id="${esc(entry.id)}" title="恢复目录和选项"><svg class="icon"><use href="#i-arrow"/></svg>载入</button><button class="icon-btn" data-history-action="open" data-history-id="${esc(entry.id)}" title="打开输出目录"><svg class="icon"><use href="#i-folder"/></svg></button></div>
    </div>`;
  }).join("");
}

function onHistoryAction(event) {
  const button = event.target.closest("button[data-history-action]");
  if (!button) return;
  const entry = state.history.find((item) => item.id === button.dataset.historyId);
  if (!entry) return;
  if (button.dataset.historyAction === "open") return app.OpenDirectory(normalizeUserPath(entry.options.outputDir));
  if (state.running) return toast("warn", "当前任务仍在运行", "任务结束后再载入历史配置。");
  $("#inpInput").value = normalizeUserPath(entry.options.inputDir || "");
  $("#inpOutput").value = normalizeUserPath(entry.options.outputDir || "");
  $("#optRecursive").checked = entry.options.recursive ?? true;
  $("#optSkipExisting").checked = entry.options.skipExisting ?? true;
  $("#optKeepStructure").checked = entry.options.keepStructure ?? true;
  $("#optAutoFixExt").checked = entry.options.autoFixExt ?? true;
  navigate("home");
  toast("ok", "任务配置已载入", "点击“扫描文件”即可重新检查文件。");
}

function clearHistory() {
  const button = $("#btnClearHistory");
  if (!state.clearHistoryArmed) {
    state.clearHistoryArmed = true;
    button.innerHTML = '<svg class="icon"><use href="#i-alert"/></svg>再次点击确认';
    window.setTimeout(() => {
      if (!state.clearHistoryArmed) return;
      state.clearHistoryArmed = false;
      button.innerHTML = '<svg class="icon"><use href="#i-trash"/></svg>清空记录';
    }, 3500);
    return;
  }
  state.clearHistoryArmed = false;
  state.history = [];
  persistHistory();
  button.innerHTML = '<svg class="icon"><use href="#i-trash"/></svg>清空记录';
  renderHistory();
  toast("ok", "任务记录已清空", "音频文件和设置未受影响。");
}

function cleanPathInput(element) {
  const value = normalizeUserPath(element.value);
  if (element.value !== value) element.value = value;
  return value;
}

function addRecentPath(kind, path) {
  const clean = normalizeUserPath(path);
  if (!clean) return;
  const list = state.recent[kind] || [];
  state.recent[kind] = [clean, ...list.filter((item) => normalizePath(item) !== normalizePath(clean))].slice(0, 5);
  try { localStorage.setItem(RECENT_KEY, JSON.stringify(state.recent)); } catch { /* non-critical */ }
  renderRecentPaths();
}

function renderRecentPaths() {
  [["input", "#recentInputs"], ["output", "#recentOutputs"]].forEach(([kind, selector]) => {
    const datalist = $(selector);
    datalist.innerHTML = "";
    (state.recent[kind] || []).forEach((path) => {
      const option = document.createElement("option");
      option.value = path;
      datalist.appendChild(option);
    });
  });
}

// ---------- Formatting ----------
function fmtSize(value) {
  const size = Number(value) || 0;
  if (size < 1024) return `${size} B`;
  if (size < 1024 ** 2) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 ** 3) return `${(size / 1024 ** 2).toFixed(2)} MB`;
  return `${(size / 1024 ** 3).toFixed(2)} GB`;
}

function fmtDuration(milliseconds) {
  const seconds = Math.max(0, milliseconds / 1000);
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${Math.round(seconds % 60)}s`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

function formatClock(date) {
  const value = date instanceof Date ? date : new Date(date);
  return value.toLocaleTimeString("zh-CN", { hour12: false });
}

function parentPath(path) {
  const value = String(path || "");
  const index = Math.max(value.lastIndexOf("\\"), value.lastIndexOf("/"));
  return index > 0 ? value.slice(0, index) : "根目录";
}

function extension(path) {
  const name = String(path || "");
  const index = name.lastIndexOf(".");
  return index >= 0 ? name.slice(index) : "";
}

function normalizePath(path) {
  return pathCompareKey(path);
}

function errorText(error) {
  if (typeof error === "string") return error;
  return String(error?.message || error || "未知错误").replace(/^Error:\s*/i, "");
}

function esc(value) {
  return String(value ?? "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;");
}

function toast(kind, title, body) {
  const element = document.createElement("div");
  const icon = kind === "ok" ? "check" : (kind === "bad" || kind === "warn" ? "alert" : "info");
  element.className = `toast ${kind}`;
  element.innerHTML = `<svg class="icon"><use href="#i-${icon}"/></svg><div><strong>${esc(title)}</strong>${body ? `<p>${esc(body)}</p>` : ""}</div>`;
  $("#toasts").appendChild(element);
  while ($("#toasts").childElementCount > 4) $("#toasts").firstElementChild.remove();
  window.setTimeout(() => {
    element.classList.add("out");
    window.setTimeout(() => element.remove(), 220);
  }, kind === "bad" ? 6000 : 3800);
}
