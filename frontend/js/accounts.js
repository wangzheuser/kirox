// ===== Outlook 账号管理 =====

var outlookCurrentPage = 1;
var outlookPageSize = 10;
var outlookAllAccounts = [];
var outlookStatusFilter = 'all';
// 当前筛选条件下的全部结果（非分页），供全选/反选/批量删除使用
var outlookFilteredAccounts = [];
// 选中账号集合：以 email 为键当 Set 用，独立于渲染，避免 3 秒自动刷新重渲染时丢失选中
var outlookSelectedEmails = {};

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
    showToast('请先输入 Outlook 账号数据', 'error');
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
    showToast('成功添加 ' + result.added + ' 个账号，当前共 ' + result.total + ' 个');
  } catch(e) {
    showToast('添加失败: ' + e.message, 'error');
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
    showToast('成功导入 ' + result.added + ' 个账号，当前共 ' + result.total + ' 个');
  } catch(e) {
    showToast('导入失败: ' + e.message, 'error');
  }
}

async function loadOutlookAccountsList() {
  try {
    var accounts = await window.go.main.App.GetOutlookAccounts();
    outlookAllAccounts = accounts || [];
    renderOutlookPage();
  } catch(e) {
    console.error('加载账号列表失败:', e);
  }
}

// 状态筛选下拉各选项的中文标签
var outlookFilterLabels = { all: '全部', unregistered: '未注册', success: '成功', failed: '失败' };

function setOutlookStatusFilter(status) {
  outlookStatusFilter = status;
  outlookCurrentPage = 1;

  // 更新下拉框显示文案
  var labelEl = document.getElementById('outlook-filter-label');
  if (labelEl) labelEl.textContent = outlookFilterLabels[status] || '全部';

  // 更新下拉选项高亮，并收起下拉
  var dropdown = document.getElementById('outlook-filter-dropdown');
  if (dropdown) {
    dropdown.querySelectorAll('.dropdown-option').forEach(function(opt) {
      var optStatus = (opt.getAttribute('onclick') || '').indexOf("'" + status + "'") !== -1;
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
  if (outlookStatusFilter === 'unregistered') {
    accounts = accounts.filter(function(a) { return !a.registered; });
  } else if (outlookStatusFilter === 'success') {
    accounts = accounts.filter(function(a) { return a.registered && a.success; });
  } else if (outlookStatusFilter === 'failed') {
    accounts = accounts.filter(function(a) { return a.registered && !a.success; });
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
      var status = acc.registered ? (acc.success ? '成功' : '失败') : '未注册';
      var statusColor = acc.registered ? (acc.success ? 'var(--success)' : 'var(--danger)') : 'var(--text-muted)';
      var addedTime = acc.addedAt ? acc.addedAt.substring(5, 16) : '-';
      var checked = outlookSelectedEmails[acc.email] ? ' checked' : '';
      html += '<tr><td style="padding:8px 16px;"><input type="checkbox" class="outlook-row-cb" data-email="' + acc.email + '" onclick="toggleOutlookRow(this)"' + checked + '></td>';
      html += '<td>' + (globalIdx+1) + '</td><td>' + acc.email + '</td>';
      html += '<td style="color:' + statusColor + ';font-weight:600;">' + status + '</td>';
      html += '<td style="font-size:11px;color:var(--text-muted);font-family:var(--font-mono);">' + addedTime + '</td>';
      html += '<td style="text-align:right;"><a href="javascript:void(0)" onclick="deleteOutlookAccount(\'' + acc.email + '\')" style="color:var(--danger);">删除</a></td></tr>';
    });
    tbody.innerHTML = html;

    if (totalPages > 1) {
      pager.style.display = 'flex';
      document.getElementById('outlook-pager-info').textContent = '第 ' + outlookCurrentPage + ' / ' + totalPages + ' 页 (共 ' + total + ' 个)';
      document.getElementById('outlook-pager-prev').disabled = outlookCurrentPage <= 1;
      document.getElementById('outlook-pager-next').disabled = outlookCurrentPage >= totalPages;
    } else {
      pager.style.display = 'none';
    }
  } else {
    tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-muted);padding:20px;">暂无邮箱账号</td></tr>';
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
  showConfirmModal('删除账号', '确认删除账号 ' + email + ' ?', '确认删除', async function() {
    try {
      var result = await window.go.main.App.DeleteOutlookAccount(email);
      if (result.error) {
        showToast(result.error, 'error');
        return;
      }
      showToast('账号已删除');
      await loadOutlookAccountsList();
    } catch(e) {
      showToast('删除失败: ' + e.message, 'error');
    }
  });
}

function clearAllOutlookAccounts() {
  showConfirmModal('清空微软邮箱', '确认清空所有微软邮箱账号？此操作不可恢复！', '确认清空', async function() {
    try {
      var result = await window.go.main.App.ClearOutlookAccounts();
      if (result.error) {
        showToast(result.error, 'error');
        return;
      }
      showToast('已清空所有账号');
      await loadOutlookAccountsList();
    } catch(e) {
      showToast('清空失败: ' + e.message, 'error');
    }
  });
}

function clearRegisteredOutlookAccounts() {
  var registered = outlookAllAccounts.filter(function(a) { return a.registered; }).length;
  if (!registered) {
    showToast('没有已注册的账号');
    return;
  }
  showConfirmModal('清除已注册', '确认删除 ' + registered + ' 个已注册（成功/失败）的账号？', '确认删除', async function() {
    try {
      var result = await window.go.main.App.ClearRegisteredOutlookAccounts();
      if (result.error) {
        showToast(result.error, 'error');
        return;
      }
      showToast('已删除 ' + (result.removed || 0) + ' 个账号');
      await loadOutlookAccountsList();
    } catch(e) {
      showToast('删除失败: ' + e.message, 'error');
    }
  });
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
