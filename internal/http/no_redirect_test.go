package http

import (
	"testing"

	tls_client "github.com/bogdanfinn/tls-client"
)

func TestNewNoRedirectTLSClientPassesChromeVersion(t *testing.T) {
	var gotFollow bool
	var gotTimeout int
	var gotChrome []string
	oldFactory := newTLSClientWithTimeout
	defer func() { newTLSClientWithTimeout = oldFactory }()
	newTLSClientWithTimeout = func(proxy string, followRedirect bool, timeoutSeconds int, chromeVer ...string) (tls_client.HttpClient, error) {
		gotFollow = followRedirect
		gotTimeout = timeoutSeconds
		gotChrome = append([]string(nil), chromeVer...)
		return nil, nil
	}

	_ = NewNoRedirectTLSClient("http://127.0.0.1:7890", "124.0.0.0")

	if gotFollow {
		t.Fatalf("NewNoRedirectTLSClient should disable redirects")
	}
	if gotTimeout != 60 {
		t.Fatalf("timeout=%d, want 60", gotTimeout)
	}
	if len(gotChrome) != 1 || gotChrome[0] != "124.0.0.0" {
		t.Fatalf("chrome version not forwarded, got %v", gotChrome)
	}
}
