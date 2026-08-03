// ===== 任务控制 + 更新系统 + 状态轮询 =====

function _tkT(key, varsOrFallback, fallbackMaybe) {
  var vars = null, fallback = null;
  if (typeof varsOrFallback === 'string') {
    fallback = varsOrFallback;
  } else if (varsOrFallback && typeof varsOrFallback === 'object') {
    vars = varsOrFallback;
    if (typeof fallbackMaybe === 'string') fallback = fallbackMaybe;
  }
  if (window.I18N && typeof window.I18N.t === 'function') {
    var v = window.I18N.t(key, vars);
    if (v && v !== key) return v;
  }
  if (fallback != null) {
    if (vars) {
      return fallback.replace(/\{(\w+)\}/g, function(_, k) {
        return vars[k] != null ? vars[k] : '{' + k + '}';
      });
    }
    return fallback;
  }
  return key;
}

function formatTime(seconds) {
  seconds = Math.round(seconds);
  if (seconds < 60) return seconds + 's';
  var m = Math.floor(seconds / 60);
  var s = seconds % 60;
  if (m < 60) return m + 'm ' + s + 's';
  var h = Math.floor(m / 60);
  m = m % 60;
  return h + 'h ' + m + 'm';
}

function switchToOverviewAfterTaskStart() {
  if (typeof switchPage === 'function') switchPage('overview');
}

var updateInfo = null;
var _prevRunning = false;
window._kiroLogs = [];

function _escapeLogHtml(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// 将一行日志解析为带高亮 span 的 HTML。
// 识别模式: "YYYY/MM/DD HH:MM:SS [prefix] [step] rest"
function _formatLogLine(line) {
  var raw = line.replace(/\r?\n$/, '');
  if (!raw) return '';

  // 整行级别判定 —— 用原始中文判定，避免翻译后关键字缺失
  var low = raw.toLowerCase();
  var cls = 'log-line';
  if (raw.indexOf('注册成功') >= 0 || raw.indexOf('已验活') >= 0 || raw.indexOf('[OK]') >= 0) {
    cls += ' log-line-success';
  } else if (raw.indexOf('失败') >= 0 || raw.indexOf('错误') >= 0 || raw.indexOf('异常') >= 0 ||
             raw.indexOf('被拦截') >= 0 || raw.indexOf('被封') >= 0 ||
             low.indexOf('error') >= 0 || low.indexOf('failed') >= 0) {
    cls += ' log-line-error';
  } else if (raw.indexOf('⚠') >= 0 || raw.indexOf('熔断') >= 0 || raw.indexOf('重试') >= 0) {
    cls += ' log-line-warn';
  } else if (raw.indexOf('[DEBUG]') >= 0) {
    cls += ' log-line-debug';
  }

  // 翻译为当前语言（zh 直接返回原文）
  var display = raw;
  if (window.I18N && typeof window.I18N.translateLog === 'function') {
    display = window.I18N.translateLog(raw);
  }

  // 分段高亮: 时间戳 + [标签] + [step] + 其余
  var html = '';
  var rest = display;

  var m = rest.match(/^((?:\d{4}\/\d{2}\/\d{2}\s+)?\d{2}:\d{2}:\d{2})\s*/);
  if (m) {
    html += '<span class="log-time">' + _escapeLogHtml(m[1]) + '</span>';
    rest = rest.slice(m[0].length);
  }

  // 匹配若干 [xxx] 前缀，时间戳之后的所有方括号标签
  while (true) {
    var t = rest.match(/^(\[[^\]]+\])\s*/);
    if (!t) break;
    var label = t[1];
    // 纯数字步骤如 [1] [12.5] 用 step 色，其余用 tag 色
    var inner = label.slice(1, -1);
    var isStep = /^\d+(\.\d+)?(\/\d+)?$/.test(inner);
    html += '<span class="' + (isStep ? 'log-step' : 'log-tag') + '">' +
      _escapeLogHtml(label) + '</span>';
    rest = rest.slice(t[0].length);
  }

  html += _escapeLogHtml(rest);
  return '<span class="' + cls + '">' + html + '</span>';
}

function renderUnifiedLogs() {
  var box = document.getElementById('unified-log-box');
  if (!box) return;

  // 首次渲染时挂上行级点击复制（事件委托，后续 innerHTML 重写不会丢）
  if (!box.dataset.copyBound) {
    box.addEventListener('click', function(e) {
      var line = e.target.closest('.log-line');
      if (!line || !box.contains(line)) return;
      // 如果用户正在选中文字，让选择行为优先，不触发行复制
      var sel = window.getSelection && window.getSelection();
      if (sel && sel.toString().length > 0) return;
      var text = line.textContent.replace(/\u00A0/g, ' ').trim();
      if (!text) return;
      navigator.clipboard.writeText(text).then(function() {
        line.classList.add('log-copied');
        setTimeout(function() { line.classList.remove('log-copied'); }, 600);
        if (typeof showToast === 'function') showToast(_tkT('toast.copied', '复制成功'), 'success');
      }).catch(function(err) {
        if (typeof showToast === 'function') showToast(_tkT('toast.copyFailed', '复制失败') + ': ' + err.message, 'error');
      });
    });
    box.dataset.copyBound = '1';
  }

  var wasAtBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 50;

  var logs = window._kiroLogs || [];
  var html;
  if (!logs.length) {
    html = '<span style="color:var(--text-muted);">' + _tkT('logs.empty', '暂无日志') + '</span>';
  } else {
    html = logs.map(function(l) {
      return _formatLogLine(l.replace(/^\s+/, ''));
    }).join('');
  }

  if (box.innerHTML !== html) {
    box.innerHTML = html;
    if (wasAtBottom) box.scrollTop = box.scrollHeight;
  }
}

function copyLogs() {
  var box = document.getElementById('unified-log-box');
  if (!box) return;

  var logs = window._kiroLogs || [];
  if (!logs.length) {
    showToast(_tkT('toast.logEmpty', '暂无日志可复制'), 'error');
    return;
  }
  var text = box.textContent;

  navigator.clipboard.writeText(text).then(function() {
    showToast(_tkT('toast.logCopied', '日志已复制到剪贴板'), 'success');
  }).catch(function(e) {
    showToast(_tkT('toast.copyFailed', '复制失败') + ': ' + e.message, 'error');
  });
}

function notifyTaskComplete(taskName, success, failed, total) {
  var msg = _tkT('toast.taskCompleteMsg', { name: taskName, s: success, f: failed, t: total }, '{name} 任务完成！成功 {s} / 失败 {f} / 共 {t}');
  showToast(msg, success > 0 ? 'success' : 'error');
  // 提示音（3声短促蜂鸣），受设置开关控制
  var soundEnabled = document.getElementById('cfg-sound');
  if (soundEnabled && soundEnabled.checked) {
    try {
      var ctx = new (window.AudioContext || window.webkitAudioContext)();
      [0, 200, 400].forEach(function(delay) {
        var osc = ctx.createOscillator();
        var gain = ctx.createGain();
        osc.connect(gain);
        gain.connect(ctx.destination);
        osc.frequency.value = 880;
        gain.gain.value = 0.3;
        osc.start(ctx.currentTime + delay / 1000);
        osc.stop(ctx.currentTime + delay / 1000 + 0.1);
      });
    } catch(e) {}
  }
}

var _lastTaskDiagnostics = null;
var _taskDiagnosticsResetBaseline = null;
var _taskDiagnosticGroupKeys = [
  { key: 'otpFailures', goKey: 'OTPFailures' },
  { key: 'postRegistrationFailures', goKey: 'PostRegistrationFailures' },
  { key: 'networkProxyFailures', goKey: 'NetworkProxyFailures' },
  { key: 'graphFailures', goKey: 'GraphFailures' },
  { key: 'riskFailures', goKey: 'RiskFailures' },
  { key: 'emailServiceFailures', goKey: 'EmailServiceFailures' },
  { key: 'proxyFailures', goKey: 'ProxyFailures' }
];

function _diagField(obj, lowerKey, upperKey, fallback) {
  if (!obj) return fallback;
  if (Object.prototype.hasOwnProperty.call(obj, lowerKey)) return obj[lowerKey];
  if (upperKey && Object.prototype.hasOwnProperty.call(obj, upperKey)) return obj[upperKey];
  return fallback;
}

function _diagGroup(diagnostics, lowerKey, upperKey) {
  var group = _diagField(diagnostics, lowerKey, upperKey, null) || {};
  return {
    total: parseInt(_diagField(group, 'total', 'Total', 0), 10) || 0,
    details: _diagField(group, 'details', 'Details', {}) || {}
  };
}

function _diagTopItems(diagnostics, lowerKey, upperKey) {
  return _diagField(diagnostics, lowerKey, upperKey, []) || [];
}

function _clonePlainDiagnostics(diagnostics) {
  if (!diagnostics) return null;
  try {
    return JSON.parse(JSON.stringify(diagnostics));
  } catch (e) {
    return diagnostics;
  }
}

function _diagnosticItemLabel(item) {
  if (!item) return '';
  return item.label != null ? item.label : item.Label;
}

function _diagnosticItemCount(item) {
  if (!item) return 0;
  var value = item.count != null ? item.count : item.Count;
  return parseInt(value, 10) || 0;
}

function _subtractTopItems(currentItems, baselineItems) {
  var base = {};
  (baselineItems || []).forEach(function(item) {
    var label = _diagnosticItemLabel(item);
    if (!label) return;
    base[label] = (base[label] || 0) + _diagnosticItemCount(item);
  });

  var current = {};
  (currentItems || []).forEach(function(item) {
    var label = _diagnosticItemLabel(item);
    if (!label) return;
    current[label] = (current[label] || 0) + _diagnosticItemCount(item);
  });

  var seen = {};
  var result = [];
  (currentItems || []).forEach(function(item) {
    var label = _diagnosticItemLabel(item);
    if (!label || seen[label]) return;
    seen[label] = true;
    var delta = (current[label] || 0) - (base[label] || 0);
    if (delta > 0) result.push({ label: label, count: delta });
  });
  return result;
}

function _subtractDiagnosticGroup(current, baseline) {
  current = current || { total: 0, details: {} };
  baseline = baseline || { total: 0, details: {} };
  var details = {};
  var detailTotal = 0;
  Object.keys(current.details || {}).forEach(function(label) {
    var delta = (parseInt(current.details[label], 10) || 0) -
      (parseInt((baseline.details || {})[label], 10) || 0);
    if (delta > 0) {
      details[label] = delta;
      detailTotal += delta;
    }
  });
  var totalDelta = (parseInt(current.total, 10) || 0) - (parseInt(baseline.total, 10) || 0);
  if (totalDelta < 0) totalDelta = 0;
  return {
    total: detailTotal > 0 ? detailTotal : totalDelta,
    details: details
  };
}

function _subtractSendOTPDiagnostics(current, baseline) {
  current = current || {};
  baseline = baseline || {};
  var result = {};
  Object.keys(current).forEach(function(key) {
    var items = _subtractTopItems(current[key], baseline[key]);
    if (items.length) result[key] = items;
  });
  return result;
}

function _diagnosticsAfterReset(diagnostics) {
  diagnostics = diagnostics || {};
  if (!_taskDiagnosticsResetBaseline) return diagnostics;

  var baseline = _taskDiagnosticsResetBaseline || {};
  var result = {};
  _taskDiagnosticGroupKeys.forEach(function(cfg) {
    result[cfg.key] = _subtractDiagnosticGroup(
      _diagGroup(diagnostics, cfg.key, cfg.goKey),
      _diagGroup(baseline, cfg.key, cfg.goKey)
    );
  });
  result.sendOTPDiagnostics = _subtractSendOTPDiagnostics(
    _diagField(diagnostics, 'sendOTPDiagnostics', 'SendOTPDiagnostics', {}) || {},
    _diagField(baseline, 'sendOTPDiagnostics', 'SendOTPDiagnostics', {}) || {}
  );
  result.topFailures = _subtractTopItems(
    _diagTopItems(diagnostics, 'topFailures', 'TopFailures'),
    _diagTopItems(baseline, 'topFailures', 'TopFailures')
  );
  return result;
}

function _clearTaskDiagnosticsResetBaseline() {
  _taskDiagnosticsResetBaseline = null;
}

function _diagLabel(label) {
  var map = {
    '验证码发送失败': 'overview.diagnosticsOtpSendFailed',
    '验证码无效': 'overview.diagnosticsOtpInvalid',
    '验证码超时': 'overview.diagnosticsOtpTimeout',
    '模型列表失败': 'overview.diagnosticsModelListFailed'
  };
  if (map[label]) return _tkT(map[label], label);
  return label;
}

function _orderedDiagnosticDetails(details, order) {
  var items = [];
  var seen = {};
  (order || []).forEach(function(label) {
    var count = parseInt(details[label], 10) || 0;
    if (count > 0) {
      items.push({ label: label, count: count });
      seen[label] = true;
    }
  });
  Object.keys(details || {}).forEach(function(label) {
    if (seen[label]) return;
    var count = parseInt(details[label], 10) || 0;
    if (count > 0) items.push({ label: label, count: count });
  });
  items.sort(function(a, b) {
    var ai = (order || []).indexOf(a.label);
    var bi = (order || []).indexOf(b.label);
    if (ai >= 0 || bi >= 0) return (ai < 0 ? 9999 : ai) - (bi < 0 ? 9999 : bi);
    if (a.count === b.count) return a.label.localeCompare(b.label);
    return b.count - a.count;
  });
  return items;
}

function formatDiagnosticGroup(label, group, order) {
  var details = _orderedDiagnosticDetails(group.details || {}, order || []);
  if (!group.total && !details.length) return label + '：-';
  var detailText = details.map(function(item) {
    return _diagLabel(item.label) + ' ' + item.count;
  }).join(' / ');
  return label + '：' + group.total + (detailText ? '（' + detailText + '）' : '');
}

function _renderDiagnosticChip(text) {
  return '<span style="padding:3px 7px;border-radius:999px;background:var(--bg-subtle);border:1px solid var(--border);">' +
    _escapeLogHtml(text) + '</span>';
}

function _renderDiagnosticRow(title, body) {
  return '<div style="display:flex;gap:8px;justify-content:space-between;border-bottom:1px dashed var(--border);padding:3px 0;">' +
    '<span style="font-weight:600;color:var(--text);">' + _escapeLogHtml(title) + '</span>' +
    '<span style="text-align:right;min-width:0;">' + _escapeLogHtml(body) + '</span>' +
    '</div>';
}

function _formatTopItems(items) {
  if (!items || !items.length) return '-';
  return items.map(function(item) {
    var label = item.label != null ? item.label : item.Label;
    var count = item.count != null ? item.count : item.Count;
    return String(label) + ' ' + (parseInt(count, 10) || 0);
  }).join(' / ');
}

function _formatSendOTPDiagnostics(diagnostics) {
  var data = _diagField(diagnostics, 'sendOTPDiagnostics', 'SendOTPDiagnostics', {}) || {};
  var parts = [];
  ['provider', 'domain', 'emailProxy', 'proxy'].forEach(function(key) {
    if (data[key] && data[key].length) {
      parts.push(key + '：' + _formatTopItems(data[key]));
    }
  });
  return parts.length ? parts.join('；') : '-';
}

function renderTaskDiagnostics(diagnostics) {
  _lastTaskDiagnostics = diagnostics || null;
  var root = document.getElementById('st-diagnostics');
  if (!root) return;
  var summaryEl = document.getElementById('st-diagnostics-summary');
  var detailsEl = document.getElementById('st-diagnostics-details');
  var toggleText = document.getElementById('st-diagnostics-toggle-text');
  if (!summaryEl || !detailsEl) return;

  diagnostics = _diagnosticsAfterReset(diagnostics || {});
  var groups = [
    {
      key: 'otpFailures', goKey: 'OTPFailures',
      label: _tkT('overview.diagnosticsOtp', 'OTP失败'),
      order: ['验证码发送失败', '验证码无效', '验证码超时']
    },
    {
      key: 'postRegistrationFailures', goKey: 'PostRegistrationFailures',
      label: _tkT('overview.diagnosticsPostRegistration', '注册后失败'),
      order: ['模型列表失败', '验活失败', '账号封禁', 'Token交换失败', 'Kiro授权失败']
    },
    {
      key: 'networkProxyFailures', goKey: 'NetworkProxyFailures',
      label: _tkT('overview.diagnosticsNetwork', '网络/代理问题'),
      order: ['设备授权', 'Portal 初始化', '工作流初始化', '提交邮箱', '发送验证码', '等待验证码', '注册初始化', '验活', '其他阶段']
    },
    {
      key: 'graphFailures', goKey: 'GraphFailures',
      label: _tkT('overview.diagnosticsGraph', 'Graph失败'),
      order: ['Graph Token失效', 'Graph权限错误', 'Graph网络错误', 'Graph响应异常']
    },
    { key: 'riskFailures', goKey: 'RiskFailures', label: _tkT('overview.diagnosticsRisk', '风控/拦截') },
    { key: 'emailServiceFailures', goKey: 'EmailServiceFailures', label: _tkT('overview.diagnosticsEmailService', '邮箱服务问题') },
    { key: 'proxyFailures', goKey: 'ProxyFailures', label: _tkT('overview.diagnosticsProxy', '代理池/Clash') }
  ];

  var rendered = groups.map(function(cfg) {
    var group = _diagGroup(diagnostics, cfg.key, cfg.goKey);
    return {
      label: cfg.label,
      total: group.total,
      text: formatDiagnosticGroup(cfg.label, group, cfg.order)
    };
  });

  var nonEmpty = rendered.filter(function(item) { return item.total > 0; });
  var summaryItems = nonEmpty.slice(0, 6);
  summaryEl.innerHTML = summaryItems.length
    ? summaryItems.map(function(item) { return _renderDiagnosticChip(item.text); }).join('')
    : '<span style="color:var(--text-muted);">-</span>';

  var rows = rendered.map(function(item) {
    return _renderDiagnosticRow(item.label, item.text.replace(item.label + '：', ''));
  });
  rows.push(_renderDiagnosticRow(_tkT('overview.diagnosticsSendOTP', 'send-otp诊断'), _formatSendOTPDiagnostics(diagnostics)));
  rows.push(_renderDiagnosticRow(_tkT('overview.diagnosticsTopFailures', '失败Top'), _formatTopItems(_diagTopItems(diagnostics, 'topFailures', 'TopFailures'))));
  detailsEl.innerHTML = rows.join('');

  var expanded = root.getAttribute('data-diagnostics-expanded') === 'true';
  detailsEl.style.display = expanded ? 'block' : 'none';
  if (toggleText) {
    toggleText.textContent = expanded
      ? _tkT('overview.diagnosticsCollapse', '收起')
      : _tkT('overview.diagnosticsExpand', '展开');
  }
}

function toggleTaskDiagnostics() {
  var root = document.getElementById('st-diagnostics');
  if (!root) return;
  var expanded = root.getAttribute('data-diagnostics-expanded') === 'true';
  root.setAttribute('data-diagnostics-expanded', expanded ? 'false' : 'true');
  renderTaskDiagnostics(_lastTaskDiagnostics);
}

function resetTaskDiagnostics() {
  _taskDiagnosticsResetBaseline = _clonePlainDiagnostics(_lastTaskDiagnostics || {});
  renderTaskDiagnostics(_lastTaskDiagnostics);
}

async function startTask() {
  if (typeof setTaskActionState === 'function') setTaskActionState('starting');
  try {
    var cfg = getFormConfig();

    if (typeof saveConfig === 'function') {
      await saveConfig({ silent: true });
    }

    var result = await window.go.main.App.StartTask(cfg);
    // 账号不足时弹窗确认，用户选择继续则以可用数量启动
    if (result.confirm === 'outlook_insufficient') {
      if (typeof setTaskActionState === 'function') setTaskActionState('idle');
      showConfirmModal('账号不足', result.message, '继续注册', function() {
        if (cfg.successTarget > 0) {
          cfg.successTarget = result.available;
        } else {
          cfg.count = result.available;
        }
        startTaskWithConfig(cfg);
      });
      return;
    }
    if (result.error) {
      if (typeof setTaskActionState === 'function') setTaskActionState('idle');
      showToast(result.error, 'error');
      return;
    }
    if (typeof setTaskActionState === 'function') setTaskActionState('running');
    else updateUIStatus(true);
    _clearTaskDiagnosticsResetBaseline();
    showToast('任务已启动');
    switchToOverviewAfterTaskStart();
  } catch(e) {
    if (typeof setTaskActionState === 'function') setTaskActionState('idle');
    showToast('启动失败: ' + e.message, 'error');
  }
}

// 以指定配置启动任务（确认弹窗回调用）
async function startTaskWithConfig(cfg) {
  if (typeof setTaskActionState === 'function') setTaskActionState('starting');
  try {
    var result = await window.go.main.App.StartTask(cfg);
    if (result.error) {
      if (typeof setTaskActionState === 'function') setTaskActionState('idle');
      showToast(result.error, 'error');
      return;
    }
    if (typeof setTaskActionState === 'function') setTaskActionState('running');
    else updateUIStatus(true);
    _clearTaskDiagnosticsResetBaseline();
    showToast(_tkT('toast.taskStarted', '任务已启动'));
    switchToOverviewAfterTaskStart();
  } catch(e) {
    if (typeof setTaskActionState === 'function') setTaskActionState('idle');
    showToast(_tkT('toast.taskStartFailed', '启动失败') + ': ' + e.message, 'error');
  }
}

var _confirmCallback = null;

function showConfirmModal(title, message, btnText, callback) {
  document.getElementById('confirm-title').textContent = title;
  document.getElementById('confirm-message').textContent = message;
  document.getElementById('confirm-action-btn').textContent = btnText || '确认';
  _confirmCallback = callback;
  document.getElementById('confirm-modal').classList.add('show');
}

function closeConfirmModal() {
  document.getElementById('confirm-modal').classList.remove('show');
  _confirmCallback = null;
}

function confirmAction() {
  var cb = _confirmCallback;
  closeConfirmModal();
  if (cb) cb();
}

async function stopTask() {
  if (typeof setTaskActionState === 'function') setTaskActionState('stopping');
  try {
    var result = await window.go.main.App.StopTask();
    if (result.error) {
      if (String(result.error).indexOf('没有正在运行的任务') >= 0) {
        if (typeof setTaskActionState === 'function') setTaskActionState('idle');
        showToast(_tkT('toast.taskStopped', '任务已停止'));
        return;
      }
      if (typeof setTaskActionState === 'function') setTaskActionState('running');
      showToast(result.error, 'error');
      return;
    }
    showToast(_tkT('toast.taskStopping', '正在停止任务...'));
  } catch(e) {
    if (typeof setTaskActionState === 'function') setTaskActionState('running');
    showToast(_tkT('toast.taskStopFailed', '停止失败') + ': ' + (e.message || e), 'error');
  }
}

// ===== 更新系统 =====

if (window.runtime) {
  window.runtime.EventsOn('update-available', function(data) {
    updateInfo = data;
    showUpdateModal(data);
  });
  window.runtime.EventsOn('kiro-rs-sync-result', function(result) {
    if (result.error) {
      showToast('kiro.rs 同步失败: ' + result.error, 'error');
      return;
    }
    var msg = 'kiro.rs 同步完成：成功 ' + result.success + ' / 失败 ' + result.failed;
    showToast(msg, result.failed > 0 ? 'error' : 'success');
    var accountPoolPage = document.getElementById('page-account-pool');
    if (accountPoolPage && accountPoolPage.classList.contains('active') && typeof loadAccountPool === 'function') {
      loadAccountPool();
    }
  });
}

function showUpdateModal(data) {
  document.getElementById('update-current-version').textContent = data.currentVersion || '-';
  document.getElementById('update-latest-version').textContent = data.latestVersion || data.version || '-';
  document.getElementById('update-release-date').textContent = data.releaseDate || '-';
  document.getElementById('update-changelog').textContent = data.changelog || '-';

  // 记下 release 页面地址供「前往下载」按钮使用
  window._latestReleaseURL = data.releaseURL || 'https://github.com/huey1in/kirox/releases/latest';

  document.getElementById('update-modal').classList.add('show');
}

async function closeUpdateModal() {
  document.getElementById('update-modal').classList.remove('show');
}

function openReleasePage() {
  var url = window._latestReleaseURL || 'https://github.com/huey1in/kirox/releases/latest';
  if (window.go && window.go.main && window.go.main.App && window.go.main.App.OpenURL) {
    window.go.main.App.OpenURL(url);
  } else {
    window.open(url, '_blank');
  }
}

async function checkUpdateManually() {
  try {
    var result = await window.go.main.App.CheckUpdate();
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }
    if (result.hasUpdate) {
      updateInfo = result;
      showUpdateModal(result);
    } else {
      showToast(_tkT('toast.upToDate', '当前已是最新版本'));
    }
  } catch(e) {
    showToast(_tkT('toast.checkUpdateFailed', '检查更新失败') + ': ' + e.message, 'error');
  }
}

// ===== 状态轮询 =====

setInterval(async function() {
  try {
    var s = await window.go.main.App.GetStatus();
    updateUIStatus(s.running);
    // 注册页状态徽章
    var regBadge = document.getElementById('reg-status-badge');
    if (regBadge) {
      var _tt = (window.I18N && window.I18N.t) ? window.I18N.t : function(k){return k;};
      var rTxt = s.running ? _tt('status.running') : _tt('status.idle');
      if (!rTxt || rTxt === 'status.running' || rTxt === 'status.idle') rTxt = s.running ? '运行中' : '空闲';
      regBadge.textContent = rTxt;
      regBadge.className = 'db-badge ' + (s.running ? 'db-badge-running' : 'db-badge-idle');
    }
    var successTarget = s.successTargetEnabled ? (parseInt(s.successTarget, 10) || 0) : 0;
    var successTargetMode = successTarget > 0;
    var progressDone = successTargetMode ? s.success : s.completed;
    var progressTotal = successTargetMode ? successTarget : s.total;
    document.getElementById('st-progress').textContent = progressDone + '/' + progressTotal;
    document.getElementById('st-success').textContent = s.success;
    document.getElementById('st-failed').textContent = s.failed;
    if (s.elapsed > 0) document.getElementById('st-elapsed').textContent = formatTime(s.elapsed);
    var pct = progressTotal > 0 ? Math.round(progressDone / progressTotal * 100) : 0;
    if (pct > 100) pct = 100;
    document.getElementById('progress-bar').style.width = pct + '%';
    // 检测任务状态切换
    var wasRunning = _prevRunning;
    if (!wasRunning && s.running) {
      _clearTaskDiagnosticsResetBaseline();
    }
    if (_prevRunning && !s.running && s.completed > 0) {
      notifyTaskComplete('Kiro', s.success, s.failed, s.completed);
      if (typeof loadEmailProviderStats === 'function') loadEmailProviderStats();
    }
    _prevRunning = s.running;
    // 状态指示灯
    var dot = document.getElementById('st-dot');
    if (s.running) { dot.classList.add('running'); } else { dot.classList.remove('running'); }
    // 平均耗时
    var avgEl = document.getElementById('st-avg');
    if (s.completed > 0 && s.elapsed > 0) {
      avgEl.textContent = (s.elapsed / s.completed).toFixed(1) + 's';
    } else {
      avgEl.textContent = '-';
    }
    // 成功率
    var rateEl = document.getElementById('st-rate');
    if (s.completed > 0) {
      rateEl.textContent = Math.round(s.success / s.completed * 100) + '%';
      rateEl.style.color = s.success > 0 ? 'var(--success)' : 'var(--danger)';
    } else {
      rateEl.textContent = '-';
      rateEl.style.color = '';
    }
    // 预计剩余
    var etaEl = document.getElementById('st-eta');
    if (s.running && successTargetMode && s.success > 0 && successTarget > s.success) {
      var avgSuccessTime = s.elapsed / s.success;
      var remaining = (successTarget - s.success) * avgSuccessTime;
      etaEl.textContent = formatTime(remaining);
    } else if (s.running && s.completed > 0 && s.total > s.completed) {
      var avgTime = s.elapsed / s.completed;
      var remaining = (s.total - s.completed) * avgTime;
      etaEl.textContent = formatTime(remaining);
    } else {
      etaEl.textContent = '-';
    }
    renderTaskDiagnostics(s.diagnostics || s.Diagnostics || null);
  } catch(e) {}
  try {
    var kiroLogs = await window.go.main.App.GetLogs() || [];
    window._kiroLogs = kiroLogs;
    renderUnifiedLogs();
  } catch(e) {}

}, 2000);

// 切换语言时立刻按新语言重渲染日志（renderUnifiedLogs 自带 innerHTML 短路，无副作用）
window.addEventListener('i18n:changed', function() {
  if (typeof renderUnifiedLogs === 'function') renderUnifiedLogs();
  if (typeof renderTaskDiagnostics === 'function') renderTaskDiagnostics(_lastTaskDiagnostics);
});
