package app.musicdecryptor.ui

import android.Manifest
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.material3.SwipeToDismissBox
import androidx.compose.material3.SwipeToDismissBoxValue
import androidx.compose.material3.rememberSwipeToDismissBoxState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import app.musicdecryptor.engine.Phase
import app.musicdecryptor.engine.TrackStatus
import app.musicdecryptor.engine.outputLabel
import app.musicdecryptor.theme.Bg
import app.musicdecryptor.theme.Danger
import app.musicdecryptor.theme.EyebrowStyle
import app.musicdecryptor.theme.Lime
import app.musicdecryptor.theme.Line
import app.musicdecryptor.theme.Success
import app.musicdecryptor.theme.Surface
import app.musicdecryptor.theme.Text
import app.musicdecryptor.theme.TextDim
import app.musicdecryptor.theme.TextFaint
import app.musicdecryptor.theme.TitleStyle
import kotlinx.coroutines.delay

private enum class Filter { ALL, OK, BAD }
private sealed interface Sheet {
    data object Output : Sheet
    data object Keys : Sheet
    data class Detail(val id: String) : Sheet
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeRoute(vm: HomeViewModel = viewModel()) {
    val context = LocalContext.current
    val session by vm.session.collectAsStateWithLifecycle()
    val settings by vm.settings.collectAsStateWithLifecycle()
    val keys by vm.keys.collectAsStateWithLifecycle()
    val toast by vm.toast.collectAsStateWithLifecycle()
    val selfTest by vm.selfTest.collectAsStateWithLifecycle()
    var filter by remember { mutableStateOf(Filter.ALL) }
    var sheet by remember { mutableStateOf<Sheet?>(null) }
    var askedNotification by remember { mutableStateOf(false) }
    val haptic = LocalHapticFeedback.current

    val pickFiles = rememberLauncherForActivityResult(ActivityResultContracts.OpenMultipleDocuments()) { uris ->
        uris.forEach { uri ->
            runCatching {
                context.contentResolver.takePersistableUriPermission(
                    uri,
                    android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION,
                )
            }
        }
        if (uris.isNotEmpty()) vm.add(uris)
    }
    val pickTree = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocumentTree()) { uri ->
        if (uri != null) {
            val flags = android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION or
                android.content.Intent.FLAG_GRANT_WRITE_URI_PERMISSION
            runCatching { context.contentResolver.takePersistableUriPermission(uri, flags) }
            vm.setTree(uri)
        }
    }
    val pickKey = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) vm.importKeys(uri)
    }
    val askNotification = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {
        vm.start(context)
    }

    LaunchedEffect(session.phase, session.finishedAt) {
        if (session.phase == Phase.DONE && session.finishedAt != 0L) {
            haptic.performHapticFeedback(HapticFeedbackType.LongPress)
        }
    }
    LaunchedEffect(toast) {
        if (toast != null) {
            delay(2800)
            vm.consumeToast()
        }
    }

    val statusTop = WindowInsets.statusBars.asPaddingValues().calculateTopPadding()
    val navBottom = WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()
    val running = session.phase == Phase.RUNNING
    val visible = session.tracks.filter {
        when (filter) {
            Filter.ALL -> true
            Filter.OK -> it.status == TrackStatus.SUCCESS
            Filter.BAD -> it.status == TrackStatus.FAILED
        }
    }
    val actionable = session.tracks.any {
        it.status == TrackStatus.PENDING && it.plan != app.musicdecryptor.engine.KeyPlan.MISSING &&
            it.plan != app.musicdecryptor.engine.KeyPlan.MAC
    }
    val allMissing = session.tracks.isNotEmpty() && session.pending > 0 && !actionable && !running

    Box(Modifier.fillMaxSize().background(Bg)) {
        Box(
            Modifier
                .fillMaxWidth()
                .height(360.dp)
                .background(Brush.radialGradient(listOf(Lime.copy(alpha = 0.09f), Color.Transparent))),
        )
        PullToRefreshBox(
            isRefreshing = session.scanning,
            onRefresh = vm::reprobe,
            modifier = Modifier.fillMaxSize(),
        ) {
            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(start = 20.dp, end = 20.dp, top = statusTop + 12.dp, bottom = navBottom + 132.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                item { Header(session, running) }
                item {
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceEvenly) {
                        StatPill(session.success, "成功", Success)
                        StatPill(session.skipped, "跳过", TextDim)
                        StatPill(session.failed, "失败", Danger)
                    }
                }
                item {
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        FilterChip("全部", filter == Filter.ALL) { filter = Filter.ALL }
                        FilterChip("成功", filter == Filter.OK) { filter = Filter.OK }
                        FilterChip("失败", filter == Filter.BAD) { filter = Filter.BAD }
                    }
                }
                if (!session.banner.isNullOrBlank()) {
                    item {
                        Text(
                            session.banner!!,
                            color = Lime,
                            fontSize = 13.sp,
                            modifier = Modifier
                                .fillMaxWidth()
                                .background(Surface, androidx.compose.foundation.shape.RoundedCornerShape(14.dp))
                                .padding(12.dp),
                        )
                    }
                }
                if (session.tracks.isEmpty()) {
                    item { EmptyState() }
                } else if (visible.isEmpty()) {
                    item {
                        Text("这个筛选下没有文件", color = TextFaint, modifier = Modifier.padding(vertical = 24.dp))
                    }
                }
                items(visible, key = { it.id }) { track ->
                    val dismissState = rememberSwipeToDismissBoxState(
                        confirmValueChange = { value ->
                            if (!running && value == SwipeToDismissBoxValue.EndToStart) {
                                vm.remove(track.id)
                            }
                            false
                        },
                    )
                    SwipeToDismissBox(
                        state = dismissState,
                        modifier = Modifier.animateItem().fillMaxWidth(),
                        enableDismissFromStartToEnd = false,
                        enableDismissFromEndToStart = !running,
                        backgroundContent = {
                            Box(
                                Modifier
                                    .fillMaxSize()
                                    .background(Danger.copy(alpha = 0.18f), androidx.compose.foundation.shape.RoundedCornerShape(18.dp))
                                    .padding(end = 18.dp),
                                contentAlignment = Alignment.CenterEnd,
                            ) {
                                Text("移出", color = Danger)
                            }
                        },
                    ) {
                        FileCard(
                            track = track,
                            fraction = if (track.id == session.runningId) session.runningFraction else null,
                            onClick = { sheet = Sheet.Detail(track.id) },
                        )
                    }
                }
            }
        }

        Column(
            Modifier
                .align(Alignment.BottomCenter)
                .fillMaxWidth()
                .background(Brush.verticalGradient(listOf(Color.Transparent, Bg)))
                .padding(horizontal = 20.dp)
                .padding(bottom = navBottom + 12.dp, top = 28.dp),
        ) {
            AnimatedVisibility(visible = toast != null, enter = fadeIn() + slideInVertically { it / 2 }, exit = fadeOut()) {
                Text(
                    toast.orEmpty(),
                    color = Text,
                    fontSize = 13.sp,
                    modifier = Modifier
                        .padding(bottom = 8.dp)
                        .background(Surface, androidx.compose.foundation.shape.RoundedCornerShape(12.dp))
                        .padding(horizontal = 12.dp, vertical = 8.dp),
                )
            }
            Text(
                "输出到 ${outputLabel(settings)}",
                color = TextFaint,
                fontSize = 12.sp,
                modifier = Modifier.padding(bottom = 8.dp),
            )
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                if (running) {
                    PrimaryButton("取消", true, vm::cancel, Modifier.weight(1f))
                } else {
                    GhostButton("选择文件", { pickFiles.launch(arrayOf("*/*")) }, Modifier.weight(1f))
                    PrimaryButton(
                        text = when {
                            !actionable && session.failed > 0 -> "重试失败"
                            allMissing -> "缺少密钥"
                            session.pending > 0 -> "解密 ${session.pending}"
                            else -> "开始解密"
                        },
                        enabled = when {
                            !actionable && session.failed > 0 -> true
                            allMissing -> false
                            else -> session.pending > 0
                        },
                        onClick = {
                            if (!actionable && session.failed > 0) {
                                vm.retry(context)
                            } else {
                                val need = Build.VERSION.SDK_INT >= 33 &&
                                    ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) !=
                                    android.content.pm.PackageManager.PERMISSION_GRANTED
                                if (need && !askedNotification) {
                                    askedNotification = true
                                    askNotification.launch(Manifest.permission.POST_NOTIFICATIONS)
                                } else {
                                    vm.start(context)
                                }
                            }
                        },
                        modifier = Modifier.weight(1.3f),
                    )
                }
            }
            Row(Modifier.padding(top = 8.dp), horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                Text("密钥", color = Lime, fontSize = 13.sp, modifier = Modifier.clickable { sheet = Sheet.Keys })
                Text("输出设置", color = TextDim, fontSize = 13.sp, modifier = Modifier.clickable { sheet = Sheet.Output })
                if (session.success > 0 && !running) {
                    Text("清除已完成", color = TextFaint, fontSize = 13.sp, modifier = Modifier.clickable { vm.clearFinished() })
                }
            }
        }
    }

    if (sheet != null) {
        ModalBottomSheet(
            onDismissRequest = { sheet = null },
            sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
            containerColor = app.musicdecryptor.theme.Surface,
            contentColor = Text,
        ) {
            when (val current = sheet) {
                Sheet.Output -> OutputSheet(settings, vm::setMode, { pickTree.launch(null) }, vm::setOverwrite, vm::setAutoExt)
                Sheet.Keys -> KeySheet(keys, settings.manualEkey, selfTest, { pickKey.launch(arrayOf("*/*")) }, vm::setManualEkey, vm::clearKeys, vm::runSelfTest)
                is Sheet.Detail -> {
                    val track = session.tracks.firstOrNull { it.id == current.id }
                    if (track == null) {
                        sheet = null
                    } else {
                        DetailSheet(
                            track = track,
                            onShare = { shareTrack(context, track) },
                            onOpen = { openTrack(context, track) },
                            onRetry = {
                                sheet = null
                                if (vm.retryOne(context, track.id)) {
                                    // 前台服务由 ViewModel 拉起
                                }
                            },
                        )
                    }
                }
                null -> Unit
            }
        }
    }
}

@Composable
private fun Header(session: app.musicdecryptor.engine.Session, running: Boolean) {
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(running) {
        while (running) {
            now = System.currentTimeMillis()
            delay(200)
        }
    }
    Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally) {
        Text("MFLAC   ·   MGG", style = EyebrowStyle)
        Text("音乐解密", style = TitleStyle)
        Text(
            "在手机上还原加密音频，不上传、不改源文件",
            color = TextFaint,
            fontSize = 13.sp,
            textAlign = TextAlign.Center,
        )
        Spacer(Modifier.height(8.dp))
        val percent = if (session.totalBytes > 0) "${(session.fraction * 100).toInt()}%" else if (session.tracks.isEmpty()) "—" else "0%"
        val caption = when {
            running -> formatElapsed(now - session.startedAt)
            session.phase == Phase.DONE -> "用时 ${formatElapsed(session.finishedAt - session.startedAt)}"
            session.tracks.isEmpty() -> "等待文件"
            else -> "${session.tracks.size} 个文件"
        }
        VinylHero(session.fraction, running, percent, caption)
    }
}

@Composable
private fun EmptyState() {
    Column(
        Modifier.fillMaxWidth().padding(vertical = 12.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        EmptyDisc()
        Spacer(Modifier.height(8.dp))
        Text("选择 .mflac / .mgg", color = Text, fontSize = 18.sp, fontWeight = FontWeight.Medium)
        Text(
            "可以从文件管理器挑选，也可以一次选很多。安卓 QQ 音乐缓存如果提示缺少密钥，先在「密钥」里导入 player_process_db。",
            color = TextDim,
            fontSize = 13.sp,
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(top = 6.dp, start = 12.dp, end = 12.dp),
        )
    }
}

@Composable
private fun FilterChip(text: String, selected: Boolean, onClick: () -> Unit) {
    Text(
        text,
        color = if (selected) app.musicdecryptor.theme.Ink else TextDim,
        fontSize = 13.sp,
        fontWeight = FontWeight.Medium,
        modifier = Modifier
            .background(if (selected) Lime else Surface, androidx.compose.foundation.shape.RoundedCornerShape(999.dp))
            .clickable(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 6.dp),
    )
}
