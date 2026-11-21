package app.musicdecryptor.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

val Bg = Color(0xFF090B08)
val Surface = Color(0xFF141910)
val Surface2 = Color(0xFF1C2416)
val Line = Color(0xFF2C3526)
val LineStrong = Color(0xFF425036)
val Text = Color(0xFFF3F6EC)
val TextDim = Color(0xFFB7C0AB)
val TextFaint = Color(0xFF7E8874)
val Lime = Color(0xFFD6F25C)
val Ink = Color(0xFF17200A)
val Gold = Color(0xFFE3C27A)
val Success = Color(0xFF8FD6B0)
val Danger = Color(0xFFF0A097)
val Skipped = Color(0xFF9AA392)

private val scheme = darkColorScheme(
    primary = Lime,
    onPrimary = Ink,
    secondary = Gold,
    onSecondary = Ink,
    background = Bg,
    onBackground = Text,
    surface = Surface,
    onSurface = Text,
    surfaceContainer = Surface2,
    surfaceContainerHigh = Surface2,
    onSurfaceVariant = TextDim,
    outline = Line,
    error = Danger,
    onError = Ink,
)

val TitleStyle = TextStyle(
    fontSize = 32.sp,
    lineHeight = 36.sp,
    fontWeight = FontWeight.Medium,
    color = Text,
)

val EyebrowStyle = TextStyle(
    fontSize = 11.sp,
    lineHeight = 14.sp,
    fontWeight = FontWeight.SemiBold,
    letterSpacing = 1.8.sp,
    color = Lime,
)

@Composable
fun MusicTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = scheme, content = content)
}
