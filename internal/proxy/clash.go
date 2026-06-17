package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultClashAPIURL 是 Clash External Controller 的默认地址。
	DefaultClashAPIURL = "http://127.0.0.1:9097"

	defaultClashTestTimeout = 10 * time.Second
	clashSwitchWait         = 500 * time.Millisecond
	maxClashAPIResponseSize = 8 * 1024 * 1024
)

var clashPriorityGroups = []string{"GLOBAL", "Proxy", "节点选择", "代理", "手动选择", "Select", "🚀 节点选择"}

// ClashConfig 描述 Clash External Controller 自动切换配置。
type ClashConfig struct {
	Enabled              bool   `json:"enabled"`
	APIURL               string `json:"apiUrl"`
	APISecret            string `json:"apiSecret"`
	ProxyGroup           string `json:"proxyGroup"`
	TestURL              string `json:"testUrl"`
	TestTimeout          int    `json:"testTimeout"`
	SkipConnectivityTest bool   `json:"skipConnectivityTest"`
}

// ClashSelection 描述一次 Clash 节点选择结果。
type ClashSelection struct {
	Version     string        `json:"version,omitempty"`
	ProxyGroup  string        `json:"proxyGroup,omitempty"`
	Node        string        `json:"node,omitempty"`
	Attempts    int           `json:"attempts,omitempty"`
	DelayMs     int           `json:"delayMs,omitempty"`
	TargetURL   string        `json:"target,omitempty"`
	Duration    time.Duration `json:"-"`
	DurationMs  int64         `json:"durationMs,omitempty"`
	SkippedTest bool          `json:"skippedTest,omitempty"`
	Errors      []string      `json:"errors,omitempty"`
}

// ClashClient 通过 Clash RESTful API 管理代理组节点。
type ClashClient struct {
	config      ClashConfig
	httpClient  *http.Client
	version     string
	proxyGroup  string
	nodeIndex   int
	nodes       []string
	failedNodes map[string]bool
	initialized bool
}

type clashProxyGroup struct {
	Type string   `json:"type"`
	All  []string `json:"all"`
	Now  string   `json:"now"`
}

// NormalizeClashConfig 填充 Clash 配置默认值。
func NormalizeClashConfig(config ClashConfig) ClashConfig {
	config.APIURL = strings.TrimRight(strings.TrimSpace(config.APIURL), "/")
	config.APISecret = strings.TrimSpace(config.APISecret)
	config.ProxyGroup = strings.TrimSpace(config.ProxyGroup)
	config.TestURL = strings.TrimSpace(config.TestURL)
	if config.APIURL == "" {
		config.APIURL = DefaultClashAPIURL
	}
	if config.TestURL == "" {
		config.TestURL = DefaultProbeTarget
	}
	if config.TestTimeout <= 0 {
		config.TestTimeout = int(defaultClashTestTimeout / time.Second)
	}
	return config
}

// NewClashClient 创建 Clash API 客户端。
func NewClashClient(config ClashConfig) *ClashClient {
	return newClashClient(config, http.DefaultClient)
}

func newClashClient(config ClashConfig, client *http.Client) *ClashClient {
	config = NormalizeClashConfig(config)
	if client == nil {
		client = http.DefaultClient
	}
	return &ClashClient{
		config:      config,
		httpClient:  client,
		nodeIndex:   -1,
		failedNodes: make(map[string]bool),
	}
}

// Initialize 初始化 Clash 客户端，检测代理组并缓存节点列表。
func (c *ClashClient) Initialize(ctx context.Context) error {
	if c.initialized {
		return nil
	}

	version, err := c.testConnection(ctx)
	if err != nil {
		return err
	}
	c.version = version

	group := c.config.ProxyGroup
	if group == "" {
		group, err = c.findSelectorGroup(ctx)
		if err != nil {
			return err
		}
	}
	c.proxyGroup = group

	nodes, err := c.getAvailableNodes(ctx, group)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return fmt.Errorf("Clash 代理组 %q 没有可用节点", group)
	}
	c.nodes = nodes

	if current, err := c.getCurrentNode(ctx, group); err == nil && current != "" {
		for i, node := range c.nodes {
			if node == current {
				c.nodeIndex = i
				break
			}
		}
	}

	c.initialized = true
	return nil
}

// SwitchToNextAvailable 切换到下一个可用 Clash 节点。
func (c *ClashClient) SwitchToNextAvailable(ctx context.Context) (ClashSelection, error) {
	start := time.Now()
	if err := c.Initialize(ctx); err != nil {
		return ClashSelection{Duration: time.Since(start), DurationMs: time.Since(start).Milliseconds()}, err
	}
	if len(c.nodes) == 0 {
		return ClashSelection{Version: c.version, ProxyGroup: c.proxyGroup}, fmt.Errorf("Clash 没有可用节点")
	}

	if c.config.SkipConnectivityTest {
		selection, err := c.switchToNext(ctx, start)
		selection.SkippedTest = true
		return selection, err
	}

	if c.availableNodeCount() == 0 {
		c.failedNodes = make(map[string]bool)
	}

	var errs []string
	for tried := 0; tried < len(c.nodes); tried++ {
		if err := ctx.Err(); err != nil {
			return c.selection("", tried, 0, start, false, errs), err
		}

		c.nodeIndex = (c.nodeIndex + 1) % len(c.nodes)
		node := c.nodes[c.nodeIndex]
		if c.failedNodes[node] {
			continue
		}

		if err := c.switchNode(ctx, node); err != nil {
			errs = appendSelectorError(errs, fmt.Sprintf("节点 %s 切换失败: %s", node, c.sanitizeError(err.Error())))
			continue
		}
		sleepWithContext(ctx, clashSwitchWait)

		delay, err := c.testNodeDelay(ctx, node)
		if err == nil {
			return c.selection(node, tried+1, delay, start, false, errs), nil
		}

		c.failedNodes[node] = true
		errs = appendSelectorError(errs, fmt.Sprintf("节点 %s 不可用: %s", node, c.sanitizeError(err.Error())))
	}

	return c.selection("", len(c.nodes), 0, start, false, errs), fmt.Errorf("Clash 所有节点均不可用: %s", strings.Join(errs, "；"))
}

func (c *ClashClient) switchToNext(ctx context.Context, start time.Time) (ClashSelection, error) {
	c.nodeIndex = (c.nodeIndex + 1) % len(c.nodes)
	node := c.nodes[c.nodeIndex]
	if err := c.switchNode(ctx, node); err != nil {
		return c.selection(node, 1, 0, start, true, nil), err
	}
	return c.selection(node, 1, 0, start, true, nil), nil
}

func (c *ClashClient) selection(node string, attempts, delay int, start time.Time, skipped bool, errs []string) ClashSelection {
	duration := time.Since(start)
	return ClashSelection{
		Version:     c.version,
		ProxyGroup:  c.proxyGroup,
		Node:        node,
		Attempts:    attempts,
		DelayMs:     delay,
		TargetURL:   c.config.TestURL,
		Duration:    duration,
		DurationMs:  duration.Milliseconds(),
		SkippedTest: skipped,
		Errors:      errs,
	}
}

func (c *ClashClient) testConnection(ctx context.Context) (string, error) {
	var data struct {
		Version string `json:"version"`
	}
	if err := c.apiRequest(ctx, http.MethodGet, "/version", nil, &data, 5*time.Second); err != nil {
		return "", fmt.Errorf("Clash API 连接失败: %w", err)
	}
	if data.Version == "" {
		data.Version = "未知"
	}
	return data.Version, nil
}

func (c *ClashClient) findSelectorGroup(ctx context.Context) (string, error) {
	proxies, err := c.getAllProxies(ctx)
	if err != nil {
		return "", err
	}
	for _, name := range clashPriorityGroups {
		if group, ok := proxies[name]; ok && group.Type == "Selector" {
			return name, nil
		}
	}
	for name, group := range proxies {
		if group.Type == "Selector" && len(group.All) > 0 {
			return name, nil
		}
	}
	return "", fmt.Errorf("未找到可用的 Clash Selector 代理组")
}

func (c *ClashClient) getAllProxies(ctx context.Context) (map[string]clashProxyGroup, error) {
	var data struct {
		Proxies map[string]clashProxyGroup `json:"proxies"`
	}
	if err := c.apiRequest(ctx, http.MethodGet, "/proxies", nil, &data, 8*time.Second); err != nil {
		return nil, err
	}
	if data.Proxies == nil {
		return nil, fmt.Errorf("Clash API 返回代理列表为空")
	}
	return data.Proxies, nil
}

func (c *ClashClient) getGroupInfo(ctx context.Context, group string) (clashProxyGroup, error) {
	var data clashProxyGroup
	if strings.TrimSpace(group) == "" {
		return data, fmt.Errorf("Clash 代理组为空")
	}
	endpoint := "/proxies/" + url.PathEscape(group)
	if err := c.apiRequest(ctx, http.MethodGet, endpoint, nil, &data, 8*time.Second); err != nil {
		return data, err
	}
	return data, nil
}

func (c *ClashClient) getAvailableNodes(ctx context.Context, group string) ([]string, error) {
	info, err := c.getGroupInfo(ctx, group)
	if err != nil {
		return nil, err
	}
	special := map[string]bool{"DIRECT": true, "REJECT": true, "PASS": true, "COMPATIBLE": true}
	nodes := make([]string, 0, len(info.All))
	for _, node := range info.All {
		if special[node] || isClashMetadataNode(node) {
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func isClashMetadataNode(node string) bool {
	name := strings.TrimSpace(node)
	if name == "" {
		return true
	}
	lower := strings.ToLower(name)
	metadataPrefixes := []string{
		"剩余流量",
		"距离下次重置剩余",
		"套餐到期",
		"建议：",
		"建议:",
		"官网:",
		"官网：",
		"放丢失官网:",
		"防丢失官网:",
		"流量:",
		"流量：",
		"到期:",
		"到期：",
		"🚀 节点选择",
		"☑️ 手动切换",
		"♻️ 自动选择",
		"🤖 openai",
		"openai",
	}
	for _, prefix := range metadataPrefixes {
		if strings.HasPrefix(name, prefix) || strings.HasPrefix(lower, strings.ToLower(prefix)) {
			return true
		}
	}
	policyGroupNames := []string{
		"节点选择", "手动切换", "自动选择", "故障转移", "负载均衡", "漏网之鱼",
		"美国节点", "香港节点", "日本节点", "新加坡节点", "台湾节点", "韩国节点",
		"us nodes", "hk nodes", "jp nodes", "sg nodes", "tw nodes", "kr nodes",
	}
	trimmedEmoji := strings.TrimLeft(name, " 🚀☑️♻️🤖🇺🇸🇺🇲🇭🇰🇯🇵🇸🇬🇹🇼🇰🇷")
	lowerTrimmedEmoji := strings.ToLower(strings.TrimSpace(trimmedEmoji))
	for _, groupName := range policyGroupNames {
		groupLower := strings.ToLower(groupName)
		if lower == groupLower || lowerTrimmedEmoji == groupLower {
			return true
		}
	}
	return false
}

func (c *ClashClient) getCurrentNode(ctx context.Context, group string) (string, error) {
	info, err := c.getGroupInfo(ctx, group)
	if err != nil {
		return "", err
	}
	return info.Now, nil
}

func (c *ClashClient) switchNode(ctx context.Context, node string) error {
	payload := map[string]string{"name": node}
	endpoint := "/proxies/" + url.PathEscape(c.proxyGroup)
	return c.apiRequest(ctx, http.MethodPut, endpoint, payload, nil, 8*time.Second)
}

func (c *ClashClient) testNodeDelay(ctx context.Context, node string) (int, error) {
	timeout := time.Duration(c.config.TestTimeout) * time.Second
	params := url.Values{}
	params.Set("timeout", fmt.Sprintf("%d", timeout.Milliseconds()))
	params.Set("url", c.config.TestURL)
	endpoint := "/proxies/" + url.PathEscape(node) + "/delay?" + params.Encode()

	var data struct {
		Delay   int    `json:"delay"`
		Message string `json:"message"`
	}
	if err := c.apiRequest(ctx, http.MethodGet, endpoint, nil, &data, timeout+2*time.Second); err != nil {
		return 0, err
	}
	if data.Delay > 0 {
		return data.Delay, nil
	}
	if data.Message != "" {
		return 0, errors.New(data.Message)
	}
	return 0, fmt.Errorf("节点延迟测试失败")
}

func (c *ClashClient) apiRequest(ctx context.Context, method, endpoint string, payload interface{}, out interface{}, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	body, err := encodeJSONBody(payload)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, c.config.APIURL+endpoint, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.config.APISecret != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APISecret)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.New(c.sanitizeError(err.Error()))
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxClashAPIResponseSize+1))
	if len(respBody) > maxClashAPIResponseSize {
		return fmt.Errorf("Clash API 响应过大，超过 %d MB", maxClashAPIResponseSize/1024/1024)
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, c.sanitizeError(strings.TrimSpace(string(respBody))))
	}
	if out == nil {
		return nil
	}
	if strings.TrimSpace(string(respBody)) == "" {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("解析 Clash API 响应失败: %w", err)
	}
	return nil
}

func (c *ClashClient) availableNodeCount() int {
	count := 0
	for _, node := range c.nodes {
		if !c.failedNodes[node] {
			count++
		}
	}
	return count
}

func (c *ClashClient) sanitizeError(msg string) string {
	if c.config.APISecret != "" {
		msg = strings.ReplaceAll(msg, c.config.APISecret, "***")
	}
	if len(msg) > 160 {
		msg = msg[:160] + "..."
	}
	return msg
}

func encodeJSONBody(payload interface{}) (io.Reader, error) {
	if payload == nil {
		return nil, nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(body), nil
}

func sleepWithContext(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
