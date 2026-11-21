package app.musicdecryptor.ui

import android.graphics.BitmapFactory
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.animateIntAsState
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.ripple
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.musicdecryptor.engine.Track
import app.musicdecryptor.engine.TrackStatus
import app.musicdecryptor.theme.Bg
import app.musicdecryptor.theme.Danger
import app.musicdecryptor.theme.EyebrowStyle
import app.musicdecryptor.theme.Gold
import app.musicdecryptor.theme.Ink
import app.musicdecryptor.theme.Lime
import app.musicdecryptor.theme.Line
import app.musicdecryptor.theme.LineStrong
import app.musicdecryptor.theme.Skipped
import app.musicdecryptor.theme.Success
import app.musicdecryptor.theme.Surface
import app.musicdecryptor.theme.Surface2
import app.musicdecryptor.theme.Text
import app.musicdecryptor.theme.TextDim
import app.musicdecryptor.theme.TextFaint
import java.util.Locale

@Composable
fun VinylHero(
    fraction: Float,
    running: Boolean,
    center: String,
    caption: String,
    modifier: Modifier = Modifier,
) {
    val spin = remember { Animatable(0f) }
    LaunchedEffect(running) {
        if (!running) return@LaunchedEffect
        while (true) {
            spin.animateTo(spin.value + 360f, tween(durationMillis = 8400, easing = LinearEasing))
        }
    }
    val sweep by animateFloatAsState(fraction.coerceIn(0f, 1f), tween(220), label = "sweep")
    Box(modifier.size(220.dp), contentAlignment = Alignment.Center) {
        Canvas(Modifier.fillMaxSize()) {
            val stroke = 3.dp.toPx()
            val inset = stroke
            drawArc(
                color = Line,
                startAngle = -90f,
                sweepAngle = 360f,
                useCenter = false,
                topLeft = Offset(inset, inset),
                size = Size(size.width - inset * 2, size.height - inset * 2),
                style = Stroke(stroke, cap = StrokeCap.Round),
            )
            if (sweep > 0f) {
                drawArc(
                    color = Lime,
                    startAngle = -90f,
                    sweepAngle = 360f * sweep,
                    useCenter = false,
                    topLeft = Offset(inset, inset),
                    size = Size(size.width - inset * 2, size.height - inset * 2),
                    style = Stroke(stroke, cap = StrokeCap.Round),
                )
            }
        }
        Canvas(
            Modifier
                .size(176.dp)
                .graphicsLayer { rotationZ = spin.value },
        ) {
            drawCircle(Color(0xFF10160E))
            val grooves = 6
            for (i in 1..grooves) {
                drawCircle(
                    color = Color(0xFF2A3424),
                    radius = size.minDimension / 2f * (0.22f + 0.1f * i),
                    style = Stroke(width = 1.dp.toPx()),
                )
            }
            drawCircle(Lime.copy(alpha = 0.9f), radius = size.minDimension * 0.11f)
            drawCircle(Bg, radius = size.minDimension * 0.045f)
        }
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text(center, color = Text, fontSize = 22.sp, fontWeight = FontWeight.Medium)
            Text(caption, color = TextDim, fontSize = 12.sp)
        }
    }
}

@Composable
fun StatPill(value: Int, label: String, color: Color) {
    val shown by animateIntAsState(value, tween(280), label = label)
    Column(horizontalAlignment = Alignment.CenterHorizontally) {
        Text("$shown", color = color, fontSize = 20.sp, fontWeight = FontWeight.SemiBold)
        Text(label, color = TextFaint, fontSize = 12.sp)
    }
}

@Composable
fun FileCard(
    track: Track,
    fraction: Float?,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val rail by animateColorAsState(statusColor(track.status), tween(220), label = "rail")
    Row(
        modifier
            .clip(RoundedCornerShape(18.dp))
            .background(Surface)
            .border(1.dp, Line, RoundedCornerShape(18.dp))
            .clickable(onClick = onClick)
            .padding(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            Modifier
                .width(3.dp)
                .height(46.dp)
                .clip(CircleShape)
                .background(rail),
        )
        Spacer(Modifier.width(12.dp))
        Cover(track)
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(
                track.title ?: track.name,
                color = Text,
                fontSize = 15.sp,
                fontWeight = FontWeight.Medium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            val sub = buildString {
                if (!track.artist.isNullOrBlank()) {
                    append(track.artist)
                    append("  ·  ")
                }
                append(formatBytes(track.size))
                append("  ·  ")
                append(track.planLabel)
            }
            Text(sub, color = TextFaint, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (fraction != null) {
                Spacer(Modifier.height(8.dp))
                Box(
                    Modifier
                        .fillMaxWidth()
                        .height(3.dp)
                        .clip(CircleShape)
                        .background(Line),
                ) {
                    Box(
                        Modifier
                            .fillMaxHeight()
                            .fillMaxWidth(fraction.coerceIn(0.04f, 1f))
                            .background(Lime),
                    )
                }
            } else if (track.message.isNotBlank() && track.status != TrackStatus.SUCCESS) {
                Text(track.message, color = rail, fontSize = 12.sp, maxLines = 2, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

@Composable
private fun Cover(track: Track) {
    val bitmap = remember(track.cover) {
        track.cover?.let { bytes ->
            BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap()
        }
    }
    Box(
        Modifier
            .size(48.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(Surface2),
        contentAlignment = Alignment.Center,
    ) {
        if (bitmap != null) {
            androidx.compose.foundation.Image(
                bitmap = bitmap,
                contentDescription = track.title ?: track.name,
                modifier = Modifier.fillMaxSize(),
                contentScale = ContentScale.Crop,
            )
        } else {
            Canvas(Modifier.size(22.dp)) {
                drawCircle(color = Lime.copy(alpha = 0.85f), style = Stroke(1.6.dp.toPx()))
                drawCircle(color = Lime, radius = size.minDimension * 0.16f)
            }
        }
    }
}

@Composable
fun PrimaryButton(text: String, enabled: Boolean, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val source = remember { MutableInteractionSource() }
    val pressed by source.collectIsPressedAsState()
    val scale by animateFloatAsState(if (pressed && enabled) 0.975f else 1f, label = "press")
    Box(
        modifier
            .graphicsLayer { scaleX = scale; scaleY = scale }
            .height(52.dp)
            .clip(RoundedCornerShape(16.dp))
            .background(if (enabled) Lime else Surface2)
            .clickable(source, ripple(color = Ink), enabled = enabled, onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Text(text, color = if (enabled) Ink else TextFaint, fontWeight = FontWeight.SemiBold, fontSize = 16.sp)
    }
}

@Composable
fun GhostButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Box(
        modifier
            .height(52.dp)
            .clip(RoundedCornerShape(16.dp))
            .border(1.dp, LineStrong, RoundedCornerShape(16.dp))
            .clickable(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Text(text, color = Text, fontWeight = FontWeight.Medium, fontSize = 15.sp)
    }
}

@Composable
fun IconButton(onClick: () -> Unit, contentDescription: String, icon: androidx.compose.ui.graphics.vector.ImageVector) {
    Box(
        Modifier
            .size(40.dp)
            .clip(CircleShape)
            .background(Surface)
            .clickable(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, contentDescription, tint = Text)
    }
}

@Composable
fun StatusIcon(status: TrackStatus) {
    val (icon, tint) = when (status) {
        TrackStatus.SUCCESS -> Icons.Filled.Check to Success
        TrackStatus.FAILED -> Icons.Filled.Close to Danger
        else -> return
    }
    Icon(icon, null, tint = tint, modifier = Modifier.size(18.dp))
}

@Composable
fun CenterLabel(target: String) {
    AnimatedContent(
        targetState = target,
        transitionSpec = { fadeIn(tween(180)) togetherWith fadeOut(tween(120)) },
        label = "center",
    ) { value ->
        Text(value, color = Text, fontSize = 22.sp, fontWeight = FontWeight.Medium)
    }
}

fun statusColor(status: TrackStatus): Color = when (status) {
    TrackStatus.SUCCESS -> Success
    TrackStatus.FAILED -> Danger
    TrackStatus.SKIPPED -> Skipped
    TrackStatus.RUNNING -> Lime
    TrackStatus.PENDING -> Gold
}

fun formatBytes(n: Long): String {
    if (n < 1024) return "$n B"
    val kb = n / 1024.0
    if (kb < 1024) return String.format(Locale.US, "%.1f KB", kb)
    val mb = kb / 1024.0
    if (mb < 1024) return String.format(Locale.US, "%.1f MB", mb)
    return String.format(Locale.US, "%.2f GB", mb / 1024.0)
}

fun formatElapsed(ms: Long): String {
    val seconds = (ms / 1000).coerceAtLeast(0)
    return if (seconds < 60) "$seconds 秒" else "${seconds / 60} 分 ${seconds % 60} 秒"
}

@Composable
fun EmptyDisc(modifier: Modifier = Modifier) {
    Box(modifier, contentAlignment = Alignment.Center) {
        Canvas(Modifier.size(168.dp)) {
            drawCircle(
                brush = Brush.radialGradient(listOf(Lime.copy(alpha = 0.16f), Color.Transparent)),
            )
            drawCircle(Surface, radius = size.minDimension * 0.42f)
            drawCircle(Lime.copy(alpha = 0.8f), radius = size.minDimension * 0.42f, style = Stroke(1.4.dp.toPx()))
            drawCircle(Line, radius = size.minDimension * 0.28f, style = Stroke(1.dp.toPx()))
            drawCircle(Lime, radius = size.minDimension * 0.07f)
        }
        Text("放入", style = EyebrowStyle)
    }
}
