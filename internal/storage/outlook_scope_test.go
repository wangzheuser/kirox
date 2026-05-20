package storage

import "testing"

func TestOutlookScopeDefaultsToIMAP(t *testing.T) {
	withTempStorageConfig(t, "")

	if got := GetOutlookScope(); got != OutlookScopeIMAP {
		t.Fatalf("Outlook 读取方式默认应为 imap: got %q", got)
	}
}

func TestSetOutlookScopeGraph(t *testing.T) {
	withTempStorageConfig(t, "")

	if err := SetOutlookScope(OutlookScopeGraph); err != nil {
		t.Fatal(err)
	}
	if got := GetOutlookScope(); got != OutlookScopeGraph {
		t.Fatalf("Outlook Graph 配置读取失败: got %q", got)
	}
}

func TestSetOutlookScopeRejectsInvalidValue(t *testing.T) {
	withTempStorageConfig(t, "")

	if err := SetOutlookScope("pop3"); err == nil {
		t.Fatalf("非法 Outlook 读取方式应返回错误")
	}
}

func TestOutlookScopeStoredInvalidValueFallsBackToIMAP(t *testing.T) {
	withTempStorageConfig(t, "outlook_scope=pop3\n")

	if got := GetOutlookScope(); got != OutlookScopeIMAP {
		t.Fatalf("非法历史配置应回退到 imap: got %q", got)
	}
}
