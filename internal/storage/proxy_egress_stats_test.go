package storage

import (
	"testing"
	"time"
)

func findProxyEgressStat(t *testing.T, stats []ProxyEgressStat, sourceKey, ip string) ProxyEgressStat {
	t.Helper()
	for _, stat := range stats {
		if stat.SourceKey == sourceKey && stat.IP == ip {
			return stat
		}
	}
	t.Fatalf("proxy egress stat %s/%s not found in stats: %#v", sourceKey, ip, stats)
	return ProxyEgressStat{}
}

func TestProxyEgressStatsRecordAttemptSuccessAndRiskCooldown(t *testing.T) {
	withTempStorageConfig(t, "")

	egress := ProxyEgressIdentity{
		IP:          " 203.0.113.10 ",
		CountryCode: "ro",
		ISP:         "Example ISP",
		ASN:         "AS64500 Example",
	}
	if err := RecordProxyEgressAttempt(" template-a ", egress); err != nil {
		t.Fatalf("RecordProxyEgressAttempt returned error: %v", err)
	}
	if err := RecordProxyEgressRegistrationSuccess("template-a", egress); err != nil {
		t.Fatalf("RecordProxyEgressRegistrationSuccess returned error: %v", err)
	}
	if err := RecordProxyEgressRiskFailure("template-a", egress, 10*time.Minute); err != nil {
		t.Fatalf("RecordProxyEgressRiskFailure returned error: %v", err)
	}

	stat := findProxyEgressStat(t, GetProxyEgressStats(), "template-a", "203.0.113.10")
	if stat.CountryCode != "RO" || stat.ISP != "Example ISP" || stat.ASN != "AS64500 Example" {
		t.Fatalf("identity fields mismatch: %#v", stat)
	}
	if stat.AttemptCount != 1 || stat.SuccessCount != 1 || stat.RiskFailureCount != 1 || stat.NetworkFailureCount != 0 {
		t.Fatalf("count fields mismatch: %#v", stat)
	}
	if !IsProxyEgressCooling("template-a", "203.0.113.10", time.Now()) {
		t.Fatalf("risk failure should put exact IP into cooldown")
	}
	if IsProxyEgressCooling("template-a", "203.0.113.11", time.Now()) {
		t.Fatalf("cooldown should not apply to another IP in the same source")
	}
}

func TestProxyEgressStatsSuccessClearsCooldownAndResetClearsAll(t *testing.T) {
	withTempStorageConfig(t, "")

	egress := ProxyEgressIdentity{IP: "203.0.113.10", CountryCode: "RO"}
	if err := RecordProxyEgressRiskFailure("template-a", egress, time.Hour); err != nil {
		t.Fatalf("RecordProxyEgressRiskFailure returned error: %v", err)
	}
	if !IsProxyEgressCooling("template-a", "203.0.113.10", time.Now()) {
		t.Fatalf("risk failure should cool the egress IP")
	}
	if err := RecordProxyEgressRegistrationSuccess("template-a", egress); err != nil {
		t.Fatalf("RecordProxyEgressRegistrationSuccess returned error: %v", err)
	}
	if IsProxyEgressCooling("template-a", "203.0.113.10", time.Now()) {
		t.Fatalf("success should clear cooldown")
	}
	if !HasProxyEgressSuccess("template-a", "203.0.113.10") {
		t.Fatalf("success should mark egress IP as historically successful")
	}

	if err := ResetProxyEgressStats(); err != nil {
		t.Fatalf("ResetProxyEgressStats returned error: %v", err)
	}
	if got := GetProxyEgressStats(); len(got) != 0 {
		t.Fatalf("stats should be empty after reset, got %#v", got)
	}
}
