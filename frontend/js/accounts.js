// ===== Outlook 账号管理 =====

var outlookCurrentPage = 1;
var outlookPageSize = 10;
var outlookAllAccounts = [];
var outlookStatusFilter = 'all';
// 当前筛选条件下的全部结果（非分页），供全选/反选/批量删除使用
var outlookFilteredAccounts = [];
// 选中账号集合：以 email 为键当 Set 用，独立于渲染，避免 3 秒自动刷新重渲染时丢失选中
var outlookSelectedEmails = {};

function _accT(key, varsOrFallback, fallbackMaybe) {
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

function openAddOutlookModal() {
  document.getElementById('add-outlook-modal').classList.add('show');
}

function closeAddOutlookModal() {
  document.getElementById('add-outlook-modal').classList.remove('show');
  document.getElementById('cfg-outlook-data').value = '';
}

async function addOutlookAccounts() {
  var data = document.getElementById('cfg-outlook-data').value.trim();
  if (!data) {
    showToast(_accT('accounts.inputRequired', '请先输入 Outlook 账号数据'), 'error');
    return;
  }
  try {
    var result = await window.go.main.App.AddOutlookAccounts(data);
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }

    closeAddOutlookModal();
    await loadOutlookAccountsList();
    showToast(_accT('accounts.addedSummary', { n: result.added, total: result.total }, '成功添加 {n} 个账号，当前共 {total} 个'));
  } catch(e) {
    showToast(_accT('toast.addFailed', '添加失败') + ': ' + e.message, 'error');
  }
}

async function importOutlookFile() {
  try {
    var filePath = await window.go.main.App.SelectOutlookFile();
    if (!filePath) {
      return;
    }

    var result = await window.go.main.App.ImportOutlookFile(filePath);
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }

    await loadOutlookAccountsList();
    closeAddOutlookModal();
    showToast(_accT('accounts.importSummary', { n: result.added, total: result.total }, '成功导入 {n} 个账号，当前共 {total} 个'));
  } catch(e) {
    showToast(_accT('accounts.importFailed', '导入失败') + ': ' + e.message, 'error');
  }
}

async function loadOutlookAccountsList() {
  try {
    var accounts = await window.go.main.App.GetOutlookAccounts();
    outlookAllAccounts = accounts || [];
    rebuildOutlookFilterOptions();
    renderOutlookPage();
  } catch(e) {
    console.error('加载账号列表失败:', e);
  }
}

// 状态筛选下拉固定选项的中文标签；具体失败原因(failReason)作为动态选项，标签即其自身
var outlookFilterLabels = { all: '全部', success: '成功', unregistered: '未注册' };

// 重建状态筛选下拉：固定项(全部/成功/未注册) + 当前账号中出现过的失败原因(动态去重)
function rebuildOutlookFilterOptions() {
  var optionsEl = document.querySelector('#outlook-filter-dropdown .dropdown-options');
  if (!optionsEl) return;

  // 收集去重的失败原因，保持稳定顺序便于查看
  var reasons = [];
  var seen = {};
  outlookAllAccounts.forEach(function(a) {
    if (a.failReason && !seen[a.failReason]) {
      seen[a.failReason] = true;
      reasons.push(a.failReason);
    }
  });
  reasons.sort();

  // 固定项 + 动态失败原因项
  var items = [
    { value: 'all', label: '全部' },
    { value: 'success', label: '成功' },
    { value: 'unregistered', label: '未注册' }
  ];
  reasons.forEach(function(r) { items.push({ value: r, label: r }); });

  // 若当前筛选值已不在选项中(失败原因消失)，回退到全部
  var valid = items.some(function(it) { return it.value === outlookStatusFilter; });
  if (!valid) outlookStatusFilter = 'all';

  var html = '';
  items.forEach(function(it) {
    var sel = it.value === outlookStatusFilter ? ' selected' : '';
    // 用单引号包裹 value，避免与 onclick 外层双引号冲突
    var escaped = it.value.replace(/'/g, "\\'");
    html += '<div class="dropdown-option' + sel + '" onclick="setOutlookStatusFilter(\'' +
      escaped + '\')">' + it.label + '</div>';
  });
  optionsEl.innerHTML = html;

  // 同步下拉显示文案
  var labelEl = document.getElementById('outlook-filter-label');
  if (labelEl) labelEl.textContent = outlookFilterLabels[outlookStatusFilter] || outlookStatusFilter;
}

function setOutlookStatusFilter(status) {
  outlookStatusFilter = status;
  outlookCurrentPage = 1;

  // 更新下拉框显示文案：固定项查表，动态失败原因项直接用其值
  var labelEl = document.getElementById('outlook-filter-label');
  if (labelEl) labelEl.textContent = outlookFilterLabels[status] || status;

  // 更新下拉选项高亮，并收起下拉
  var dropdown = document.getElementById('outlook-filter-dropdown');
  if (dropdown) {
    dropdown.querySelectorAll('.dropdown-option').forEach(function(opt) {
      var onclick = opt.getAttribute('onclick') || '';
      var optStatus = onclick.indexOf("'" + status + "'") !== -1;
      opt.classList.toggle('selected', optStatus);
    });
    var sel = dropdown.querySelector('.dropdown-selected');
    var opts = dropdown.querySelector('.dropdown-options');
    if (sel) sel.classList.remove('active');
    if (opts) opts.classList.remove('show');
  }

  renderOutlookPage();
}

function renderOutlookPage() {
  var accounts = outlookAllAccounts;

  // 状态筛选
  // all: 全部；success: 注册成功；unregistered: 从未跑过(无失败原因且未注册)；
  // 其余值视为具体失败原因(failReason)，按原因精确匹配。
  if (outlookStatusFilter === 'success') {
    accounts = accounts.filter(function(a) { return a.registered && a.success; });
  } else if (outlookStatusFilter === 'unregistered') {
    accounts = accounts.filter(function(a) { return !a.registered && !a.failReason; });
  } else if (outlookStatusFilter !== 'all') {
    accounts = accounts.filter(function(a) { return a.failReason === outlookStatusFilter; });
  }

  // 暂存筛选后的全部结果（非分页），供全选/反选/批量删除使用
  outlookFilteredAccounts = accounts;

  // 清理选中集合中已不存在的 email（账号被删/清空后防止脏数据），范围为全量账号
  var existingEmails = {};
  outlookAllAccounts.forEach(function(a) { existingEmails[a.email] = true; });
  Object.keys(outlookSelectedEmails).forEach(function(em) {
    if (!existingEmails[em]) delete outlookSelectedEmails[em];
  });

  var tbody = document.getElementById('parsed-outlook-body');
  var pager = document.getElementById('outlook-pager');
  var countEl = document.getElementById('outlook-count');

  if (countEl) countEl.textContent = accounts ? accounts.length : 0;

  if (accounts && accounts.length > 0) {
    var total = accounts.length;
    var totalPages = Math.ceil(total / outlookPageSize);
    if (outlookCurrentPage > totalPages) outlookCurrentPage = totalPages;
    if (outlookCurrentPage < 1) outlookCurrentPage = 1;

    var start = (outlookCurrentPage - 1) * outlookPageSize;
    var end = Math.min(start + outlookPageSize, total);
    var pageAccounts = accounts.slice(start, end);

    var html = '';
    pageAccounts.forEach(function(acc, i) {
      var globalIdx = start + i;
      // 状态显示优先级：成功 > 失败原因(failReason) > 未注册
      var status, statusColor;
      if (acc.registered && acc.success) {
        status = _accT('status.success', '成功');
        statusColor = 'var(--success)';
      } else if (acc.failReason) {
        // 失败原因优先：即便 registered 仍为 false(待重试)，也直接展示具体失败原因
        status = acc.failReason;
        statusColor = 'var(--danger)';
      } else {
        status = _accT('status.unregistered', '未注册');
        statusColor = 'var(--text-muted)';
      }
      var addedTime = acc.addedAt ? acc.addedAt.substring(5, 16) : '-';
      var checked = outlookSelectedEmails[acc.email] ? ' checked' : '';
      html += '<tr><td style="padding:8px 16px;"><input type="checkbox" class="outlook-row-cb" data-email="' + acc.email + '" onclick="toggleOutlookRow(this)"' + checked + '></td>';
      html += '<td>' + (globalIdx+1) + '</td><td>' + acc.email + '</td>';
      html += '<td style="color:' + statusColor + ';font-weight:600;">' + status + '</td>';
      html += '<td style="font-size:11px;color:var(--text-muted);font-family:var(--font-mono);">' + addedTime + '</td>';
      html += '<td style="text-align:right;"><a href="javascript:void(0)" onclick="deleteOutlookAccount(\'' + acc.email + '\')" style="color:var(--danger);">' + _accT('common.delete', '删除') + '</a></td></tr>';
    });
    tbody.innerHTML = html;

    if (totalPages > 1) {
      pager.style.display = 'flex';
      document.getElementById('outlook-pager-info').textContent = _accT('accounts.pagerInfo', { cur: outlookCurrentPage, total: totalPages, n: total }, '第 {cur} / {total} 页 (共 {n} 个)');
      document.getElementById('outlook-pager-prev').disabled = outlookCurrentPage <= 1;
      document.getElementById('outlook-pager-next').disabled = outlookCurrentPage >= totalPages;
    } else {
      pager.style.display = 'none';
    }
  } else {
    tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-muted);padding:20px;">' + _accT('accounts.emptyRow', '暂无邮箱账号') + '</td></tr>';
    pager.style.display = 'none';
  }

  // 同步全选框状态、选中计数与批量删除按钮可用性
  refreshOutlookSelectionUI();
}

function changeOutlookPage(delta) {
  outlookCurrentPage += delta;
  if (outlookCurrentPage < 1) outlookCurrentPage = 1;
  renderOutlookPage();
}

// 切换单行选中状态
function toggleOutlookRow(cb) {
  var email = cb.getAttribute('data-email');
  if (cb.checked) {
    outlookSelectedEmails[email] = true;
  } else {
    delete outlookSelectedEmails[email];
  }
  refreshOutlookSelectionUI();
}

// 表头全选/取消：作用于当前筛选条件下的全部结果（跨页）
function toggleOutlookSelectAll(cb) {
  if (cb.checked) {
    outlookFilteredAccounts.forEach(function(a) { outlookSelectedEmails[a.email] = true; });
  } else {
    outlookFilteredAccounts.forEach(function(a) { delete outlookSelectedEmails[a.email]; });
  }
  // 仅当前页有 DOM 复选框，重渲染同步勾选状态
  renderOutlookPage();
}

// 反选：对当前筛选结果逐个翻转选中状态
function invertOutlookSelection() {
  outlookFilteredAccounts.forEach(function(a) {
    if (outlookSelectedEmails[a.email]) {
      delete outlookSelectedEmails[a.email];
    } else {
      outlookSelectedEmails[a.email] = true;
    }
  });
  renderOutlookPage();
}

// 同步选中计数、批量删除按钮可用性，以及全选框的选中/半选状态
function refreshOutlookSelectionUI() {
  var selectedCount = Object.keys(outlookSelectedEmails).length;
  var countEl = document.getElementById('outlook-selected-count');
  if (countEl) countEl.textContent = selectedCount;

  var btn = document.getElementById('outlook-batch-delete-btn');
  if (btn) btn.disabled = selectedCount === 0;

  // 按「筛选结果中被选中的数量」决定全选框状态
  var selectAll = document.getElementById('outlook-select-all');
  if (selectAll) {
    var filteredTotal = outlookFilteredAccounts.length;
    var selectedInFiltered = outlookFilteredAccounts.filter(function(a) {
      return outlookSelectedEmails[a.email];
    }).length;
    if (filteredTotal === 0 || selectedInFiltered === 0) {
      selectAll.checked = false;
      selectAll.indeterminate = false;
    } else if (selectedInFiltered === filteredTotal) {
      selectAll.checked = true;
      selectAll.indeterminate = false;
    } else {
      selectAll.checked = false;
      selectAll.indeterminate = true;
    }
  }
}

// 批量删除选中账号
function batchDeleteOutlookAccounts() {
  var emails = Object.keys(outlookSelectedEmails);
  if (emails.length === 0) {
    showToast('请先选择要删除的账号');
    return;
  }
  showConfirmModal('批量删除', '确认删除选中的 ' + emails.length + ' 个账号？此操作不可恢复！', '确认删除', async function() {
    try {
      var result = await window.go.main.App.DeleteOutlookAccounts(emails);
      if (result.error) {
        showToast(result.error, 'error');
        return;
      }
      outlookSelectedEmails = {};
      await loadOutlookAccountsList();
      showToast('已删除 ' + (result.removed || 0) + ' 个账号');
    } catch(e) {
      showToast('批量删除失败: ' + e.message, 'error');
    }
  });
}

async function deleteOutlookAccount(email) {
  showConfirmModal(
    _accT('accounts.deleteTitle', '删除账号'),
    _accT('accounts.deleteMsg', { email: email }, '确认删除账号 {email} ?'),
    _accT('accounts.deleteConfirm', '确认删除'),
    async function() {
      try {
        var result = await window.go.main.App.DeleteOutlookAccount(email);
        if (result.error) {
          showToast(result.error, 'error');
          return;
        }
        showToast(_accT('accounts.deletedOne', '账号已删除'));
        await loadOutlookAccountsList();
      } catch(e) {
        showToast(_accT('toast.deleteFailed', '删除失败') + ': ' + e.message, 'error');
      }
    }
  );
}

function clearAllOutlookAccounts() {
  showConfirmModal(
    _accT('accounts.clearAllTitle', '清空微软邮箱'),
    _accT('accounts.clearAllMsg', '确认清空所有微软邮箱账号？此操作不可恢复！'),
    _accT('accounts.clearAllConfirm', '确认清空'),
    async function() {
      try {
        var result = await window.go.main.App.ClearOutlookAccounts();
        if (result.error) {
          showToast(result.error, 'error');
          return;
        }
        showToast(_accT('accounts.allCleared', '已清空所有账号'));
        await loadOutlookAccountsList();
      } catch(e) {
        showToast(_accT('toast.clearFailed', '清空失败') + ': ' + e.message, 'error');
      }
    }
  );
}

function clearRegisteredOutlookAccounts() {
  var registered = outlookAllAccounts.filter(function(a) { return a.registered; }).length;
  if (!registered) {
    showToast(_accT('accounts.noRegistered', '没有已注册的账号'));
    return;
  }
  showConfirmModal(
    _accT('accounts.clearRegisteredTitle', '清除已注册'),
    _accT('accounts.clearRegisteredMsg', { n: registered }, '确认删除 {n} 个已注册（成功/失败）的账号？'),
    _accT('accounts.deleteConfirm', '确认删除'),
    async function() {
      try {
        var result = await window.go.main.App.ClearRegisteredOutlookAccounts();
        if (result.error) {
          showToast(result.error, 'error');
          return;
        }
        showToast(_accT('toast.accountsDeleted', { n: (result.removed || 0) }, '已删除 {n} 个账号'));
        await loadOutlookAccountsList();
      } catch(e) {
        showToast(_accT('toast.deleteFailed', '删除失败') + ': ' + e.message, 'error');
      }
    }
  );
}

function resetOutlookAccountStatuses() {
  var total = outlookAllAccounts.length;
  if (!total) {
    showToast('没有可重置的账号');
    return;
  }
  showConfirmModal('重置状态', '确认将所有微软邮箱状态重置为未注册？账号数据不会删除。', '确认重置', async function() {
    try {
      var result = await window.go.main.App.ResetOutlookAccountStatuses();
      if (result.error) {
        showToast(result.error, 'error');
        return;
      }
      showToast('已重置 ' + (result.reset || 0) + ' 个账号状态');
      await loadOutlookAccountsList();
    } catch(e) {
      showToast('重置失败: ' + e.message, 'error');
    }
  });
}

function openOutlookModal() {
  switchPage('accounts');
  loadOutlookAccountsList();
}

// ===== 自动刷新（停留在邮箱池页时每 3 秒刷新状态） =====
var outlookRefreshTimer = null;

function startOutlookAutoRefresh() {
  stopOutlookAutoRefresh();
  outlookRefreshTimer = setInterval(loadOutlookAccountsList, 3000);
}

function stopOutlookAutoRefresh() {
  if (outlookRefreshTimer) {
    clearInterval(outlookRefreshTimer);
    outlookRefreshTimer = null;
  }
}

// 语言切换后重新渲染表格行（状态/操作链接等动态文本）
window.addEventListener('i18n:changed', function() {
  try { if (typeof renderOutlookPage === 'function') renderOutlookPage(); } catch (e) {}
});
