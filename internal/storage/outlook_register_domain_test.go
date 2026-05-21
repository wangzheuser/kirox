package storage

import "testing"

func TestOutlookRegisterDomainOverrideDefaultsToEmpty(t *testing.T) {
	withTempStorageConfig(t, "")

	if got := GetOutlookRegisterDomainOverride(); got != "" {
		t.Fatalf("默认不应启用 Outlook 注册邮箱后缀覆盖: got %q", got)
	}
}

func TestSetOutlookRegisterDomainOverrideNormalizesDomain(t *testing.T) {
	withTempStorageConfig(t, "")

	domain, err := SetOutlookRegisterDomainOverride(" @Outlook.FR ")
	if err != nil {
		t.Fatal(err)
	}
	if domain != "outlook.fr" {
		t.Fatalf("后缀规范化失败: got %q", domain)
	}
	if got := GetOutlookRegisterDomainOverride(); got != "outlook.fr" {
		t.Fatalf("后缀读取失败: got %q", got)
	}
}

func TestSetOutlookRegisterDomainOverrideAllowsOutlookWildcardDomains(t *testing.T) {
	withTempStorageConfig(t, "")

	for _, input := range []string{"outlook.fr", "outlook.cl", "outlook.com"} {
		domain, err := SetOutlookRegisterDomainOverride(input)
		if err != nil {
			t.Fatalf("outlook.* 后缀应允许保存: input=%q err=%v", input, err)
		}
		if domain != input {
			t.Fatalf("outlook.* 后缀保存结果异常: input=%q got=%q", input, domain)
		}
	}
}

func TestSetOutlookRegisterDomainOverrideRejectsInvalidDomain(t *testing.T) {
	withTempStorageConfig(t, "")

	for _, input := range []string{"http://outlook.fr", "outlook.fr/path", "outlook fr", "outlook", "-bad.fr"} {
		if _, err := SetOutlookRegisterDomainOverride(input); err == nil {
			t.Fatalf("非法后缀应返回错误: %q", input)
		}
	}
}

func TestOutlookRegisterDomainOverrideStoredInvalidValueFallsBackToEmpty(t *testing.T) {
	withTempStorageConfig(t, "outlook_register_domain_override=http://outlook.fr\n")

	if got := GetOutlookRegisterDomainOverride(); got != "" {
		t.Fatalf("非法历史后缀应回退为空: got %q", got)
	}
}
