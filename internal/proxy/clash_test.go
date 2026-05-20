package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
