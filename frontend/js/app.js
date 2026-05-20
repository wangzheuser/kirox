// ===== 核心：导航 / 标签页 / 下拉框 / 配置 / 卡密 / Toast / 窗口控制 =====

// 页面切换
var pageTitles = { overview: '概览', logs: '运行日志', register: '注册', accounts: '邮箱池', subscription: '获取订阅支付链接', info: '关于', settings: '设置' };
function switchPage(pageId) {
  document.querySelectorAll('.page, .page-placeholder, .page-iframe').forEach(function(p) {
    p.classList.remove('active');
  });
  var target = document.getElementById('page-' + pageId);
  if (target) target.classList.add('active');
  document.querySelectorAll('.nav-item[data-page]').forEach(function(item) {
    item.classList.toggle('active', item.getAttribute('data-page') === pageId);
  });
  document.getElementById('titlebar-text').textContent = pageTitles[pageId] || pageId;
  if (pageId === 'overview') {
    startOverviewTimer();
  } else {
    stopOverviewTimer();
  }
  if (pageId === 'accounts') {
    loadOutlookAccountsList();
    startOutlookAutoRefresh();
  } else {
    stopOutlookAutoRefresh();
  }
  if (pageId === 'info') {
    loadInfoVersion();
  }
  if (pageId === 'subscription') {
    reloadSubscriptionAccounts();
  }
}

// 标签页切换
function switchTab(tabId) {
  var tabBar = document.querySelector('.tab-item[data-tab="' + tabId + '"]').parentElement;
  tabBar.querySelectorAll('.tab-item').forEach(function(t) {
    t.classList.toggle('active', t.getAttribute('data-tab') === tabId);
  });
  var page = tabBar.parentElement;
  page.querySelectorAll('.tab-panel').forEach(function(p) {
    p.classList.remove('active');
  });
  var target = document.getElementById('tab-' + tabId);
  if (target) target.classList.add('active');
}

// 下拉框
function toggleDropdown(id) {
  var dropdown = document.getElementById(id);
  var selected = dropdown.querySelector('.dropdown-selected');
  var options = dropdown.querySelector('.dropdown-options');
  document.querySelectorAll('.dropdown-options.show').forEach(function(el) {
    if (el !== options) {
      el.classList.remove('show');
      el.parentElement.querySelector('.dropdown-selected').classList.remove('active');
    }
  });
  selected.classList.toggle('active');
  options.classList.toggle('show');
}

document.addEventListener('click', function(e) {
  if (!e.target.closest('.custom-dropdown')) {
    document.querySelectorAll('.dropdown-options.show').forEach(function(el) {
      el.classList.remove('show');
      el.parentElement.querySelector('.dropdown-selected').classList.remove('active');
    });
  }
});

async function loadInfoVersion() {
  try {
    var data = await window.go.main.App.GetOverview();
    var ver = (data && data.version) ? data.version : '';
    if (ver) {
      ['info-version-detail', 'info-version-detail2'].forEach(function(id) {
        var el = document.getElementById(id);
        if (el) el.textContent = ver;
      });
    }
  } catch(e) {}

  // 从 GitHub 加载最新 release 信息
  var changelogEl = document.getElementById('info-changelog');
  var latestEl = document.getElementById('info-latest-version');
  var dateEl = document.getElementById('info-release-date');
  var tagEl = document.getElementById('info-changelog-version');
  if (changelogEl) changelogEl.innerHTML = '<span style="color:var(--text-muted);">加载中...</span>';
  try {
    var result = await window.go.main.App.CheckUpdate();
    if (result.error) {
      if (changelogEl) changelogEl.innerHTML = '<span style="color:var(--text-muted);">加载失败: ' + result.error + '</span>';
      return;
    }
    if (latestEl) {
      latestEl.textContent = result.latestVersion || '-';
      latestEl.style.color = result.hasUpdate ? 'var(--success)' : 'var(--text)';
    }
    if (dateEl) dateEl.textContent = result.releaseDate || '-';
    if (tagEl) tagEl.textContent = result.latestVersion || '-';
    var banner = document.getElementById('info-update-banner');
    var bannerVer = document.getElementById('info-banner-version');
    if (banner) banner.style.display = result.hasUpdate ? 'block' : 'none';
    if (bannerVer) bannerVer.textContent = result.latestVersion || '';
    if (changelogEl) {
      var body = (result.changelog || '').trim();
      changelogEl.innerHTML = body ? renderChangelog(body) : '<span style="color:var(--text-muted);">暂无更新说明</span>';
    }
  } catch(e) {
    if (changelogEl) changelogEl.innerHTML = '<span style="color:var(--text-muted);">加载失败</span>';
  }
}

// 存储目录设置
async function loadDataDir() {
  try {
    var dir = await window.go.main.App.GetDataDir();
    document.getElementById('cfg-data-dir').value = dir || '';
  } catch(e) {}
}

async function selectDataDir() {
  try {
    var path = await window.go.main.App.SelectDirectory();
    if (!path) return;
    var result = await window.go.main.App.SetDataDir(path);
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }
    document.getElementById('cfg-data-dir').value = result.path;
    showToast('存储目录已更新，重启后完全生效');
  } catch(e) {
    showToast('设置失败: ' + e.message, 'error');
  }
}

async function resetDataDir() {
  try {
    var result = await window.go.main.App.ResetDataDir();
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }
    document.getElementById('cfg-data-dir').value = result.path;
    showToast('已恢复默认存储目录');
  } catch(e) {
    showToast('重置失败: ' + e.message, 'error');
  }
}

// 注册结果输出目录设置
async function loadResultOutputDir() {
  try {
    var dir = await window.go.main.App.GetResultOutputDir();
    var el = document.getElementById('cfg-result-output-dir');
    if (el) el.value = dir || '';
  } catch(e) {}
}

async function selectResultOutputDir() {
  try {
    var path = await window.go.main.App.SelectDirectory();
    if (!path) return;
    var result = await window.go.main.App.SetResultOutputDir(path);
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }
    document.getElementById('cfg-result-output-dir').value = result.path;
    showToast('注册结果将写入: ' + result.path);
  } catch(e) {
    showToast('设置失败: ' + e.message, 'error');
  }
}

async function resetResultOutputDir() {
  try {
    var result = await window.go.main.App.ResetResultOutputDir();
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }
    document.getElementById('cfg-result-output-dir').value = result.path;
    showToast('已恢复默认输出目录');
  } catch(e) {
    showToast('重置失败: ' + e.message, 'error');
  }
}

// 模拟页面停留时间设置
var pageStaySaveTimer = null;
var lastSavedPageStayPayload = '';

function pageStaySecondsText(ms) {
  var seconds = (parseInt(ms, 10) || 0) / 1000;
  return Number.isInteger(seconds) ? String(seconds) : String(Number(seconds.toFixed(3)));
}

function readPageStayMs(inputId, label) {
  var el = document.getElementById(inputId);
  var value = parseFloat(el && el.value);
  if (!Number.isFinite(value) || value < 0) {
    throw new Error(label + '必须大于或等于 0');
  }
  return Math.round(value * 1000);
}

async function loadPageStayConfig() {
  try {
    var cfg = await window.go.main.App.GetPageStayConfig();
    var minEl = document.getElementById('cfg-page-stay-min-seconds');
    var maxEl = document.getElementById('cfg-page-stay-max-seconds');
    if (minEl) minEl.value = pageStaySecondsText(cfg.minMs);
    if (maxEl) maxEl.value = pageStaySecondsText(cfg.maxMs);
    lastSavedPageStayPayload = JSON.stringify({ minMs: cfg.minMs, maxMs: cfg.maxMs });
  } catch(e) {}
}

async function savePageStayConfig() {
  try {
    var minMs = readPageStayMs('cfg-page-stay-min-seconds', '最小停留时间');
    var maxMs = readPageStayMs('cfg-page-stay-max-seconds', '最大停留时间');
    if (minMs > maxMs) {
      throw new Error('最小停留时间不能大于最大停留时间');
    }
    var payload = { minMs: minMs, maxMs: maxMs };
    var payloadKey = JSON.stringify(payload);
    if (payloadKey === lastSavedPageStayPayload) {
      return;
    }
    var result = await window.go.main.App.SetPageStayConfig(payload);
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }
    var cfg = result.config || { minMs: minMs, maxMs: maxMs };
    var minEl = document.getElementById('cfg-page-stay-min-seconds');
    var maxEl = document.getElementById('cfg-page-stay-max-seconds');
    if (minEl) minEl.value = pageStaySecondsText(cfg.minMs);
    if (maxEl) maxEl.value = pageStaySecondsText(cfg.maxMs);
    lastSavedPageStayPayload = JSON.stringify({ minMs: cfg.minMs, maxMs: cfg.maxMs });
    showToast(cfg.minMs === 0 && cfg.maxMs === 0 ? '已关闭模拟页面停留' : '模拟页面停留时间已保存');
  } catch(e) {
    showToast('保存失败: ' + e.message, 'error');
  }
}

function schedulePageStayConfigSave() {
  if (pageStaySaveTimer) {
    clearTimeout(pageStaySaveTimer);
  }
  pageStaySaveTimer = setTimeout(function() {
    pageStaySaveTimer = null;
    savePageStayConfig();
  }, 450);
}

// Outlook 读取方式设置
async function loadOutlookScope() {
  try {
    var scope = await window.go.main.App.GetOutlookScope();
    var el = document.getElementById('cfg-outlook-scope');
    if (el) el.value = scope || 'imap';
  } catch(e) {}
}

async function saveOutlookScope() {
  try {
    var el = document.getElementById('cfg-outlook-scope');
    var scope = (el && el.value) || 'imap';
    var result = await window.go.main.App.SetOutlookScope(scope);
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }
    if (el) el.value = result.scope || scope;
    showToast((result.scope || scope) === 'graph' ? 'Outlook 将使用 Microsoft Graph 读取验证码' : 'Outlook 将使用 IMAP 读取验证码');
  } catch(e) {
    showToast('保存失败: ' + e.message, 'error');
  }
}

function bindSettingsAutosave() {
  ['cfg-page-stay-min-seconds', 'cfg-page-stay-max-seconds'].forEach(function(id) {
    var el = document.getElementById(id);
    if (!el) return;
    el.addEventListener('input', schedulePageStayConfigSave);
    el.addEventListener('change', schedulePageStayConfigSave);
  });

  var outlookScopeEl = document.getElementById('cfg-outlook-scope');
  if (outlookScopeEl) {
    outlookScopeEl.addEventListener('change', saveOutlookScope);
  }
}

// 代理设置
async function loadProxy() {
  try {
    var p = await window.go.main.App.GetProxy();
    var el = document.getElementById('cfg-proxy');
    if (el) el.value = p || '';
  } catch(e) {}
}

async function loadClashProxy() {
  try {
    var p = await window.go.main.App.GetClashProxy();
    var el = document.getElementById('cfg-clash-proxy');
    if (el) el.value = p || '';
  } catch(e) {}
}

function currentProxyMode() {
  var checked = document.querySelector('input[name="proxy-mode"]:checked');
  return checked ? checked.value : 'none';
}

function applyProxyMode(mode) {
  mode = mode || 'none';
  var radio = document.getElementById('cfg-proxy-mode-' + mode);
  if (radio) radio.checked = true;
  var normalPanel = document.getElementById('proxy-normal-panel');
  var clashPanel = document.getElementById('proxy-clash-panel');
  if (normalPanel) normalPanel.style.display = mode === 'normal' ? 'flex' : 'none';
  if (clashPanel) clashPanel.style.display = mode === 'clash' ? 'flex' : 'none';
}

async function loadProxyMode() {
  try {
    var mode = await window.go.main.App.GetProxyMode();
    applyProxyMode(mode || 'none');
  } catch(e) {
    applyProxyMode('none');
  }
}

async function saveProxyMode(mode) {
  try {
    var result = await window.go.main.App.SetProxyMode(mode);
    if (result.error) {
      showToast(result.error, 'error');
      return null;
    }
    applyProxyMode(result.mode || mode);
    renderProxyDetectCard('hidden');
    showToast('代理模式已切换为' + ({ none: '直连', normal: '普通代理', clash: 'Clash 代理' }[result.mode || mode] || mode));
    return result.mode || mode;
  } catch(e) {
    showToast('代理模式保存失败: ' + e.message, 'error');
    return null;
  }
}

function proxyEscapeHtml(s) {
  return String(s || '').replace(/[&<>"']/g, function(ch) {
    return ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[ch];
  });
}

function getClashConfigFromForm() {
  var timeout = parseInt((document.getElementById('cfg-clash-test-timeout') || {}).value || '10', 10);
  if (!timeout || timeout < 1) timeout = 10;
  return {
    enabled: currentProxyMode() === 'clash',
    apiUrl: ((document.getElementById('cfg-clash-api-url') || {}).value || '').trim() || 'http://127.0.0.1:9097',
    apiSecret: ((document.getElementById('cfg-clash-api-secret') || {}).value || '').trim(),
    proxyGroup: ((document.getElementById('cfg-clash-proxy-group') || {}).value || '').trim(),
    testUrl: ((document.getElementById('cfg-clash-test-url') || {}).value || '').trim() || 'https://oidc.us-east-1.amazonaws.com/ping',
    testTimeout: timeout,
    skipConnectivityTest: !!((document.getElementById('cfg-clash-skip-test') || {}).checked)
  };
}

function applyClashConfigToForm(config) {
  config = config || {};
  var map = {
    'cfg-clash-api-url': config.apiUrl || 'http://127.0.0.1:9097',
    'cfg-clash-api-secret': config.apiSecret || '',
    'cfg-clash-proxy-group': config.proxyGroup || '',
    'cfg-clash-test-url': config.testUrl || 'https://oidc.us-east-1.amazonaws.com/ping',
    'cfg-clash-test-timeout': config.testTimeout || 10,
    'cfg-clash-skip-test': !!config.skipConnectivityTest
  };
  Object.keys(map).forEach(function(id) {
    var el = document.getElementById(id);
    if (!el) return;
    if (el.type === 'checkbox') el.checked = !!map[id];
    else el.value = map[id];
  });
}

async function loadClashConfig() {
  try {
    var config = await window.go.main.App.GetClashConfig();
    applyClashConfigToForm(config);
  } catch(e) {}
}

async function saveClashConfig(silent) {
  try {
    var config = getClashConfigFromForm();
    var result = await window.go.main.App.SetClashConfig(config);
    if (result.error) {
      if (!silent) showToast(result.error, 'error');
      return null;
    }
    applyClashConfigToForm(result.config || config);
    if (!silent) showToast('Clash 配置已保存');
    return result.config || config;
  } catch(e) {
    if (!silent) showToast('Clash 配置保存失败: ' + e.message, 'error');
    return null;
  }
}

async function saveClashSettings(silent) {
  try {
    await window.go.main.App.SetProxyMode('clash');
    applyProxyMode('clash');

    var proxyEl = document.getElementById('cfg-clash-proxy');
    var proxyResult = await window.go.main.App.SetClashProxy((proxyEl && proxyEl.value || '').trim());
    if (proxyResult.error) {
      if (!silent) showToast(proxyResult.error, 'error');
      return null;
    }
    if (proxyEl) proxyEl.value = proxyResult.proxy || '';

    var config = await saveClashConfig(true);
    if (!config) return null;
    if (!silent) showToast('Clash 配置已保存');
    return { proxy: proxyResult.proxy || '', config: config };
  } catch(e) {
    if (!silent) showToast('Clash 配置保存失败: ' + e.message, 'error');
    return null;
  }
}

function renderProxyDetectCard(state, payload) {
  var box = document.getElementById('proxy-detect-card');
  if (!box) return;
  if (state === 'hidden') { box.style.display = 'none'; box.innerHTML = ''; return; }
  box.style.display = 'block';
  var base = 'border:1px solid var(--border);border-radius:8px;padding:10px 12px;font-size:12px;';
  if (state === 'loading') {
    box.style.cssText = base + 'background:var(--card-bg, transparent);color:var(--muted);';
    box.innerHTML = '正在检测代理出口…' +
      (payload && payload.clash ? '<div style="margin-top:4px;font-size:11px;">正在通过 Clash API 切换并验证节点。</div>' : '') +
      (payload && payload.templated ? '<div style="margin-top:4px;font-size:11px;">检测时会临时生成 {uuid}。</div>' : '');
    return;
  }
  if (state === 'ok') {
    var loc = [payload.country, payload.region, payload.city].filter(Boolean).join(' · ');
    var okTitle = payload.clash ? '✓ Clash 节点可用' : (payload.pool ? '✓ 代理池可用' : '✓ 可用');
    var poolNote = payload.pool
      ? '<div style="margin-top:4px;color:var(--muted);font-size:11px;">第 ' + (payload.successAttempt || payload.attempts || 1) + ' 个 UUID 节点可用；注册前会重新抽样并绑定可用节点。' + (payload.durationMs ? '耗时 ' + payload.durationMs + 'ms。' : '') + '</div>'
      : '';
    var templateNote = payload.templated && !payload.pool ? '<div style="margin-top:4px;color:var(--muted);font-size:11px;">模板代理已使用临时 UUID 完成检测；注册时每次尝试会重新生成会话代理。</div>' : '';
    var clashNote = payload.clash
      ? '<div style="margin-top:4px;color:var(--muted);font-size:11px;">Clash ' + proxyEscapeHtml(payload.clashVersion || '未知版本') +
        ' · 代理组 ' + proxyEscapeHtml(payload.clashGroup || '未识别') +
        ' · 节点 ' + proxyEscapeHtml(payload.clashNode || '未知') +
        (payload.clashSkipped ? ' · 已跳过节点连通性测试' : (' · 延迟 ' + (payload.clashDelayMs || 0) + 'ms')) +
        ' · 尝试 ' + (payload.attempts || 1) + ' 个节点' +
        (payload.durationMs ? ' · 耗时 ' + payload.durationMs + 'ms' : '') + '</div>'
      : '';
    box.style.cssText = base + 'background:rgba(16,185,129,0.08);border-color:rgba(16,185,129,0.35);';
    box.innerHTML =
      '<div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;">' +
        '<span style="font-weight:600;color:#10b981;">' + okTitle + '</span>' +
        '<span style="padding:1px 6px;border-radius:4px;background:rgba(16,185,129,0.15);color:#10b981;font-size:11px;font-weight:600;">' + (payload.scheme || '').toUpperCase() + '</span>' +
        (payload.ip ? '<span style="color:var(--text);font-weight:600;">' + payload.ip + '</span>' : '') +
        (loc ? '<span style="color:var(--muted);">· ' + loc + '</span>' : '') +
      '</div>' +
      (payload.isp ? '<div style="margin-top:4px;color:var(--muted);font-size:11px;">' + payload.isp + '</div>' : '') +
      clashNote +
      poolNote +
      templateNote;
    return;
  }
  // error
  var errDetails = payload && payload.errors && payload.errors.length
    ? '<div style="margin-top:6px;font-size:11px;line-height:1.5;">' + payload.errors.map(function(x) { return '• ' + proxyEscapeHtml(x); }).join('<br>') + '</div>'
    : '';
  box.style.cssText = base + 'background:rgba(239,68,68,0.08);border-color:rgba(239,68,68,0.35);color:#ef4444;';
  var errorText = payload && payload.error ? payload.error : '未知错误';
  var staleBackendHint = /解析 Clash API 响应失败: unexpected end of JSON input/.test(errorText)
    ? '<div style="margin-top:4px;font-size:11px;">当前前端已更新，但 Go/Wails 后端仍像旧版本：请完全退出应用后重新启动或重新构建后再检测。</div>'
    : '';
  box.innerHTML = '✗ 检测失败：' + proxyEscapeHtml(errorText) +
    staleBackendHint +
    (payload && payload.clash ? '<div style="margin-top:4px;font-size:11px;">Clash 节点检测失败' +
      (payload.clashGroup ? ' · 代理组 ' + proxyEscapeHtml(payload.clashGroup) : '') +
      (payload.clashNode ? ' · 节点 ' + proxyEscapeHtml(payload.clashNode) : '') +
      (payload.attempts ? ' · 已尝试 ' + payload.attempts + ' 个节点' : '') + '。</div>' : '') +
    (payload && payload.pool ? '<div style="margin-top:4px;font-size:11px;">已连续抽样 ' + (payload.attempts || 0) + ' 个 UUID 节点，均不可用。</div>' : '') +
    (payload && payload.templated && !payload.pool ? '<div style="margin-top:4px;font-size:11px;">已尝试使用临时 UUID 检测模板代理。</div>' : '') +
    errDetails;
}

async function saveProxy() {
  var el = document.getElementById('cfg-proxy');
  if (!el) return;
  try {
    await window.go.main.App.SetProxyMode('normal');
    applyProxyMode('normal');
    if (el.value.trim()) renderProxyDetectCard('loading', { templated: el.value.indexOf('{uuid}') >= 0 });
    else renderProxyDetectCard('hidden');
    var result = await window.go.main.App.SetProxy(el.value.trim());
    if (result.error) {
      showToast(result.error, 'error');
      renderProxyDetectCard('hidden');
      return;
    }
    el.value = result.proxy || '';
    if (!result.proxy) {
      renderProxyDetectCard('hidden');
      showToast('已清空代理（直连）');
      return;
    }
    showToast('代理已保存');
    var d = result.detect;
    if (d && d.ok) renderProxyDetectCard('ok', d);
    else renderProxyDetectCard('error', d || {});
  } catch(e) {
    showToast('保存失败: ' + e.message, 'error');
    renderProxyDetectCard('error', { error: e.message });
  }
}

async function detectClashProxy() {
  var el = document.getElementById('cfg-clash-proxy');
  if (!el || !el.value.trim()) {
    showToast('请先填写本地 Clash 代理地址', 'error');
    return;
  }
  try {
    var settings = await saveClashSettings(true);
    if (!settings) return;
    renderProxyDetectCard('loading', { clash: true });
    var d = await window.go.main.App.DetectClashProxy(settings.proxy, settings.config);
    if (d && d.ok) renderProxyDetectCard('ok', d);
    else renderProxyDetectCard('error', d || {});
  } catch(e) {
    showToast('Clash 检测失败: ' + e.message, 'error');
    renderProxyDetectCard('error', { clash: true, error: e.message });
  }
}

async function resetProxy() {
  try {
    await window.go.main.App.ResetProxy();
    var el = document.getElementById('cfg-proxy');
    if (el) el.value = '';
    renderProxyDetectCard('hidden');
    showToast('已清空代理（直连）');
  } catch(e) {
    showToast('清空失败: ' + e.message, 'error');
  }
}

// 熔断级错误自动停止开关
async function loadKillSwitchEnabled() {
  var el = document.getElementById('cfg-kill-switch');
  if (!el) return;
  try {
    var enabled = await window.go.main.App.GetKillSwitchEnabled();
    el.checked = enabled !== false;
  } catch(e) {
    el.checked = true;
  }
}

async function saveKillSwitchEnabled() {
  var el = document.getElementById('cfg-kill-switch');
  if (!el) return;
  try {
    var result = await window.go.main.App.SetKillSwitchEnabled(!!el.checked);
    if (result.error) {
      showToast(result.error, 'error');
      await loadKillSwitchEnabled();
      return;
    }
    showToast(el.checked ? '已开启熔断级错误自动停止' : '已关闭熔断级错误自动停止');
  } catch(e) {
    showToast('保存失败: ' + e.message, 'error');
    await loadKillSwitchEnabled();
  }
}

// UI 状态
function updateUIStatus(running) {
  var btnStart = document.getElementById('btn-start');
  var btnStop = document.getElementById('btn-stop');
  if (btnStart) btnStart.disabled = running;
  if (btnStop) btnStop.disabled = !running;
}

// 配置读写
function getFormConfig() {
  const config = {
    count: parseInt(document.getElementById('cfg-count').value) || 1,
    concurrency: parseInt(document.getElementById('cfg-concurrency').value) || 1,
    delay: parseInt(document.getElementById('cfg-delay').value) || 3,
    emailProvider: selectedEmailProvider || 'outlook'
  };

  // 如果选择了 MoeMail，添加域名信息和前缀配置
  if (config.emailProvider === 'moemail') {
    if (!selectedMoeMailDomains || selectedMoeMailDomains.length === 0) {
      throw new Error('请选择至少一个域名或选择随机/全部');
    }

    // 如果选择了随机或全部，传递所有可用域名和配置
    if (selectedMoeMailDomains.includes('__random__') || selectedMoeMailDomains.includes('__all__')) {
      config.moemailDomains = allMoeMailDomains.map(item => item.domain);
      config.moemailConfigs = {};
      allMoeMailDomains.forEach(item => {
        config.moemailConfigs[item.domain] = item.configs;
      });
      // 标记是否为随机模式
      config.moemailRandomMode = selectedMoeMailDomains.includes('__random__');
    } else {
      // 传递选中的域名和对应的配置
      config.moemailDomains = selectedMoeMailDomains;
      config.moemailConfigs = {};
      selectedMoeMailDomains.forEach(domain => {
        const item = allMoeMailDomains.find(d => d.domain === domain);
        if (item) {
          config.moemailConfigs[domain] = item.configs;
        }
      });
      config.moemailRandomMode = false;
    }
  }

  return config;
}

function saveConfig() {
  try {
    var cfg = getFormConfig();
    cfg.outlookData = document.getElementById('cfg-outlook-data').value;
    localStorage.setItem('kiro-config', JSON.stringify(cfg));
  } catch(e) {
    console.error('配置保存失败:', e);
  }
}



// 自动保存
['cfg-count', 'cfg-concurrency', 'cfg-delay'].forEach(function(id) {
  var el = document.getElementById(id);
  if (el) {
    el.addEventListener('change', saveConfig);
    el.addEventListener('input', saveConfig);
  }
});

// 提示音开关
(function() {
  var cb = document.getElementById('cfg-sound');
  if (cb) {
    var saved = localStorage.getItem('kiro-sound');
    cb.checked = saved !== 'false';
    cb.addEventListener('change', function() {
      localStorage.setItem('kiro-sound', cb.checked);
    });
  }
})();

// 初始化加载
async function loadConfig() {
  console.log('[启动] 开始初始化...');
  
  // 默认禁用所有功能，等待卡密验证
  
  let retries = 0;
  while ((!window.go || !window.go.main || !window.go.main.App) && retries < 100) {
    await new Promise(resolve => setTimeout(resolve, 50));
    retries++;
  }
  if (!window.go || !window.go.main || !window.go.main.App) {
    console.error('[启动] Wails runtime 加载失败');
    // 即使失败也显示界面
    document.getElementById('main-container').style.display = 'block';
    return;
  }
  console.log('[启动] Wails runtime 已就绪');

  // 检测平台，macOS 使用原生窗口控件
  try {
    const env = await window.runtime.Environment();
    if (env && env.platform === 'darwin') {
      document.body.classList.add('platform-darwin');
    }
  } catch(e) {}

  // 直接显示主界面
  console.log('[启动] 显示主界面');
  const mainContainer = document.getElementById('main-container');
  if (mainContainer) {
    mainContainer.style.display = 'block';
    mainContainer.style.height = '100vh';
    mainContainer.style.width = '100vw';
    mainContainer.style.position = 'fixed';
    mainContainer.style.top = '0';
    mainContainer.style.left = '0';
    mainContainer.style.zIndex = '1';
    
    // 隐藏骨架屏
    const skeleton = document.getElementById('skeleton-loader');
    if (skeleton) {
      skeleton.style.display = 'none';
    }
    
    console.log('[启动] main-container 已显示');
  } else {
    console.error('[启动] 找不到 main-container 元素');
  }
  
  try {
    var savedConfig = localStorage.getItem('kiro-config');
    if (savedConfig) {
      var cfg = JSON.parse(savedConfig);
      document.getElementById('cfg-count').value = cfg.count || 1;
      document.getElementById('cfg-concurrency').value = cfg.concurrency || 1;
      document.getElementById('cfg-delay').value = cfg.delay || 3;
    }
  } catch(e) {
    console.error('[启动] 加载配置失败:', e);
  }
  loadOutlookAccountsList();
  loadDataDir();
  loadResultOutputDir();
  loadPageStayConfig();
  loadOutlookScope();
  loadProxyMode();
  loadProxy();
  loadClashProxy();
  loadClashConfig();
  loadKillSwitchEnabled();
  startOverviewTimer();
  console.log('[启动] 初始化完成');
}

// 页面加载时自动初始化
window.addEventListener('DOMContentLoaded', async function() {
  await loadConfig();
  initEmailProviderSelection();
  bindSettingsAutosave();
  // 启动时静默检查更新
  setTimeout(checkUpdateOnStartup, 2000);
});

async function checkUpdateOnStartup() {
  try {
    var result = await window.go.main.App.CheckUpdate();
    if (result && result.hasUpdate) {
      if (typeof showUpdateModal === 'function') showUpdateModal(result);
    }
  } catch(e) {}
}

// 侧边栏信息按钮：10次点击触发调试弹窗
var _infoClickCount = 0;
var _infoClickTimer = null;
function onNavInfoClick() {
  switchPage('info');
  _infoClickCount++;
  clearTimeout(_infoClickTimer);
  _infoClickTimer = setTimeout(function() { _infoClickCount = 0; }, 2000);
  if (_infoClickCount >= 10) {
    _infoClickCount = 0;
    if (typeof showUpdateModal === 'function') {
      showUpdateModal({
        currentVersion: 'v1.0.1',
        latestVersion: 'v99.0.0',
        releaseDate: new Date().toISOString().slice(0, 10),
        changelog: '## 调试模式\n- 这是一条测试更新弹窗\n- 触发方式：点击信息按钮 10 次',
        hasUpdate: true
      });
    }
  }
}

function renderChangelog(md) {
  var esc = function(s) {
    return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  };
  var inline = function(s) {
    return esc(s)
      .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      .replace(/`(.+?)`/g, '<code style="background:var(--bg-subtle);padding:1px 5px;border-radius:4px;font-family:var(--font-mono);font-size:12px;">$1</code>');
  };

  var lines = md.split('\n');
  var html = '';
  var inList = false;

  for (var i = 0; i < lines.length; i++) {
    var line = lines[i];
    var h2 = line.match(/^##\s+(.+)/);
    var h3 = line.match(/^###\s+(.+)/);
    var li = line.match(/^[-*]\s+(.+)/);
    var blank = line.trim() === '';

    if (h2) {
      if (inList) { html += '</ul>'; inList = false; }
      html += '<div class="cl-h2">' + inline(h2[1]) + '</div>';
    } else if (h3) {
      if (inList) { html += '</ul>'; inList = false; }
      html += '<div class="cl-h3">' + inline(h3[1]) + '</div>';
    } else if (li) {
      if (!inList) { html += '<ul class="cl-list">'; inList = true; }
      html += '<li>' + inline(li[1]) + '</li>';
    } else if (blank) {
      if (inList) { html += '</ul>'; inList = false; }
    } else {
      if (inList) { html += '</ul>'; inList = false; }
      html += '<p class="cl-p">' + inline(line) + '</p>';
    }
  }
  if (inList) html += '</ul>';
  return html;
}
