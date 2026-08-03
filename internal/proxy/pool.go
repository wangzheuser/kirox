package proxy

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reg_go/internal/fileutil"
)

// PoolEntry 多代理池条目
type PoolEntry struct {
	ID      string `json:"id"`      // 内部 ID，UI 用
	Name    string `json:"name"`    // 用户可见名称
	URL     string `json:"url"`     // 完整代理 URL（已归一化）
	Weight  int    `json:"weight"`  // 1-100，越高被选中概率越大
	Enabled bool   `json:"enabled"` // 关闭时不参与抽签
}

// poolFile JSON 持久化结构
type poolFile struct {
	Entries []PoolEntry `json:"entries"`
}

const (
	// Power 用于"软最大化"：>1 时拉大权重差，<1 时压平。0.6 保证哪怕权重 1 vs 100 也有 ~6% 概率被选中。
	weightPower            = 0.6
	poolQuarantineFailures = 3
	poolQuarantineDuration = 30 * time.Minute
)

var (
	poolMu          sync.Mutex
	poolLoaded      bool
	poolEntries     []PoolEntry
	poolPath        string
	poolFailures    = make(map[string]int)
	poolQuarantined = make(map[string]time.Time)
)

// InitPool 在 App 启动时调用一次，传入数据目录
func InitPool(dataDir string) {
	poolMu.Lock()
	defer poolMu.Unlock()
	poolPath = filepath.Join(dataDir, "proxy_pool.json")
	poolLoaded = false
	poolFailures = make(map[string]int)
	poolQuarantined = make(map[string]time.Time)
	_ = loadPoolLocked()
}

func loadPoolLocked() error {
	if poolLoaded {
		return nil
	}
	poolEntries = nil
	poolLoaded = true
	b, err := os.ReadFile(poolPath)
	if err != nil {
		return nil
	}
	var pf poolFile
	if err := json.Unmarshal(b, &pf); err != nil {
		return fmt.Errorf("解析代理池失败: %w", err)
	}
	poolEntries = pf.Entries
	return nil
}

func savePoolLocked() error {
	if poolPath == "" {
		return fmt.Errorf("代理池未初始化")
	}
	b, err := json.MarshalIndent(poolFile{Entries: poolEntries}, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(poolPath, b, 0o600)
}

// List 返回当前所有代理（含禁用项）
func List() []PoolEntry {
	poolMu.Lock()
	defer poolMu.Unlock()
	loadPoolLocked()
	out := make([]PoolEntry, len(poolEntries))
	copy(out, poolEntries)
	return out
}

func newID() string {
	return fmt.Sprintf("p_%d_%04d", time.Now().UnixNano(), rand.Intn(10000))
}

// Add 新增一条代理。url 会被外部归一化后传入。
func Add(entry PoolEntry) (PoolEntry, error) {
	entry.URL = strings.TrimSpace(entry.URL)
	if entry.URL == "" {
		return entry, fmt.Errorf("代理地址不能为空")
	}
	if entry.Weight <= 0 {
		entry.Weight = 50
	}
	if entry.Weight > 100 {
		entry.Weight = 100
	}
	entry.Name = strings.TrimSpace(entry.Name)
	if entry.Name == "" {
		entry.Name = entry.URL
	}
	poolMu.Lock()
	defer poolMu.Unlock()
	loadPoolLocked()
	for _, e := range poolEntries {
		if e.URL == entry.URL {
			return entry, fmt.Errorf("该代理已存在")
		}
	}
	if entry.ID == "" {
		entry.ID = newID()
	}
	entry.Enabled = true
	poolEntries = append(poolEntries, entry)
	if err := savePoolLocked(); err != nil {
		// 回滚
		poolEntries = poolEntries[:len(poolEntries)-1]
		return entry, err
	}
	return entry, nil
}

// Update 修改一条（按 id 匹配）。url 不允许改成已存在的另一条。
func Update(id string, patch PoolEntry) (PoolEntry, error) {
	poolMu.Lock()
	defer poolMu.Unlock()
	loadPoolLocked()
	idx := -1
	for i, e := range poolEntries {
		if e.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return PoolEntry{}, fmt.Errorf("代理不存在")
	}
	cur := poolEntries[idx]
	if name := strings.TrimSpace(patch.Name); name != "" {
		cur.Name = name
	}
	if u := strings.TrimSpace(patch.URL); u != "" && u != cur.URL {
		for j, e := range poolEntries {
			if j != idx && e.URL == u {
				return PoolEntry{}, fmt.Errorf("该代理 URL 已存在")
			}
		}
		cur.URL = u
	}
	if patch.Weight > 0 {
		w := patch.Weight
		if w > 100 {
			w = 100
		}
		cur.Weight = w
	}
	cur.Enabled = patch.Enabled
	poolEntries[idx] = cur
	if err := savePoolLocked(); err != nil {
		return PoolEntry{}, err
	}
	return cur, nil
}

// Delete 按 id 删除
func Delete(id string) error {
	poolMu.Lock()
	defer poolMu.Unlock()
	loadPoolLocked()
	for i, e := range poolEntries {
		if e.ID == id {
			poolEntries = append(poolEntries[:i], poolEntries[i+1:]...)
			return savePoolLocked()
		}
	}
	return fmt.Errorf("代理不存在")
}

// PickRandomEntry 按权重抽签返回一个未隔离的启用条目。
func PickRandomEntry() (PoolEntry, bool) {
	poolMu.Lock()
	defer poolMu.Unlock()
	loadPoolLocked()

	type cand struct {
		entry PoolEntry
		soft  float64
	}
	candidates := make([]cand, 0, len(poolEntries))
	var total float64
	for _, e := range poolEntries {
		if !e.Enabled || e.URL == "" {
			continue
		}
		if isPoolProxyQuarantinedLocked(e.URL, time.Now()) {
			continue
		}
		w := e.Weight
		if w <= 0 {
			w = 1
		}
		soft := math.Pow(float64(w), weightPower)
		candidates = append(candidates, cand{e, soft})
		total += soft
	}
	if total <= 0 || len(candidates) == 0 {
		return PoolEntry{}, false
	}
	r := rand.Float64() * total
	for _, c := range candidates {
		r -= c.soft
		if r <= 0 {
			return c.entry, true
		}
	}
	return candidates[len(candidates)-1].entry, true
}

// PickRandom 保留现有调用契约。
func PickRandom() string {
	entry, ok := PickRandomEntry()
	if !ok {
		return ""
	}
	return entry.URL
}

func RecordPoolProxyNetworkFailure(proxyURL string) {
	key := strings.TrimSpace(proxyURL)
	if key == "" {
		return
	}
	poolMu.Lock()
	defer poolMu.Unlock()
	poolFailures[key]++
	if poolFailures[key] >= poolQuarantineFailures {
		poolQuarantined[key] = time.Now().Add(poolQuarantineDuration)
	}
}

func RecordPoolProxySuccess(proxyURL string) {
	key := strings.TrimSpace(proxyURL)
	if key == "" {
		return
	}
	poolMu.Lock()
	defer poolMu.Unlock()
	delete(poolFailures, key)
	delete(poolQuarantined, key)
}

func QuarantinedPoolProxyCount() int {
	poolMu.Lock()
	defer poolMu.Unlock()
	now := time.Now()
	count := 0
	for proxyURL := range poolQuarantined {
		if isPoolProxyQuarantinedLocked(proxyURL, now) {
			count++
		}
	}
	return count
}

func isPoolProxyQuarantinedLocked(proxyURL string, now time.Time) bool {
	until, ok := poolQuarantined[proxyURL]
	if !ok {
		return false
	}
	if now.After(until) {
		delete(poolQuarantined, proxyURL)
		delete(poolFailures, proxyURL)
		return false
	}
	return true
}

// HasEnabled 是否至少一个启用的池条目
func HasEnabled() bool {
	poolMu.Lock()
	defer poolMu.Unlock()
	loadPoolLocked()
	for _, e := range poolEntries {
		if e.Enabled && e.URL != "" {
			return true
		}
	}
	return false
}
