// ===== 账号池：导入 / 导出 Kiro 账号 =====

var accountPoolState = {
  accounts: [],
  outputDir: ''
};

function accountPoolEscapeHtml(s) {
  if (s == null) return '';
  return String(s).replace(/[&<>"']/g, function(c) {
    return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
  });
}

function accountPoolTokenStatus(account) {
  var missing = [];
  if (!account.refreshToken) missing.push('RefreshToken');
  if (!account.clientId) missing.push('ClientID');
  if (!account.clientSecret) missing.push('ClientSecret');
  if (missing.length === 0) {
    return '<span style="color:#10b981;">完整</span>';
  }
  return '<span style="color:#f59e0b;" title="缺少 ' + accountPoolEscapeHtml(missing.join(', ')) + '">缺少字段</span>';
}

function accountPoolKiroRSSyncStatus(account) {
  if (account && account.kiroRsSynced === true) {
    return '<span style="color:#10b981;">已同步</span>';
  }
  return '<span style="color:#f59e0b;">未同步</span>';
}

async function loadAccountPool() {
  var res;
  try {
    res = await window.go.main.App.ListAccountPool();
  } catch (e) {
    showToast('加载账号池失败: ' + e, 'error');
    return;
  }

  var hint = document.getElementById('account-pool-output-dir');
  if (hint) {
    hint.textContent = res && res.outputDir
      ? '已自动加载：' + res.outputDir
      : '已自动加载输出文件夹中的账号';
  }

  if (!res || !res.success) {
    accountPoolState.accounts = [];
    renderAccountPoolTable();
    updateAccountPoolSummary();
    if (res && res.error) showToast(res.error, 'error');
    return;
  }

  accountPoolState.accounts = res.accounts || [];
  accountPoolState.outputDir = res.outputDir || '';
  renderAccountPoolTable();
  updateAccountPoolSummary();
}

function updateAccountPoolSummary() {
  var count = accountPoolState.accounts.length;
  var countEl = document.getElementById('account-pool-count');
  var summaryEl = document.getElementById('account-pool-summary');
  if (countEl) countEl.textContent = count;
  if (summaryEl) summaryEl.textContent = '共 ' + count + ' 个账号';
}

function renderAccountPoolTable() {
  var body = document.getElementById('account-pool-table-body');
  if (!body) return;
  if (!accountPoolState.accounts.length) {
    body.innerHTML = '<tr><td colspan="7" style="padding:24px;text-align:center;color:var(--muted);font-size:13px;">账号池为空，请导入账号 JSON 或先完成注册。</td></tr>';
    return;
  }

  body.innerHTML = accountPoolState.accounts.map(function(account, idx) {
    return (
      '<tr style="border-top:1px solid var(--border);">' +
        '<td style="padding:8px 12px;color:var(--muted);font-size:12px;">' + (idx + 1) + '</td>' +
        '<td style="padding:8px;">' + accountPoolEscapeHtml(account.email || '') + '</td>' +
        '<td style="padding:8px;font-size:12px;color:var(--muted);">' + accountPoolEscapeHtml(account.subscription || '') + '</td>' +
        '<td style="padding:8px;font-size:12px;color:var(--muted);">' + accountPoolEscapeHtml(account.priority == null ? '' : account.priority) + '</td>' +
        '<td style="padding:8px;font-size:12px;color:var(--muted);">' + accountPoolEscapeHtml(account.time || '') + '</td>' +
        '<td style="padding:8px 12px;font-size:12px;">' + accountPoolTokenStatus(account) + '</td>' +
        '<td style="padding:8px 12px;font-size:12px;">' + accountPoolKiroRSSyncStatus(account) + '</td>' +
      '</tr>'
    );
  }).join('');
}

function openAccountPoolSyncModal() {
  var modal = document.getElementById('account-pool-sync-modal');
  if (modal) modal.classList.add('show');
}

function closeAccountPoolSyncModal() {
  var modal = document.getElementById('account-pool-sync-modal');
  if (modal) modal.classList.remove('show');
}

function openAccountPoolImportModal() {
  var modal = document.getElementById('account-pool-import-modal');
  var input = document.getElementById('account-pool-import-data');
  if (input) input.value = '';
  if (modal) modal.classList.add('show');
}

function closeAccountPoolImportModal() {
  var modal = document.getElementById('account-pool-import-modal');
  if (modal) modal.classList.remove('show');
}

async function readAccountPoolClipboard() {
  try {
    var text = await navigator.clipboard.readText();
    var input = document.getElementById('account-pool-import-data');
    if (input) input.value = text;
    showToast('已读取剪切板');
  } catch (e) {
    showToast('读取剪切板失败: ' + e.message, 'error');
  }
}

async function importAccountPoolJSON() {
  var input = document.getElementById('account-pool-import-data');
  var data = input ? input.value.trim() : '';
  if (!data) {
    showToast('请先粘贴账号 JSON', 'error');
    return;
  }
  try {
    var res = await window.go.main.App.ImportAccountPoolJSON(data);
    if (!res || !res.success) {
      showToast((res && res.error) || '导入失败', 'error');
      return;
    }
    closeAccountPoolImportModal();
    await loadAccountPool();
    showToast('导入完成：新增 ' + (res.imported || 0) + '，更新 ' + (res.updated || 0) + '，跳过 ' + (res.skipped || 0));
  } catch (e) {
    showToast('导入失败: ' + e.message, 'error');
  }
}

async function exportAccountPoolToClipboard() {
  try {
    var res = await window.go.main.App.ExportAccountPoolJSON();
    if (!res || !res.success) {
      showToast((res && res.error) || '导出失败', 'error');
      return;
    }
    var text = res.data || res.json || '[]';
    await navigator.clipboard.writeText(text);
    showToast('已导出 ' + (res.count || 0) + ' 个账号到剪切板');
  } catch (e) {
    showToast('导出失败: ' + e.message, 'error');
  }
}

async function startAccountPoolKiroRSSync(mode) {
  closeAccountPoolSyncModal();
  await syncAccountPoolToKiroRS(mode);
}

async function syncAccountPoolToKiroRS(mode) {
  var btn = document.getElementById('btn-sync-kiro-rs');
  var originalText = btn ? btn.textContent : '';
  if (btn) { btn.disabled = true; btn.textContent = '同步中...'; }
  showToast('已开始同步到 kiro.rs，进度见运行日志', 'success');
  try {
    if (mode === 'all') {
      var result = await window.go.main.App.SyncAccountPoolToKiroRS('all');
    } else {
      var result = await window.go.main.App.SyncAccountPoolToKiroRS('unsynced');
    }
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }
    var msg = 'kiro.rs 同步完成：成功 ' + result.syncSuccess + ' / 失败 ' + result.syncFailed;
    if ((result.removedRejected || 0) > 0) msg += '，已删除本地失效账号 ' + result.removedRejected + ' 个';
    if (result.message) msg += '（' + result.message + '）';
    showToast(msg, result.syncFailed > 0 ? 'error' : 'success');
    await loadAccountPool();
  } catch (e) {
    showToast('同步失败: ' + e.message, 'error');
  } finally {
    if (btn) { btn.disabled = false; btn.textContent = originalText || '同步到 kiro.rs'; }
  }
}
