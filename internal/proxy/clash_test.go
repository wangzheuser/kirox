package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClashClientSwitchesToSecondNodeAfterFirstDelayFails(t *testing.T) {
	var switched []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.2.3"})
		case r.URL.Path == "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"proxies": map[string]interface{}{
					"Proxy": map[string]interface{}{"type": "Selector", "all": []string{"DIRECT", "bad", "good"}, "now": "DIRECT"},
				},
			})
		case r.URL.Path == "/proxies/Proxy" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "Selector", "all": []string{"DIRECT", "bad", "good"}, "now": "DIRECT"})
		case r.URL.Path == "/proxies/Proxy" && r.Method == http.MethodPut:
			var body struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			switched = append(switched, body.Name)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/proxies/bad/delay":
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "bad node"})
		case r.URL.Path == "/proxies/good/delay":
			_ = json.NewEncoder(w).Encode(map[string]int{"delay": 123})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClashClient(ClashConfig{Enabled: true, APIURL: server.URL, TestTimeout: 1})
	selection, err := client.SwitchToNextAvailable(context.Background())
	if err != nil {
		t.Fatalf("SwitchToNextAvailable() error = %v", err)
	}
	if selection.Node != "good" {
		t.Fatalf("expected good node, got %q", selection.Node)
	}
	if selection.DelayMs != 123 {
		t.Fatalf("expected delay 123, got %d", selection.DelayMs)
	}
	if strings.Join(switched, ",") != "bad,good" {
		t.Fatalf("unexpected switch order: %v", switched)
	}
}

func TestClashClientFiltersSpecialNodesAndDetectsPriorityGroup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.2.3"})
		case r.URL.Path == "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"proxies": map[string]interface{}{
					"Proxy": map[string]interface{}{"type": "Selector", "all": []string{"DIRECT", "node-a", "REJECT", "PASS", "COMPATIBLE"}, "now": "node-a"},
				},
			})
		case r.URL.Path == "/proxies/Proxy":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "Selector", "all": []string{"DIRECT", "node-a", "REJECT", "PASS", "COMPATIBLE"}, "now": "node-a"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClashClient(ClashConfig{Enabled: true, APIURL: server.URL})
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if client.proxyGroup != "Proxy" {
		t.Fatalf("expected Proxy group, got %q", client.proxyGroup)
	}
	if len(client.nodes) != 1 || client.nodes[0] != "node-a" {
		t.Fatalf("unexpected nodes: %v", client.nodes)
	}
}

func TestClashClientSkipConnectivityTestDoesNotCallDelay(t *testing.T) {
	delayCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.2.3"})
		case r.URL.Path == "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"proxies": map[string]interface{}{
					"Proxy": map[string]interface{}{"type": "Selector", "all": []string{"node-a"}, "now": "node-a"},
				},
			})
		case r.URL.Path == "/proxies/Proxy" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "Selector", "all": []string{"node-a"}, "now": "node-a"})
		case r.URL.Path == "/proxies/Proxy" && r.Method == http.MethodPut:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/delay"):
			delayCalled = true
			_ = json.NewEncoder(w).Encode(map[string]int{"delay": 10})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClashClient(ClashConfig{Enabled: true, APIURL: server.URL, SkipConnectivityTest: true})
	selection, err := client.SwitchToNextAvailable(context.Background())
	if err != nil {
		t.Fatalf("SwitchToNextAvailable() error = %v", err)
	}
	if !selection.SkippedTest {
		t.Fatalf("expected skipped test selection")
	}
	if delayCalled {
		t.Fatalf("delay endpoint should not be called")
	}
}

func TestClashClientErrorDoesNotLeakAPISecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("token-secret"))
	}))
	defer server.Close()

	client := NewClashClient(ClashConfig{Enabled: true, APIURL: server.URL, APISecret: "token-secret"})
	err := client.Initialize(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	if strings.Contains(err.Error(), "token-secret") {
		t.Fatalf("secret leaked in error: %v", err)
	}
}

func TestClashClientToleratesEmptyVersionBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"proxies": map[string]interface{}{
					"Proxy": map[string]interface{}{"type": "Selector", "all": []string{"node-a"}, "now": "node-a"},
				},
			})
		case r.URL.Path == "/proxies/Proxy":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "Selector", "all": []string{"node-a"}, "now": "node-a"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClashClient(ClashConfig{Enabled: true, APIURL: server.URL})
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() should tolerate empty /version body: %v", err)
	}
	if client.version != "未知" {
		t.Fatalf("empty version body should fallback to 未知, got %q", client.version)
	}
}

func TestClashClientReadsLargeProxiesResponse(t *testing.T) {
	longNode := strings.Repeat("node-", 1200)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.2.3"})
		case r.URL.Path == "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"proxies": map[string]interface{}{
					"Proxy": map[string]interface{}{"type": "Selector", "all": []string{longNode}, "now": longNode},
				},
			})
		case r.URL.Path == "/proxies/Proxy":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "Selector", "all": []string{longNode}, "now": longNode})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClashClient(ClashConfig{Enabled: true, APIURL: server.URL})
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("large /proxies response should not be truncated: %v", err)
	}
	if len(client.nodes) != 1 || client.nodes[0] != longNode {
		t.Fatalf("large node name not parsed correctly")
	}
}

func TestClashClientFiltersSubscriptionMetadataNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.2.3"})
		case r.URL.Path == "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"proxies": map[string]interface{}{
					"GLOBAL": map[string]interface{}{"type": "Selector", "all": []string{
						"DIRECT",
						"剩余流量：53.63 GB",
						"距离下次重置剩余：25 天",
						"套餐到期：2026-07-11",
						"建议：感到卡顿请切换到专线节点",
						"放丢失官网:https://love.p6m6.com",
						"🇺🇸【北美洲】美国01原生丨直连【2x】",
					}, "now": "建议：感到卡顿请切换到专线节点"},
				},
			})
		case r.URL.Path == "/proxies/GLOBAL":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "Selector", "all": []string{
				"DIRECT",
				"剩余流量：53.63 GB",
				"距离下次重置剩余：25 天",
				"套餐到期：2026-07-11",
				"建议：感到卡顿请切换到专线节点",
				"放丢失官网:https://love.p6m6.com",
				"🇺🇸【北美洲】美国01原生丨直连【2x】",
			}, "now": "建议：感到卡顿请切换到专线节点"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClashClient(ClashConfig{Enabled: true, APIURL: server.URL, ProxyGroup: "GLOBAL"})
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if len(client.nodes) != 1 || client.nodes[0] != "🇺🇸【北美洲】美国01原生丨直连【2x】" {
		t.Fatalf("metadata nodes should be filtered, got %v", client.nodes)
	}
	if client.nodeIndex != -1 {
		t.Fatalf("current metadata node should not set nodeIndex, got %d", client.nodeIndex)
	}
}

func TestClashClientFiltersPolicyGroupPseudoNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.2.3"})
		case r.URL.Path == "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"proxies": map[string]interface{}{
					"GLOBAL": map[string]interface{}{"type": "Selector", "all": []string{"🚀 节点选择", "☑️ 手动切换", "♻️ 自动选择", "🤖 OpenAi", "🇺🇲 美国节点", "🇺🇸 _洛杉矶-01"}, "now": "🚀 节点选择"},
				},
			})
		case r.URL.Path == "/proxies/GLOBAL":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "Selector", "all": []string{"🚀 节点选择", "☑️ 手动切换", "♻️ 自动选择", "🤖 OpenAi", "🇺🇲 美国节点", "🇺🇸 _洛杉矶-01"}, "now": "🚀 节点选择"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClashClient(ClashConfig{Enabled: true, APIURL: server.URL, ProxyGroup: "GLOBAL"})
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if len(client.nodes) != 1 || client.nodes[0] != "🇺🇸 _洛杉矶-01" {
		t.Fatalf("policy group pseudo nodes should be filtered, got %v", client.nodes)
	}
}

func TestClashClientFiltersNestedProxyGroups(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.2.3"})
		case r.URL.Path == "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"proxies": map[string]interface{}{
					"GLOBAL":        map[string]interface{}{"type": "Selector", "all": []string{"Free Cloud", "自动选择", "故障转移", "日本东京1-AN | 1x"}, "now": "Free Cloud"},
					"Free Cloud":    map[string]interface{}{"type": "Selector", "all": []string{"自动选择", "故障转移", "日本东京1-AN | 1x"}, "now": "自动选择"},
					"自动选择":          map[string]interface{}{"type": "URLTest", "all": []string{"日本东京1-AN | 1x"}, "now": "日本东京1-AN | 1x"},
					"故障转移":          map[string]interface{}{"type": "Fallback", "all": []string{"日本东京1-AN | 1x"}, "now": "日本东京1-AN | 1x"},
					"日本东京1-AN | 1x": map[string]interface{}{"type": "Vless"},
				},
			})
		case r.URL.Path == "/proxies/GLOBAL":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "Selector", "all": []string{"Free Cloud", "自动选择", "故障转移", "日本东京1-AN | 1x"}, "now": "Free Cloud"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClashClient(ClashConfig{Enabled: true, APIURL: server.URL, ProxyGroup: "GLOBAL"})
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if len(client.nodes) != 1 || client.nodes[0] != "日本东京1-AN | 1x" {
		t.Fatalf("nested selector/urltest/fallback groups should be filtered, got %v", client.nodes)
	}
	if client.nodeIndex != -1 {
		t.Fatalf("current nested group should not set nodeIndex, got %d", client.nodeIndex)
	}
}

func TestClashClientQuarantinesNodeAfterThreeNetworkFailures(t *testing.T) {
	var switched []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.2.3"})
		case r.URL.Path == "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"proxies": map[string]interface{}{
				"Proxy": map[string]interface{}{"type": "Selector", "all": []string{"bad", "good"}, "now": "bad"},
			}})
		case r.URL.Path == "/proxies/Proxy" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "Selector", "all": []string{"bad", "good"}, "now": "bad"})
		case r.URL.Path == "/proxies/Proxy" && r.Method == http.MethodPut:
			var body struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			switched = append(switched, body.Name)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/delay"):
			_ = json.NewEncoder(w).Encode(map[string]int{"delay": 10})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClashClient(ClashConfig{Enabled: true, APIURL: server.URL, TestTimeout: 1})
	client.RecordNodeNetworkFailure("bad")
	client.RecordNodeNetworkFailure("bad")
	if client.QuarantinedNodeCount() != 0 {
		t.Fatalf("node should not be quarantined before third failure")
	}
	client.RecordNodeNetworkFailure("bad")
	if client.QuarantinedNodeCount() != 1 {
		t.Fatalf("node should be quarantined after third failure")
	}
	selection, err := client.SwitchToNextAvailable(context.Background())
	if err != nil {
		t.Fatalf("SwitchToNextAvailable: %v", err)
	}
	if selection.Node != "good" {
		t.Fatalf("quarantined bad node should be skipped, got %q", selection.Node)
	}
	if strings.Contains(strings.Join(switched, ","), "bad") {
		t.Fatalf("should not switch to quarantined node, switches=%v", switched)
	}
}

func TestClashClientQuarantinesNodeAfterRiskFailure(t *testing.T) {
	var switched []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.2.3"})
		case r.URL.Path == "/proxies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"proxies": map[string]interface{}{
				"Proxy": map[string]interface{}{"type": "Selector", "all": []string{"risk", "good"}, "now": "risk"},
			}})
		case r.URL.Path == "/proxies/Proxy" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "Selector", "all": []string{"risk", "good"}, "now": "risk"})
		case r.URL.Path == "/proxies/Proxy" && r.Method == http.MethodPut:
			var body struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			switched = append(switched, body.Name)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/delay"):
			_ = json.NewEncoder(w).Encode(map[string]int{"delay": 10})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClashClient(ClashConfig{Enabled: true, APIURL: server.URL, TestTimeout: 1})
	client.RecordNodeRiskFailure("risk")
	if client.QuarantinedNodeCount() != 1 {
		t.Fatalf("risk failure should immediately quarantine the node")
	}
	selection, err := client.SwitchToNextAvailable(context.Background())
	if err != nil {
		t.Fatalf("SwitchToNextAvailable: %v", err)
	}
	if selection.Node != "good" {
		t.Fatalf("risk-quarantined node should be skipped, got %q", selection.Node)
	}
	if strings.Contains(strings.Join(switched, ","), "risk") {
		t.Fatalf("should not switch to risk-quarantined node, switches=%v", switched)
	}
}

func TestClashClientSuccessClearsNetworkFailures(t *testing.T) {
	client := NewClashClient(ClashConfig{Enabled: true, APIURL: "http://127.0.0.1:9"})
	client.RecordNodeNetworkFailure("node-a")
	client.RecordNodeNetworkFailure("node-a")
	client.RecordNodeSuccess("node-a")
	client.RecordNodeNetworkFailure("node-a")
	if client.QuarantinedNodeCount() != 0 {
		t.Fatalf("success should reset failure count before quarantine")
	}
}

func TestClashClientQuarantineExpires(t *testing.T) {
	client := NewClashClient(ClashConfig{Enabled: true, APIURL: "http://127.0.0.1:9"})
	client.RecordNodeNetworkFailure("node-a")
	client.RecordNodeNetworkFailure("node-a")
	client.RecordNodeNetworkFailure("node-a")
	if client.QuarantinedNodeCount() != 1 {
		t.Fatalf("node should be quarantined")
	}
	client.quarantinedUntil["node-a"] = time.Now().Add(-time.Second)
	if client.QuarantinedNodeCount() != 0 {
		t.Fatalf("expired quarantine should not count")
	}
	if client.failedNodes["node-a"] {
		t.Fatalf("expired quarantine should clear failed node marker")
	}
}
