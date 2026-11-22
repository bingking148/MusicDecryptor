package updater

import "testing"

func TestParseAndCompareVersion(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "相同正式版", a: "1.1.0", b: "v1.1.0", want: 0},
		{name: "四段零等于三段", a: "1.1.0", b: "1.1.0.0", want: 0},
		{name: "补丁更新", a: "1.1.0", b: "1.1.1", want: -1},
		{name: "次版本按数字比较", a: "v1.9.0", b: "v1.10.0", want: -1},
		{name: "旧版本", a: "1.2.0", b: "1.1.9", want: 1},
		{name: "正式版高于预发布", a: "1.2.0", b: "1.2.0-rc.1", want: 1},
		{name: "预发布仍高于更旧正式版", a: "1.1.0", b: "1.2.0-rc.1", want: -1},
		{name: "大写前缀", a: "V1.2.0", b: "v1.2.0", want: 0},
		{name: "构建元数据不影响比较", a: "1.2.0+build.3", b: "1.2.0", want: 0},
		{name: "文件修订号", a: "1.1.0.0", b: "1.1.0.1", want: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			av, err := ParseVersion(tc.a)
			if err != nil {
				t.Fatalf("解析 %q：%v", tc.a, err)
			}
			bv, err := ParseVersion(tc.b)
			if err != nil {
				t.Fatalf("解析 %q：%v", tc.b, err)
			}
			if got := av.Compare(bv); got != tc.want {
				t.Fatalf("Compare(%q, %q) = %d，期望 %d", tc.a, tc.b, got, tc.want)
			}
			newer, err := Newer(tc.a, tc.b)
			if err != nil {
				t.Fatal(err)
			}
			if newer != (tc.want < 0) {
				t.Fatalf("Newer(%q, %q) = %v", tc.a, tc.b, newer)
			}
		})
	}
}

func TestParseVersionRejectsInvalid(t *testing.T) {
	for _, raw := range []string{"", "   ", "v", "abc", "1.x.0", "1.2.0-", "1.2.3.4.5", "版本1.2"} {
		if _, err := ParseVersion(raw); err == nil {
			t.Fatalf("应拒绝 %q", raw)
		}
	}
}

func TestCurrentVersionParses(t *testing.T) {
	if _, err := ParseVersion(CurrentVersion); err != nil {
		t.Fatal(err)
	}
}
