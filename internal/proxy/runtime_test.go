package proxy

import (
	"net/url"
	"testing"
)

func TestRenderURLTemplateReplacesEveryUUIDWithOneGeneratedValue(t *testing.T) {
	fixedUUID := "11111111-2222-4333-8444-555555555555"
	wantID := "11111111222243338444555555555555"
	calls := 0

	got := renderURLTemplate(
		" http://127.0.0.1:7890/{uuid}?session={uuid} ",
		func() string {
			calls++
			return fixedUUID
		},
	)

	want := "http://127.0.0.1:7890/" + wantID + "?session=" + wantID
	if got != want {
		t.Fatalf("渲染结果不符合预期: got %q, want %q", got, want)
	}
	if calls != 1 {
		t.Fatalf("同一个代理模板应只生成一次 UUID: got %d", calls)
	}
}

func TestRenderURLTemplateLeavesPlainProxyUnchanged(t *testing.T) {
	calls := 0

	got := renderURLTemplate(" http://127.0.0.1:7890 ", func() string {
		calls++
		return "unused"
	})

	if got != "http://127.0.0.1:7890" {
		t.Fatalf("普通代理不应被修改: got %q", got)
	}
	if calls != 0 {
		t.Fatalf("普通代理不应生成 UUID: got %d", calls)
	}
}

func TestRenderURLTemplateReturnsEmptyForBlankInput(t *testing.T) {
	got := renderURLTemplate("   ", func() string {
		return "unused"
	})

	if got != "" {
		t.Fatalf("空白代理应返回空字符串: got %q", got)
	}
}

func TestRenderURLTemplateSupportsUserInfoTemplateWithoutHyphen(t *testing.T) {
	got := renderURLTemplate(
		"https://user.{uuid}:pass@proxy.example.test:443",
		func() string {
			return "ABCDEFAB-CDEF-4ABC-8DEF-ABCDEFABCDEF"
		},
	)
	wantID := "abcdefabcdef4abc8defabcdefabcdef"
	want := "https://user." + wantID + ":pass@proxy.example.test:443"
	if got != want {
		t.Fatalf("userinfo 模板渲染失败: got %q, want %q", got, want)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("渲染后的代理 URL 应可解析: %v", err)
	}
	if parsed.User.Username() != "user."+wantID || parsed.Host != "proxy.example.test:443" {
		t.Fatalf("渲染后的代理 URL 结构异常: %q", got)
	}
}
