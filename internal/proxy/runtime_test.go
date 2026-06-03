package proxy

import "testing"

func TestRenderURLTemplateReplacesEveryUUIDWithOneGeneratedValue(t *testing.T) {
	fixedUUID := "11111111-2222-4333-8444-555555555555"
	calls := 0

	got := renderURLTemplate(
		" http://127.0.0.1:7890/{uuid}?session={uuid} ",
		func() string {
			calls++
			return fixedUUID
		},
	)

	want := "http://127.0.0.1:7890/" + fixedUUID + "?session=" + fixedUUID
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
