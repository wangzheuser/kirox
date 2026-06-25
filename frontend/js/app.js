// ===== 核心：导航 / 标签页 / 下拉框 / 配置 / 卡密 / Toast / 窗口控制 =====

// 页面切换
var _currentPageId = 'overview';
function getPageTitle(pageId) {
  if (window.I18N && pageId) {
    var v = window.I18N.t('page.' + pageId);
    if (v && v !== 'page.' + pageId) return v;
  }
  var fallback = { overview: '概览', logs: '运行日志', register: '注册', accounts: '邮箱池', subscription: '获取订阅支付链接', 'account-pool': '账号池', info: '关于', settings: '设置' };
  return fallback[pageId] || pageId;
}
function switchPage(pageId) {
  _currentPageId = pageId;
  document.querySelectorAll('.page, .page-placeholder, .page-iframe').forEach(function(p) {
    p.classList.remove('active');
  });
  var target = document.getElementById('page-' + pageId);
  if (target) target.classList.add('active');
  document.querySelectorAll('.nav-item[data-page]').forEach(function(item) {
    item.classList.toggle('active', item.getAttribute('data-page') === pageId);
  });
  document.getElementById('titlebar-text').textContent = getPageTitle(pageId);
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
  if (pageId === 'account-pool') {
    loadAccountPool();
  }
  if (pageId === 'register' && typeof loadEmailProviderStats === 'function') {
    loadEmailProviderStats();
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
  if (changelogEl) changelogEl.innerHTML = '<span style="color:var(--text-muted);">' + tr('common.loading', '加载中...') + '</span>';
  try {
    var result = await window.go.main.App.CheckUpdate();
    if (result.error) {
      if (changelogEl) changelogEl.innerHTML = '<span style="color:var(--text-muted);">' + tr('common.loadFailed', '加载失败') + ': ' + result.error + '</span>';
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
      changelogEl.innerHTML = body ? renderChangelog(body) : '<span style="color:var(--text-muted);">' + tr('common.noData', '暂无更新说明') + '</span>';
    }
  } catch(e) {
    if (changelogEl) changelogEl.innerHTML = '<span style="color:var(--text-muted);">' + tr('common.loadFailed', '加载失败') + '</span>';
  }
}

// 翻译辅助：t() 返回 key 自身时回落到 fallback
function tr(key, fallback) {
  if (window.I18N && typeof window.I18N.t === 'function') {
    var v = window.I18N.t(key);
    if (v && v !== key) return v;
  }
  return fallback != null ? fallback : key;
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
    showToast(tr('toast.dataDirSet', '存储目录已设置'));
  } catch(e) {
    showToast(tr('toast.operationFailed', '操作失败') + ': ' + e.message, 'error');
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
    showToast(tr('toast.dataDirReset', '已重置为默认存储目录'));
  } catch(e) {
    showToast(tr('toast.operationFailed', '操作失败') + ': ' + e.message, 'error');
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
    showToast(tr('toast.outputDirSet', '输出目录已设置') + ': ' + result.path);
  } catch(e) {
    showToast(tr('toast.operationFailed', '操作失败') + ': ' + e.message, 'error');
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
    showToast(tr('toast.outputDirReset', '已重置为默认输出目录'));
  } catch(e) {
    showToast(tr('toast.operationFailed', '操作失败') + ': ' + e.message, 'error');
  }
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

async function loadOutlookGraphRegistrationEmailMode() {
  try {
    var mode = await window.go.main.App.GetOutlookGraphRegistrationEmailMode();
    var el = document.getElementById('cfg-outlook-graph-registration-email-mode');
    if (el) el.value = mode || 'auto';
  } catch(e) {}
}

async function saveOutlookGraphRegistrationEmailMode() {
  try {
    var el = document.getElementById('cfg-outlook-graph-registration-email-mode');
    var mode = (el && el.value) || 'auto';
    var result = await window.go.main.App.SetOutlookGraphRegistrationEmailMode(mode);
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }
    if (el) el.value = result.mode || mode;
    var label = { auto: '自动识别别名', imported: '始终使用导入邮箱', primary: '始终使用 Graph 主邮箱' }[result.mode || mode] || '自动识别别名';
    showToast('Graph 注册邮箱策略已保存：' + label);
  } catch(e) {
    showToast('保存失败: ' + e.message, 'error');
  }
}

function bindSettingsAutosave() {

  var outlookScopeEl = document.getElementById('cfg-outlook-scope');
  if (outlookScopeEl) {
    outlookScopeEl.addEventListener('change', saveOutlookScope);
  }

  var outlookGraphModeEl = document.getElementById('cfg-outlook-graph-registration-email-mode');
  if (outlookGraphModeEl) {
    outlookGraphModeEl.addEventListener('change', saveOutlookGraphRegistrationEmailMode);
  }

  // kiro.rs 同步：文本框防抖静默保存，开关即时保存
  ['cfg-kiro-rs-url', 'cfg-kiro-rs-key'].forEach(function(id) {
    var el = document.getElementById(id);
    if (!el) return;
    el.addEventListener('input', scheduleKiroRSConfigSave);
    el.addEventListener('change', scheduleKiroRSConfigSave);
  });
  var kiroAutoEl = document.getElementById('cfg-kiro-rs-auto-sync');
  if (kiroAutoEl) {
    kiroAutoEl.addEventListener('change', function() { saveKiroRSConfig(true); });
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

async function loadEmailProxy() {
  try {
    var p = await window.go.main.App.GetEmailProxy();
    var el = document.getElementById('cfg-email-proxy');
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
  var poolPanel = document.getElementById('proxy-pool-panel');
  var clashPanel = document.getElementById('proxy-clash-panel');
  if (normalPanel) normalPanel.style.display = mode === 'normal' ? 'flex' : 'none';
  if (poolPanel) poolPanel.style.display = mode === 'pool' ? 'flex' : 'none';
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
    showToast('代理模式已切换为' + ({ none: '直连', normal: '普通代理', pool: '多代理池', clash: 'Clash 代理' }[result.mode || mode] || mode));
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
      '<div style="margin-top:4px;font-size:11px;">最多等待约 8 秒，失败后会显示具体原因。</div>' +
      (payload && payload.clash ? '<div style="margin-top:4px;font-size:11px;">正在通过 Clash API 切换并验证节点。</div>' : '') +
      (payload && payload.templated ? '<div style="margin-top:4px;font-size:11px;">检测时会将 {uuid} 替换为 32 位无短横线会话 ID。</div>' : '');
    return;
  }
  if (state === 'ok') {
    var loc = [payload.country, payload.region, payload.city].filter(Boolean).join(' · ');
    var okTitle = payload.clash ? '✓ Clash 节点可用' : (payload.pool ? '✓ 代理池可用' : '✓ 可用');
    var poolNote = payload.pool
      ? '<div style="margin-top:4px;color:var(--muted);font-size:11px;">第 ' + (payload.successAttempt || payload.attempts || 1) + ' 个 UUID 节点可用；注册前会重新抽样并绑定可用节点。' + (payload.durationMs ? '耗时 ' + payload.durationMs + 'ms。' : '') + '</div>'
      : '';
    var templateNote = payload.templated && !payload.pool ? '<div style="margin-top:4px;color:var(--muted);font-size:11px;">模板代理已使用 32 位无短横线会话 ID 完成检测；注册时每次尝试会重新生成会话代理。</div>' : '';
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

function renderEmailProxyDetectCard(state, payload) {
  var box = document.getElementById('email-proxy-detect-card');
  if (!box) return;
  if (state === 'hidden') { box.style.display = 'none'; box.innerHTML = ''; return; }
  box.style.display = 'block';
  var base = 'border:1px solid var(--border);border-radius:8px;padding:10px 12px;font-size:12px;';
  if (state === 'loading') {
    box.style.cssText = base + 'background:var(--card-bg, transparent);color:var(--muted);';
    box.innerHTML = '正在检测邮箱代理出口…' +
      (payload && payload.templated ? '<div style="margin-top:4px;font-size:11px;">检测时会将 {uuid} 替换为 32 位无短横线会话 ID。</div>' : '');
    return;
  }
  if (state === 'ok') {
    var loc = [payload.country, payload.region, payload.city].filter(Boolean).join(' · ');
    box.style.cssText = base + 'background:rgba(16,185,129,0.08);border-color:rgba(16,185,129,0.35);';
    box.innerHTML =
      '<div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;">' +
        '<span style="font-weight:600;color:#10b981;">✓ 邮箱代理可用</span>' +
        '<span style="padding:1px 6px;border-radius:4px;background:rgba(16,185,129,0.15);color:#10b981;font-size:11px;font-weight:600;">' + proxyEscapeHtml((payload.scheme || '').toUpperCase()) + '</span>' +
        (payload.ip ? '<span style="color:var(--text);font-weight:600;">' + proxyEscapeHtml(payload.ip) + '</span>' : '') +
        (loc ? '<span style="color:var(--muted);">· ' + proxyEscapeHtml(loc) + '</span>' : '') +
      '</div>' +
      (payload.isp ? '<div style="margin-top:4px;color:var(--muted);font-size:11px;">' + proxyEscapeHtml(payload.isp) + '</div>' : '') +
      (payload.pool ? '<div style="margin-top:4px;color:var(--muted);font-size:11px;">第 ' + (payload.successAttempt || payload.attempts || 1) + ' 个 UUID 节点可用；邮箱 API 调用时会重新生成会话代理。</div>' : '');
    return;
  }
  var errDetails = payload && payload.errors && payload.errors.length
    ? '<div style="margin-top:6px;font-size:11px;line-height:1.5;">' + payload.errors.map(function(x) { return '• ' + proxyEscapeHtml(x); }).join('<br>') + '</div>'
    : '';
  box.style.cssText = base + 'background:rgba(239,68,68,0.08);border-color:rgba(239,68,68,0.35);color:#ef4444;';
  box.innerHTML = '✗ 邮箱代理检测失败：' + proxyEscapeHtml(payload && payload.error ? payload.error : '未知错误') +
    (payload && payload.pool ? '<div style="margin-top:4px;font-size:11px;">已连续抽样 ' + (payload.attempts || 0) + ' 个 UUID 节点，均不可用。</div>' : '') +
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
      showToast(tr('toast.proxyCleared', '代理已清除'));
      return;
    }
    showToast(tr('toast.proxySaved', '代理已保存'));
    var d = result.detect;
    if (d && d.ok) renderProxyDetectCard('ok', d);
    else renderProxyDetectCard('error', d || {});
  } catch(e) {
    showToast(tr('toast.operationFailed', '操作失败') + ': ' + e.message, 'error');
    renderProxyDetectCard('error', { error: e.message });
  }
}

async function saveEmailProxy() {
  var el = document.getElementById('cfg-email-proxy');
  if (!el) return;
  try {
    if (el.value.trim()) renderEmailProxyDetectCard('loading', { templated: el.value.indexOf('{uuid}') >= 0 });
    else renderEmailProxyDetectCard('hidden');
    var result = await window.go.main.App.SetEmailProxy(el.value.trim());
    if (result.error) {
      showToast(result.error, 'error');
      renderEmailProxyDetectCard('hidden');
      return;
    }
    el.value = result.proxy || '';
    if (!result.proxy) {
      renderEmailProxyDetectCard('hidden');
      showToast('已清空邮箱代理（直连）');
      return;
    }
    showToast('邮箱代理已保存');
    var d = result.detect;
    if (d && d.ok) renderEmailProxyDetectCard('ok', d);
    else renderEmailProxyDetectCard('error', d || {});
  } catch(e) {
    showToast('邮箱代理保存失败: ' + e.message, 'error');
    renderEmailProxyDetectCard('error', { error: e.message });
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

async function resetEmailProxy() {
  try {
    await window.go.main.App.ResetEmailProxy();
    var el = document.getElementById('cfg-email-proxy');
    if (el) el.value = '';
    renderEmailProxyDetectCard('hidden');
    showToast('已清空邮箱代理（直连）');
  } catch(e) {
    showToast('清空邮箱代理失败: ' + e.message, 'error');
  }
}

async function resetProxy() {
  try {
    await window.go.main.App.ResetProxy();
    var el = document.getElementById('cfg-proxy');
    if (el) el.value = '';
    renderProxyDetectCard('hidden');
    showToast(tr('toast.proxyCleared', '代理已清除'));
  } catch(e) {
    showToast(tr('toast.operationFailed', '操作失败') + ': ' + e.message, 'error');
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

// ===== kiro.rs 同步配置 =====

var kiroRSSaveTimer = null;
var lastSavedKiroRSPayload = null;

async function loadKiroRSConfig() {
  try {
    var cfg = await window.go.main.App.GetKiroRSConfig();
    var urlEl = document.getElementById('cfg-kiro-rs-url');
    var keyEl = document.getElementById('cfg-kiro-rs-key');
    var autoEl = document.getElementById('cfg-kiro-rs-auto-sync');
    if (urlEl) urlEl.value = cfg.apiURL || '';
    if (keyEl) keyEl.value = cfg.apiKey || '';
    if (autoEl) autoEl.checked = !!cfg.autoSync;
    // 记录已保存快照，避免初始化或无变化时重复写盘
    lastSavedKiroRSPayload = JSON.stringify({
      url: (cfg.apiURL || '').trim(),
      key: (cfg.apiKey || '').trim(),
      autoSync: !!cfg.autoSync
    });
  } catch(e) {}
}

// 防抖触发文本框（API 地址 / Key）的静默自动保存
function scheduleKiroRSConfigSave() {
  if (kiroRSSaveTimer) {
    clearTimeout(kiroRSSaveTimer);
  }
  kiroRSSaveTimer = setTimeout(function() {
    kiroRSSaveTimer = null;
    saveKiroRSConfig(false);
  }, 450);
}

// saveKiroRSConfig 保存 kiro.rs 配置。
// withToast=true 时给出明确反馈（用于开关切换）；文本框自动保存则静默。
async function saveKiroRSConfig(withToast) {
  var url = ((document.getElementById('cfg-kiro-rs-url') || {}).value || '').trim();
  var key = ((document.getElementById('cfg-kiro-rs-key') || {}).value || '').trim();
  var autoSync = !!(document.getElementById('cfg-kiro-rs-auto-sync') || {}).checked;
  var payloadKey = JSON.stringify({ url: url, key: key, autoSync: autoSync });
  if (payloadKey === lastSavedKiroRSPayload) {
    return; // 无变化，跳过
  }
  try {
    var result = await window.go.main.App.SetKiroRSConfig(url, key, autoSync);
    if (result.error) {
      showToast(result.error, 'error');
      await loadKiroRSConfig();
      return;
    }
    lastSavedKiroRSPayload = payloadKey;
    if (withToast) {
      showToast(autoSync ? '已开启注册后自动同步' : '已关闭注册后自动同步');
    }
  } catch(e) {
    showToast('保存失败: ' + e.message, 'error');
    await loadKiroRSConfig();
  }
}

async function testKiroRSConnection() {
  var url = (document.getElementById('cfg-kiro-rs-url') || {}).value || '';
  var key = (document.getElementById('cfg-kiro-rs-key') || {}).value || '';
  try {
    var result = await window.go.main.App.TestKiroRSConnection(url.trim(), key.trim());
    if (result.success) {
      showToast('连接成功', 'success');
    } else {
      showToast(result.error || '连接失败', 'error');
    }
  } catch(e) {
    showToast('测试失败: ' + e.message, 'error');
  }
}

// UI 状态
var taskActionState = 'idle';

function setButtonDisabled(id, disabled) {
  var el = document.getElementById(id);
  if (el) el.disabled = !!disabled;
}

function setTaskActionState(state) {
  taskActionState = state || 'idle';
  var starting = taskActionState === 'starting';
  var running = taskActionState === 'running';
  var stopping = taskActionState === 'stopping';
  var busy = starting || running || stopping;

  setButtonDisabled('ov-btn-start', busy);
  setButtonDisabled('reg-btn-start', busy);
  setButtonDisabled('ov-btn-stop', !running);
  setButtonDisabled('reg-btn-stop', !running);
}

function updateUIStatus(running) {
  if (running) {
    if (taskActionState !== 'stopping') setTaskActionState('running');
    return;
  }
  if (taskActionState !== 'starting') setTaskActionState('idle');
}

// 配置读写
var registrationConfigLoaded = false;
var registrationConfigApplying = false;
var registrationSaveTimer = null;

function readIntegerInput(id, fallback, min) {
  var el = document.getElementById(id);
  var raw = el ? String(el.value).trim() : '';
  var value = raw === '' ? fallback : parseInt(raw, 10);
  if (!Number.isFinite(value)) value = fallback;
  if (typeof min === 'number' && value < min) value = min;
  return value;
}

function writeIntegerInput(id, value, fallback) {
  var el = document.getElementById(id);
  if (!el) return;
  var n = Number(value);
  el.value = Number.isFinite(n) ? String(n) : String(fallback);
}

function readCheckboxInput(id) {
  var el = document.getElementById(id);
  return !!(el && el.checked);
}

function writeCheckboxInput(id, value) {
  var el = document.getElementById(id);
  if (!el) return;
  el.checked = !!value;
}

function getMoeMailSelectionForPersistence() {
  var selected = Array.isArray(selectedMoeMailDomains) ? selectedMoeMailDomains.slice() : [];
  if (selected.includes('__all__')) {
    return { moemailDomainMode: 'all', moemailDomains: [] };
  }
  if (selected.includes('__random__') || selected.length === 0) {
    return { moemailDomainMode: 'random', moemailDomains: [] };
  }
  return { moemailDomainMode: 'custom', moemailDomains: selected };
}

function getRegistrationConfigPayload() {
  var selection = getMoeMailSelectionForPersistence();
  return {
    count: readIntegerInput('cfg-count', 1, 1),
    successTarget: readIntegerInput('cfg-success-target', 0, 0),
    concurrency: readIntegerInput('cfg-concurrency', 1, 1),
    delay: readIntegerInput('cfg-delay', 1, 0),
    retryCount: readIntegerInput('cfg-retry-count', 1, 0),
    otpTimeout: readIntegerInput('cfg-otp-timeout', 60, 30),
    reuseFailedEmail: readCheckboxInput('cfg-reuse-failed-email'),
    emailProviders: getSelectedEmailProviders(),
    moemailDomainMode: selection.moemailDomainMode,
    moemailDomains: selection.moemailDomains
  };
}

function moeMailSelectionFromConfig(cfg) {
  cfg = cfg || {};
  if (cfg.moemailDomainMode === 'all') return ['__all__'];
  if (cfg.moemailDomainMode === 'custom' && Array.isArray(cfg.moemailDomains) && cfg.moemailDomains.length) {
    return cfg.moemailDomains.slice();
  }
  return ['__random__'];
}

function applyRegistrationConfig(cfg) {
  cfg = cfg || {};
  registrationConfigApplying = true;
  writeIntegerInput('cfg-count', cfg.count, 1);
  writeIntegerInput('cfg-success-target', cfg.successTarget, 0);
  writeIntegerInput('cfg-concurrency', cfg.concurrency, 1);
  writeIntegerInput('cfg-delay', cfg.delay, 1);
  writeIntegerInput('cfg-retry-count', cfg.retryCount, 1);
  writeIntegerInput('cfg-otp-timeout', cfg.otpTimeout, 60);
  writeCheckboxInput('cfg-reuse-failed-email', cfg.reuseFailedEmail);
  selectedMoeMailDomains = moeMailSelectionFromConfig(cfg);
  if (typeof setEmailProviders === 'function') {
    setEmailProviders(cfg.emailProviders || ['outlook'], {
      skipSave: true,
      moemailSelection: selectedMoeMailDomains
    });
  }
  registrationConfigApplying = false;
  registrationConfigLoaded = true;
}

function getFormConfig() {
  const config = {
    count: readIntegerInput('cfg-count', 1, 1),
    successTarget: readIntegerInput('cfg-success-target', 0, 0),
    concurrency: readIntegerInput('cfg-concurrency', 1, 1),
    delay: readIntegerInput('cfg-delay', 1, 0),
    retryCount: readIntegerInput('cfg-retry-count', 1, 0),
    otpTimeout: readIntegerInput('cfg-otp-timeout', 60, 30),
    reuseFailedEmail: readCheckboxInput('cfg-reuse-failed-email'),
    emailProviders: getSelectedEmailProviders()
  };
  if (!config.emailProviders.length) {
    throw new Error('请至少选择一个邮箱渠道');
  }

  // 如果选择了 MoeMail，添加域名信息和前缀配置
  if (config.emailProviders.includes('moemail')) {
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

  // 如果选择了 Cloud-Mail，添加域名信息和配置
  if (config.emailProviders.includes('cloudmail')) {
    if (!selectedCloudMailDomains || selectedCloudMailDomains.length === 0) {
      throw new Error('请选择至少一个 Cloud-Mail 域名');
    }

    if (selectedCloudMailDomains.includes('__random__') || selectedCloudMailDomains.includes('__all__')) {
      config.cloudmailDomains = allCloudMailDomains.map(item => item.domain);
      config.cloudmailConfigs = {};
      allCloudMailDomains.forEach(item => {
        config.cloudmailConfigs[item.domain] = item.configs;
      });
      config.cloudmailRandomMode = selectedCloudMailDomains.includes('__random__');
    } else {
      config.cloudmailDomains = selectedCloudMailDomains;
      config.cloudmailConfigs = {};
      selectedCloudMailDomains.forEach(domain => {
        const item = allCloudMailDomains.find(d => d.domain === domain);
        if (item) {
          config.cloudmailConfigs[domain] = item.configs;
        }
      });
      config.cloudmailRandomMode = false;
    }
  }

  return config;
}

async function saveConfig(options) {
  options = options || {};
  if (registrationConfigApplying) return;
  try {
    var result = await window.go.main.App.SetRegistrationConfig(getRegistrationConfigPayload());
    if (result && result.error) {
      if (!options.silent) showToast(result.error, 'error');
      return;
    }
    if (!options.silent && result && result.config) {
      applyRegistrationConfig(result.config);
      showToast('注册配置已保存');
    }
  } catch(e) {
    console.error('配置保存失败:', e);
    if (!options.silent) showToast('注册配置保存失败: ' + e.message, 'error');
  }
}

function scheduleRegistrationConfigSave() {
  if (!registrationConfigLoaded || registrationConfigApplying) return;
  if (registrationSaveTimer) clearTimeout(registrationSaveTimer);
  registrationSaveTimer = setTimeout(function() {
    registrationSaveTimer = null;
    saveConfig({ silent: true });
  }, 300);
}

async function loadRegistrationConfig() {
  try {
    var cfg = await window.go.main.App.GetRegistrationConfig();
    applyRegistrationConfig(cfg);
  } catch(e) {
    console.error('[启动] 加载注册配置失败:', e);
    applyRegistrationConfig({
      count: 1,
      successTarget: 0,
      concurrency: 1,
      delay: 1,
      retryCount: 1,
      otpTimeout: 60,
      reuseFailedEmail: false,
      emailProviders: ['outlook'],
      moemailDomainMode: 'random',
      moemailDomains: []
    });
  }
}

function getEmailProviderDisplayName(provider) {
  var map = {
    outlook: '微软邮箱',
    moemail: 'MoeMail',
    cloudmail: 'Cloud-Mail',
    mailporary: 'Mailporary',
    emailnator: 'Emailnator',
    mailgw: 'mail.gw',
    mailtm: 'mail.tm',
    tempmail_lol: 'TempMail.lol',
    guerrillamail: 'GuerrillaMail',
    mailtemp: 'MailTemp',
    tempmail_plus: 'TempMail.plus',
    inboxkitten: 'InboxKitten',
    inboxes: 'Inboxes',
    freecustom: 'FreeCustom.Email',
    dropmail: 'DropMail',
    mailcatch: 'MailCatch',
    tempmailo: 'TempMailo',
    minuteinbox: 'MinuteInbox',
    smailpro: 'SmailPro',
    tempmailbox: 'TempMailbox',
    generator_email: 'Generator.Email',
    mailtowin: 'MailToWin',
    mail2me: 'Mail2Me',
    pickmemail: 'PickMeMail',
    maximail: 'MaxiMail',
    emlpro: 'EmlPro',
    freeml: 'FreeML',
    emlhub: 'EmlHub',
    emltmp: 'EmlTmp',
    mailpwr: 'MailPwr',
    tenmail: '10Mail',
    dropmail_me: 'DropMail.me',
    mimimail: 'MimiMail',
    pickmail: 'PickMail',
    spymail: 'SpyMail',
    yomail: 'YoMail',
    tmio_bltiwd: 'TempMailIO bltiwd',
    tmio_wnbaldwy: 'TempMailIO wnbaldwy',
    tmio_bwmyga: 'TempMailIO bwmyga',
    tmio_ozsaip: 'TempMailIO ozsaip'
  };
  return map[provider] || provider || '-';
}

function formatEmailProviderSuccessDomains(domains) {
  domains = domains || {};
  var items = Object.keys(domains).map(function(domain) {
    return { domain: domain, count: parseInt(domains[domain], 10) || 0 };
  }).filter(function(item) {
    return item.domain && item.count > 0;
  });
  items.sort(function(a, b) {
    if (a.count === b.count) return a.domain.localeCompare(b.domain);
    return b.count - a.count;
  });
  if (!items.length) return '<span style="color:var(--text-muted);">-</span>';
  return items.map(function(item) {
    return '<span style="display:inline-block;margin:2px 4px 2px 0;padding:2px 6px;border-radius:999px;background:var(--bg-subtle);border:1px solid var(--border);font-family:var(--font-mono);">' +
      _escapeHtml(item.domain) + '(' + item.count + ')' +
      '</span>';
  }).join('');
}

function _escapeHtml(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

async function loadEmailProviderStats() {
  var body = document.getElementById('email-provider-stats-body');
  if (!body || !window.go || !window.go.main || !window.go.main.App) return;
  try {
    var stats = await window.go.main.App.GetEmailProviderStats();
    stats = Array.isArray(stats) ? stats : [];
    if (!stats.length) {
      body.innerHTML = '<tr><td colspan="4" style="text-align:center;color:var(--text-muted);padding:12px;">暂无统计</td></tr>';
      return;
    }
    body.innerHTML = stats.map(function(stat) {
      var provider = stat.provider || stat.Provider || '';
      var otpCount = stat.otpReceivedCount != null ? stat.otpReceivedCount : stat.OTPReceivedCount;
      var successCount = stat.registrationSuccessCount != null ? stat.registrationSuccessCount : stat.RegistrationSuccessCount;
      var domains = stat.successDomains || stat.SuccessDomains || {};
      return '<tr>' +
        '<td style="white-space:nowrap;font-weight:700;">' + _escapeHtml(getEmailProviderDisplayName(provider)) + '</td>' +
        '<td style="text-align:right;font-family:var(--font-mono);">' + (parseInt(otpCount, 10) || 0) + '</td>' +
        '<td style="text-align:right;font-family:var(--font-mono);">' + (parseInt(successCount, 10) || 0) + '</td>' +
        '<td style="min-width:220px;">' + formatEmailProviderSuccessDomains(domains) + '</td>' +
        '</tr>';
    }).join('');
  } catch(e) {
    body.innerHTML = '<tr><td colspan="4" style="text-align:center;color:var(--danger);padding:12px;">加载失败: ' + _escapeHtml(e.message || e) + '</td></tr>';
  }
}

async function selectEmailProvidersFromStats() {
  if (!window.go || !window.go.main || !window.go.main.App) return;
  try {
    var stats = await window.go.main.App.GetEmailProviderStats();
    stats = Array.isArray(stats) ? stats : [];
    var providers = stats.map(function(stat) {
      return String((stat && (stat.provider || stat.Provider)) || '').trim();
    }).filter(function(provider, index, list) {
      return provider && list.indexOf(provider) === index;
    });
    if (!providers.length) {
      showToast('暂无统计渠道可勾选', 'error');
      return;
    }
    setEmailProviders(providers);
    showToast('已按统计渠道覆盖当前邮箱渠道选择');
  } catch(e) {
    showToast('勾选统计渠道失败: ' + (e.message || e), 'error');
  }
}

async function resetEmailProviderStats() {
  if (!window.go || !window.go.main || !window.go.main.App) return;
  try {
    var result = await window.go.main.App.ResetEmailProviderStats();
    if (result && result.error) {
      showToast(result.error, 'error');
      return;
    }
    await loadEmailProviderStats();
    showToast('邮箱渠道统计已重置');
  } catch(e) {
    showToast('重置统计失败: ' + (e.message || e), 'error');
  }
}

// 自动保存
['cfg-count', 'cfg-success-target', 'cfg-concurrency', 'cfg-delay', 'cfg-retry-count', 'cfg-otp-timeout'].forEach(function(id) {
  var el = document.getElementById(id);
  if (el) {
    el.addEventListener('change', scheduleRegistrationConfigSave);
    el.addEventListener('input', scheduleRegistrationConfigSave);
  }
});
['cfg-reuse-failed-email'].forEach(function(id) {
  var el = document.getElementById(id);
  if (el) {
    el.addEventListener('change', scheduleRegistrationConfigSave);
  }
});

// 提示音开关
async function loadSoundEnabled() {
  var cb = document.getElementById('cfg-sound');
  if (!cb) return;
  if (!cb.dataset.soundBound) {
    cb.addEventListener('change', saveSoundEnabled);
    cb.dataset.soundBound = '1';
  }
  try {
    var legacy = localStorage.getItem('kiro-sound');
    if (legacy !== null) {
      var legacyEnabled = legacy !== 'false';
      var migrated = await window.go.main.App.SetSoundEnabled(legacyEnabled);
      if (!migrated || !migrated.error) {
        localStorage.removeItem('kiro-sound');
      }
      cb.checked = legacyEnabled;
      return;
    }
  } catch(e) {}
  try {
    var enabled = await window.go.main.App.GetSoundEnabled();
    cb.checked = enabled !== false;
  } catch(e) {
    cb.checked = true;
  }
}

async function saveSoundEnabled() {
  var cb = document.getElementById('cfg-sound');
  if (!cb) return;
  try {
    var result = await window.go.main.App.SetSoundEnabled(!!cb.checked);
    if (result && result.error) {
      showToast(result.error, 'error');
      await loadSoundEnabled();
      return;
    }
    showToast(cb.checked ? '已开启任务结束提示音' : '已关闭任务结束提示音');
  } catch(e) {
    showToast('提示音设置保存失败: ' + e.message, 'error');
    await loadSoundEnabled();
  }
}

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
  
  await loadRegistrationConfig();
  loadEmailProviderStats();
  loadOutlookAccountsList();
  loadDataDir();
  loadResultOutputDir();
  loadOutlookScope();
  loadOutlookGraphRegistrationEmailMode();
  loadProxyMode();
  loadProxy();
  loadEmailProxy();
  loadClashProxy();
  loadClashConfig();
  loadKillSwitchEnabled();
  loadSoundEnabled();
  loadKiroRSConfig();
  if (typeof loadProxyPool === 'function') loadProxyPool();
  startOverviewTimer();
  console.log('[启动] 初始化完成');
}

// 页面加载时自动初始化
window.addEventListener('DOMContentLoaded', async function() {
  await loadConfig();
  initEmailProviderSelection();
  bindSettingsAutosave();
  // 初始化 i18n（在 Wails runtime 就绪后），失败时不阻塞主流程
  try {
    if (window.I18N && typeof window.I18N.init === 'function') {
      await window.I18N.init();
      var sel = document.getElementById('cfg-language');
      if (sel) sel.value = window.I18N.getLanguage();
      refreshLanguageNavLabel();
      // 重新渲染依赖 i18n 的动态文本
      var tb = document.getElementById('titlebar-text');
      if (tb) tb.textContent = getPageTitle(_currentPageId);
    }
  } catch(e) {}
  // 语言切换时刷新动态文本
  window.addEventListener('i18n:changed', function() {
    var tb = document.getElementById('titlebar-text');
    if (tb) tb.textContent = getPageTitle(_currentPageId);
    refreshLanguageNavLabel();
  });
  // 启动时静默检查更新
  setTimeout(checkUpdateOnStartup, 2000);
});

// 语言切换（设置页下拉）
async function onLanguageChange(lang) {
  if (!lang || !window.I18N) return;
  try {
    window.I18N.setLanguage(lang);
    showToast(tr('toast.languageChanged', '已切换语言'));
  } catch(e) {
    showToast(tr('toast.operationFailed', '操作失败') + ': ' + e.message, 'error');
  }
}

// 语言循环切换（侧栏点击）：zh → en → ja → zh
var _langOrder = ['zh', 'en', 'ja'];
var _langLabel = { zh: '中', en: 'EN', ja: 'あ' };
var _langFlag = { zh: 'cn', en: 'us', ja: 'jp' };
function cycleLanguage() {
  if (!window.I18N) return;
  var cur = window.I18N.getLanguage();
  var idx = _langOrder.indexOf(cur);
  var next = _langOrder[(idx + 1) % _langOrder.length];
  try {
    window.I18N.setLanguage(next);
    showToast(tr('toast.languageChanged', '已切换语言'));
  } catch(e) {
    showToast(tr('toast.operationFailed', '操作失败') + ': ' + e.message, 'error');
  }
}
function refreshLanguageNavLabel() {
  var el = document.getElementById('nav-language-label');
  if (!el || !window.I18N) return;
  var cur = window.I18N.getLanguage();
  if (el.tagName === 'IMG') {
    el.src = 'https://flagcdn.com/w40/' + (_langFlag[cur] || 'cn') + '.png';
    el.alt = cur;
  } else {
    el.textContent = _langLabel[cur] || cur;
  }
}

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
