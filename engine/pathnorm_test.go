package engine

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 截图中的输出目录，以及会触发同一类误判的写法。
const screenshotDir = `E:\qq音乐下载\输出`

func TestNormalizeLocalDirScreenshotCases(t *testing.T) {
	if []rune("：")[0] != '\uFF1A' || []rune("＼")[0] != '\uFF3C' {
		t.Fatal("测试文件里的全角冒号或反斜杠不是预期码点")
	}
	accepted := []struct {
		name string
		in   string
		want string
	}{
		{name: "半角冒号", in: screenshotDir, want: screenshotDir},
		{name: "全角冒号", in: "E：\\qq音乐下载\\输出", want: screenshotDir},
		{name: "ASCII双引号", in: `"E:\qq音乐下载\输出"`, want: screenshotDir},
		{name: "ASCII单引号", in: `'E:\qq音乐下载\输出'`, want: screenshotDir},
		{name: "引号加全角冒号", in: `"E：\qq音乐下载\输出"`, want: screenshotDir},
		{name: "首尾空格", in: "  " + screenshotDir + "  ", want: screenshotDir},
		{name: "首尾制表符和换行", in: "\t" + screenshotDir + "\r\n", want: screenshotDir},
		{name: "全角空格", in: "\u3000" + screenshotDir + "\u3000", want: screenshotDir},
		{name: "不换行空格", in: "\u00a0" + screenshotDir + "\u00a0", want: screenshotDir},
		{name: "引号内空白和全角冒号", in: "  \"  E：\\qq音乐下载\\输出  \"  ", want: screenshotDir},
		{name: "中文双引号", in: "“E:\\qq音乐下载\\输出”", want: screenshotDir},
		{name: "中文单引号", in: "‘E:\\qq音乐下载\\输出’", want: screenshotDir},
		{name: "直角引号加全角冒号", in: "「E：\\qq音乐下载\\输出」", want: screenshotDir},
		{name: "双直角引号", in: "『E:\\qq音乐下载\\输出』", want: screenshotDir},
		{name: "全角引号", in: "＂E:\\qq音乐下载\\输出＂", want: screenshotDir},
		{name: "全角单引号", in: "＇E:\\qq音乐下载\\输出＇", want: screenshotDir},
		{name: "书名号式引号", in: "«E:\\qq音乐下载\\输出»", want: screenshotDir},
		{name: "德文引号", in: "„E:\\qq音乐下载\\输出”", want: screenshotDir},
		{name: "嵌套引号", in: `"“E:\qq音乐下载\输出”"`, want: screenshotDir},
		{name: "全角反斜杠", in: `E:\qq音乐下载＼输出`, want: screenshotDir},
		{name: "全角盘符全角冒号和全角反斜杠", in: `Ｅ：＼qq音乐下载＼输出`, want: screenshotDir},
		{name: "小写全角盘符", in: `ｅ：\qq音乐下载\输出`, want: `e:\qq音乐下载\输出`},
		{name: "正斜杠分隔", in: `E:/qq音乐下载/输出`, want: `E:/qq音乐下载/输出`},
		{name: "带引号的UNC", in: `"\\server\share\qq音乐下载"`, want: `\\server\share\qq音乐下载`},
		{name: "全角反斜杠UNC", in: `＼＼server＼share＼music`, want: `\\server\share\music`},
		{name: "设备路径", in: `\\?\E:\qq音乐下载\输出`, want: `\\?\E:\qq音乐下载\输出`},
	}

	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeLocalDir(tc.in)
			if err != nil {
				t.Fatalf("应视为本地路径，输入 %q，错误：%v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("规范化结果 = %q，期望 %q", got, tc.want)
			}
			again, err := normalizeLocalDir(got)
			if err != nil || again != got {
				t.Fatalf("重复规范化不稳定：%q -> %q (%v)", got, again, err)
			}
		})
	}
}

func TestNormalizeLocalDirRejectsNonLocal(t *testing.T) {
	rejected := []struct {
		name string
		in   string
		want string
	}{
		{name: "空字符串", in: "", want: ""},
		{name: "空白", in: " \t\u3000 ", want: ""},
		{name: "空引号", in: `""`, want: ""},
		{name: "空中文引号", in: "“”", want: ""},
		{name: "相对路径", in: `qq音乐下载\输出`, want: `qq音乐下载\输出`},
		{name: "引号包裹的相对路径", in: `  "qq音乐下载\输出"  `, want: `qq音乐下载\输出`},
		{name: "点相对路径", in: `./输出`, want: `./输出`},
		{name: "上级目录", in: `../输出`, want: `../输出`},
		{name: "Unix绝对路径", in: `/home/user/输出`, want: `/home/user/输出`},
		{name: "URL", in: `https://example.com/输出`, want: `https://example.com/输出`},
		{name: "目录名中的全角冒号", in: `歌曲：精选`, want: `歌曲：精选`},
		{name: "缺少盘符的全角冒号", in: `：\qq音乐下载\输出`, want: `：\qq音乐下载\输出`},
		{name: "盘符后没有冒号", in: `E\qq音乐下载\输出`, want: `E\qq音乐下载\输出`},
		{name: "file URL", in: `file:///E:/输出`, want: `file:///E:/输出`},
		{name: "只有半边引号", in: `"E:\qq音乐下载\输出`, want: `"E:\qq音乐下载\输出`},
	}

	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeUserPath(tc.in); got != tc.want {
				t.Fatalf("规范化结果 = %q，期望 %q", got, tc.want)
			}
			_, err := normalizeLocalDir(tc.in)
			if err == nil {
				t.Fatalf("应拒绝非本地路径 %q", tc.in)
			}
			if !strings.Contains(err.Error(), "不是本地路径") {
				t.Fatalf("错误应保持原校验文案，实际：%v", err)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("错误应带上规范化后的路径 %q，实际：%v", tc.want, err)
			}
		})
	}
}

func TestNormalizeUserPathKeepsLegalFullwidthInNames(t *testing.T) {
	// 全角冒号、全角斜杠在 Windows 文件名里是合法字符，不能当成路径语法改掉。
	cases := []string{
		`E:\音乐\歌曲：精选`,
		`E:\音乐\2024／演唱会`,
		`D:\qq音乐下载\输出\歌曲：精选／现场`,
	}
	for _, in := range cases {
		got, err := normalizeLocalDir(in)
		if err != nil {
			t.Fatalf("应接受 %q：%v", in, err)
		}
		if got != in {
			t.Fatalf("不应改写目录名中的全角字符：%q -> %q", in, got)
		}
	}
}

func TestNormalizeOptionsUsesCanonicalPath(t *testing.T) {
	opts := Options{
		InputDir:  " „E：\\qq音乐\\输入“ ",
		OutputDir: "  \"E：\\qq音乐下载\\输出\"  ",
	}
	NormalizeOptions(&opts)
	if opts.InputDir != `E:\qq音乐\输入` {
		t.Fatalf("输入目录 = %q", opts.InputDir)
	}
	if opts.OutputDir != screenshotDir {
		t.Fatalf("输出目录 = %q", opts.OutputDir)
	}
	got, err := normalizeLocalDir(opts.OutputDir)
	if err != nil || got != opts.OutputDir {
		t.Fatalf("后续本地路径校验应直接使用规范化结果，得到 %q, %v", got, err)
	}

	settings := NormalizeSettings(&Settings{OutputDir: `"E：\qq音乐下载\输出"`, Theme: "dark"})
	if settings.OutputDir != screenshotDir {
		t.Fatalf("保存的输出目录 = %q", settings.OutputDir)
	}
}

func TestPrepareStagingRejectsNonLocalBeforeCreate(t *testing.T) {
	_, err := prepareStaging("  \"qq音乐下载\\输出\"  ")
	if err == nil {
		t.Fatal("带引号的相对路径应被拒绝")
	}
	if !strings.Contains(err.Error(), "不是本地路径") || !strings.Contains(err.Error(), `qq音乐下载\输出`) {
		t.Fatalf("应在创建目录前按规范化结果拒绝，实际：%v", err)
	}
	if strings.Contains(err.Error(), `"`) {
		t.Fatalf("错误里不应再保留外层引号：%v", err)
	}
}

func TestWindowsVolumeNameMatchesStdlib(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("filepath.VolumeName 只在 Windows 上识别盘符；本机已用同等规则测试 windowsVolumeName")
	}
	samples := []string{
		`E:\qq音乐下载\输出`,
		`e:/qq音乐下载/输出`,
		`C:foo`,
		`\\server\share\qq音乐下载`,
		`//server/share/qq音乐下载`,
		`\\?\E:\qq音乐下载\输出`,
		`\\.\UNC\server\share\dir`,
		`qq音乐下载\输出`,
		`\qq音乐下载`,
		``,
	}
	for _, sample := range samples {
		if got, want := windowsVolumeName(sample), filepath.VolumeName(sample); got != want {
			t.Errorf("windowsVolumeName(%q) = %q，标准库 = %q", sample, got, want)
		}
	}
}

func TestWindowsVolumeName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: `E:\qq音乐下载\输出`, want: `E:`},
		{in: `e:/qq音乐下载/输出`, want: `e:`},
		{in: `C:foo`, want: `C:`},
		{in: `\\server\share\qq音乐下载`, want: `\\server\share`},
		{in: `//server/share/qq音乐下载`, want: `\\server\share`},
		{in: `\\server/share\qq音乐下载`, want: `\\server\share`},
		{in: `//?/E:/qq音乐下载/输出`, want: `\\?\E:`},
		{in: `\\?\E:\qq音乐下载\输出`, want: `\\?\E:`},
		{in: `\\.\UNC\server\share\dir`, want: `\\.\UNC\server\share`},
		{in: `qq音乐下载\输出`, want: ""},
		{in: `/home/user/输出`, want: ""},
		{in: `https://example.com/输出`, want: ""},
		{in: ``, want: ""},
		{in: `\qq音乐下载`, want: ""},
	}
	for _, tc := range cases {
		if got := windowsVolumeName(tc.in); got != tc.want {
			t.Errorf("windowsVolumeName(%q) = %q，期望 %q", tc.in, got, tc.want)
		}
	}
}
