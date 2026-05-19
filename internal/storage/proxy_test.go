package storage

import "testing"

func TestNormalizeProxyAddressKeepsFullTemplateURL(t *testing.T) {
	input := "https://node.{uuid}:admin2012@resin-proxy.codeai.de5.net:443"

	if got := NormalizeProxyAddress(input); got != input {
		t.Fatalf("完整 URL 模板不应被改写: got %q, want %q", got, input)
	}
}

func TestNormalizeProxyAddressSupportsHostPortUserPassTemplate(t *testing.T) {
	got := NormalizeProxyAddress("resin-proxy.codeai.de5.net:443:node.{uuid}:admin2012")
	want := "http://node.{uuid}:admin2012@resin-proxy.codeai.de5.net:443"

	if got != want {
		t.Fatalf("host:port:user:pass 模板归一化失败: got %q, want %q", got, want)
	}
}

func TestNormalizeProxyAddressKeepsHostPortDefaultBehavior(t *testing.T) {
	got := NormalizeProxyAddress("127.0.0.1:7890")
	want := "socks5://127.0.0.1:7890"

	if got != want {
		t.Fatalf("host:port 默认归一化行为不应回退: got %q, want %q", got, want)
	}
}
