// ===== UI工具：Toast / 窗口控制 / 主题 / 健康检查 / 邮箱提供商 =====

// Toast 通知
function showToast(msg, type) {
  // 容器
  var container = document.getElementById('toast-container');
  if (!container) {
    container = document.createElement('div');
    container.id = 'toast-container';
    document.body.appendChild(container);
  }

  var toast = document.createElement('div');
  toast.className = 'toast-item' + (type === 'error' ? ' toast-error' : ' toast-success');

  // 图标
  var icon = type === 'error'
    ? '<svg viewBox="0 0 24 24" class="toast-icon"><circle cx="12" cy="12" r="10"/><path d="M15 9l-6 6M9 9l6 6"/></svg>'
    : '<svg viewBox="0 0 24 24" class="toast-icon"><circle cx="12" cy="12" r="10"/><path d="M9 12l2 2 4-4"/></svg>';

  toast.innerHTML = icon + '<span class="toast-msg">' + msg + '</span>' +
    '<div class="toast-progress"><div class="toast-progress-bar"></div></div>';

  container.appendChild(toast);

  // 触发入场动画
  requestAnimationFrame(function() { toast.classList.add('show'); });

  // 自动消失
  setTimeout(function() {
    toast.classList.remove('show');
    toast.classList.add('hide');
    setTimeout(function() { toast.remove(); }, 400);
  }, 3000);
}

// 窗口控制
function closeApp() {
  try {
    if (window.runtime && window.runtime.Quit) { window.runtime.Quit(); }
    else { window.close(); }
  } catch (e) { console.error('关闭窗口失败:', e); }
}

function minimizeApp() {
  try {
    if (window.runtime && window.runtime.WindowMinimise) { window.runtime.WindowMinimise(); }
  } catch (e) { console.error('最小化窗口失败:', e); }
}

function maximizeApp() {
  try {
    if (window.runtime && window.runtime.WindowToggleMaximise) { window.runtime.WindowToggleMaximise(); }
  } catch (e) { console.error('最大化窗口失败:', e); }
}

// 主题切换（View Transition 圆形扩展动画）
function toggleTheme(e) {
  // 注入样式禁用所有 transition，防止主题切换闪烁
  var lockStyle = document.createElement('style');
  lockStyle.textContent = '*, *::before, *::after { transition-duration: 0s !important; }';
  document.head.appendChild(lockStyle);

  var applyTheme = function() {
    var html = document.documentElement;
    var isDark = html.getAttribute('data-theme') === 'dark';
    if (isDark) {
      html.removeAttribute('data-theme');
      localStorage.setItem('kiro-theme', 'light');
      document.getElementById('theme-icon-light').style.display = '';
      document.getElementById('theme-icon-dark').style.display = 'none';
    } else {
      html.setAttribute('data-theme', 'dark');
      localStorage.setItem('kiro-theme', 'dark');
      document.getElementById('theme-icon-light').style.display = 'none';
      document.getElementById('theme-icon-dark').style.display = '';
    }
  };

  var unlockTransitions = function() {
    setTimeout(function() { lockStyle.remove(); }, 100);
  };

  // 不支持 View Transition 时直接切换
  if (!document.startViewTransition) {
    applyTheme();
    unlockTransitions();
    return;
  }

  var transition = document.startViewTransition(applyTheme);
  transition.finished.then(unlockTransitions);
  transition.ready.then(function() {
    var clientX = 0;
    var clientY = innerHeight;
    var radius = Math.hypot(
      Math.max(clientX, innerWidth - clientX),
      Math.max(clientY, innerHeight - clientY)
    );
    document.documentElement.animate(
      { clipPath: [
        'circle(0% at ' + clientX + 'px ' + clientY + 'px)',
        'circle(' + radius + 'px at ' + clientX + 'px ' + clientY + 'px)'
      ]},
      {
        duration: 500,
        easing: 'ease-in-out',
        pseudoElement: '::view-transition-new(root)'
      }
    );
  });
}

// 恢复主题
(function() {
  var saved = localStorage.getItem('kiro-theme');
  if (saved === 'dark') {
    document.documentElement.setAttribute('data-theme', 'dark');
    var light = document.getElementById('theme-icon-light');
    var dark = document.getElementById('theme-icon-dark');
    if (light) light.style.display = 'none';
    if (dark) dark.style.display = '';
  }
})();

// 快捷键
document.addEventListener('keydown', function(e) {
  // Ctrl+Enter 开始任务
  if (e.ctrlKey && e.key === 'Enter') {
    e.preventDefault();
    var startBtn = document.getElementById('reg-btn-start');
    if (startBtn && !startBtn.disabled) startTask();
  }
  // Esc 停止任务
  if (e.key === 'Escape') {
    var stopButtons = ['ov-btn-stop', 'reg-btn-stop'].map(function(id) {
      return document.getElementById(id);
    });
    if (stopButtons.some(function(btn) { return btn && !btn.disabled; })) stopTask();
  }
});

// 当前选中的邮箱提供商（多选）
var selectedEmailProviders = ['outlook'];
var emailProviderNames = ['outlook', 'moemail', 'mailporary', 'cloudmail', 'emailnator', 'inboxes', 'mailgw', 'mailtm', 'tempmail_lol', 'guerrillamail', 'inboxkitten', 'freecustom', 'fce_areueally', 'fce_junkstopper', 'fce_ditpay', 'dropmail', 'mailcatch', 'tempmailo', 'minuteinbox', 'smailpro', 'tempmailbox', 'generator_email', 'mailtowin', 'mail2me', 'pickmemail', 'maximail', 'emlpro', 'freeml', 'emlhub', 'emltmp', 'mailpwr', 'tenmail', 'dropmail_me', 'mimimail', 'pickmail', 'spymail', 'yomail', 'tmio_bltiwd', 'tmio_wnbaldwy', 'tmio_bwmyga', 'tmio_ozsaip', 'gonebox', 'openinbox', 'blinkbox'];
var selectedMoeMailDomains = [];
var allMoeMailDomains = []; // 存储所有可用域名及其配置映射
var selectedCloudMailDomains = [];
var allCloudMailDomains = []; // 存储所有 cloud-mail 域名及对应配置

// HTML 转义函数
function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

// 初始化邮箱提供商选择（页面加载时调用）
function initEmailProviderSelection() {
  if (typeof registrationConfigLoaded !== 'undefined' && registrationConfigLoaded) return;
  setEmailProviders(['outlook'], { skipSave: true });
}

function normalizeEmailProviders(providers) {
  const input = Array.isArray(providers) ? providers : [providers];
  const out = [];
  input.forEach(function(provider) {
    provider = String(provider || '').trim();
    if (!provider || !emailProviderNames.includes(provider) || out.includes(provider)) return;
    out.push(provider);
  });
  return out.length ? out : ['outlook'];
}

function getSelectedEmailProviders() {
  return normalizeEmailProviders(selectedEmailProviders).slice();
}

function setEmailProviders(providers, options) {
  options = options || {};
  selectedEmailProviders = normalizeEmailProviders(providers);

  // 更新按钮样式
  emailProviderNames.forEach(function(name) {
    const btn = document.querySelector('label[onclick*="' + name + '"]');
    const input = document.querySelector('input[name="email-provider"][value="' + name + '"]');
    if (!btn) return;
    const active = selectedEmailProviders.includes(name);
    btn.classList.toggle('is-active', active);
    if (input) input.checked = active;
  });

  // 显示/隐藏配置块
  const moemailConfigDiv = document.getElementById('moemail-config-select');
  const cloudmailConfigDiv = document.getElementById('cloudmail-config-select');

  if (moemailConfigDiv) moemailConfigDiv.style.display = selectedEmailProviders.includes('moemail') ? 'block' : 'none';
  if (cloudmailConfigDiv) cloudmailConfigDiv.style.display = selectedEmailProviders.includes('cloudmail') ? 'block' : 'none';
  if (selectedEmailProviders.includes('moemail')) {
    loadMoeMailDomainsToList(options.moemailSelection);
  }
  if (selectedEmailProviders.includes('cloudmail')) {
    loadCloudMailDomainsToList();
  }
  updateEmailProviderHint(selectedEmailProviders[selectedEmailProviders.length - 1], options);
  if (!options.skipSave && typeof scheduleRegistrationConfigSave === 'function') {
    scheduleRegistrationConfigSave();
  }
}

function toggleEmailProvider(provider, options) {
  options = options || {};
  const current = getSelectedEmailProviders();
  const exists = current.includes(provider);
  if (exists && current.length === 1) {
    if (typeof showToast === 'function') showToast('至少选择一个邮箱渠道', 'error');
    setEmailProviders(current, { skipSave: true });
    return;
  }
  const next = exists ? current.filter(item => item !== provider) : current.concat(provider);
  setEmailProviders(next, options);
}

function updateEmailProviderHint(provider, options) {
  options = options || {};
  const hintDiv = document.getElementById('email-provider-hint');
  if (!hintDiv) return;
  if (selectedEmailProviders.length > 1) {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = '已选择 ' + selectedEmailProviders.length + ' 个邮箱渠道，注册时会按渠道轮询。';
    return;
  }

  if (provider === 'moemail') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.moemailHint', '使用 MoeMail 临时邮箱进行注册，每次任务会自动生成新邮箱。');
    hintDiv.setAttribute('data-i18n', 'register.moemailHint');
  } else if (provider === 'mailporary') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.mailporaryHint', '使用 Mailporary 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.mailporaryHint');
  } else if (provider === 'emailnator') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.emailnatorHint', '使用 Emailnator Gmail 风格临时邮箱进行注册，零配置。');
    hintDiv.setAttribute('data-i18n', 'register.emailnatorHint');
  } else if (provider === 'inboxes') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.inboxesHint', '使用 Inboxes 多域名临时邮箱进行注册，零配置。');
    hintDiv.setAttribute('data-i18n', 'register.inboxesHint');
  } else if (provider === 'mailgw') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.mailgwHint', '使用 mail.gw 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.mailgwHint');
  } else if (provider === 'mailtm') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.mailtmHint', '使用 mail.tm 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.mailtmHint');
  } else if (provider === 'tempmail_lol') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.tempmailLolHint', '使用 TempMail.lol 随机临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.tempmailLolHint');
  } else if (provider === 'guerrillamail') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.guerrillaMailHint', '使用 GuerrillaMail 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.guerrillaMailHint');
  } else if (provider === 'inboxkitten') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.inboxKittenHint', '使用 InboxKitten 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.inboxKittenHint');
  } else if (provider === 'freecustom') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.freeCustomHint', '使用 FreeCustom.Email 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.freeCustomHint');
  } else if (provider === 'fce_ditpay') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.fceDitpayHint', '使用 ditpay.info 固定域名的 FreeCustom.Email 临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.fceDitpayHint');
  } else if (provider === 'fce_junkstopper') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.fceJunkstopperHint', '使用 junkstopper.info 固定域名的 FreeCustom.Email 临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.fceJunkstopperHint');
  } else if (provider === 'fce_areueally') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.fceAreueallyHint', '使用 areueally.info 固定域名的 FreeCustom.Email 临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.fceAreueallyHint');
  } else if (provider === 'dropmail') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.dropMailHint', '使用 DropMail 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.dropMailHint');
  } else if (provider === 'mailcatch') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.mailCatchHint', '使用 MailCatch 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.mailCatchHint');
  } else if (provider === 'tempmailo') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.tempMailoHint', '使用 TempMailo 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.tempMailoHint');
  } else if (provider === 'minuteinbox') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.minuteInboxHint', '使用 MinuteInbox 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.minuteInboxHint');
  } else if (provider === 'smailpro') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.smailProHint', '使用 SmailPro 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.smailProHint');
  } else if (provider === 'tempmailbox') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.tempMailboxHint', '使用 TempMailbox 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.tempMailboxHint');
  } else if (provider === 'generator_email') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.generatorEmailHint', '使用 Generator.Email 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.generatorEmailHint');
  } else if (provider === 'mailtowin') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.mailToWinHint', '使用 MailToWin 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.mailToWinHint');
  } else if (provider === 'mail2me') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.mail2MeHint', '使用 Mail2Me 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.mail2MeHint');
  } else if (provider === 'pickmemail') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.pickMeMailHint', '使用 PickMeMail 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.pickMeMailHint');
  } else if (provider === 'maximail') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.maxiMailHint', '使用 MaxiMail 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.maxiMailHint');
  } else if (provider === 'emlpro') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.emlProHint', '使用 EmlPro 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.emlProHint');
  } else if (provider === 'freeml') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.freeMLHint', '使用 FreeML 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.freeMLHint');
  } else if (provider === 'emlhub') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.emlHubHint', '使用 EmlHub 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.emlHubHint');
  } else if (provider === 'emltmp') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.emlTmpHint', '使用 EmlTmp 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.emlTmpHint');
  } else if (provider === 'mailpwr') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.mailPwrHint', '使用 MailPwr 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.mailPwrHint');
  } else if (provider === 'tenmail') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.tenMailHint', '使用 10Mail 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.tenMailHint');
  } else if (provider === 'dropmail_me') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.dropMailMeHint', '使用 DropMail.me 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.dropMailMeHint');
  } else if (provider === 'mimimail') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.mimiMailHint', '使用 MimiMail 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.mimiMailHint');
  } else if (provider === 'pickmail') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.pickMailHint', '使用 PickMail 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.pickMailHint');
  } else if (provider === 'spymail') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.spyMailHint', '使用 SpyMail 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.spyMailHint');
  } else if (provider === 'yomail') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.yoMailHint', '使用 YoMail 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.yoMailHint');
  } else if (provider === 'tmio_bltiwd') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.tmioBltiwdHint', '使用 TempMailIO bltiwd.com 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.tmioBltiwdHint');
  } else if (provider === 'tmio_wnbaldwy') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.tmioWnbaldwyHint', '使用 TempMailIO wnbaldwy.com 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.tmioWnbaldwyHint');
  } else if (provider === 'tmio_bwmyga') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.tmioBwmygaHint', '使用 TempMailIO bwmyga.com 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.tmioBwmygaHint');
  } else if (provider === 'tmio_ozsaip') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.tmioOzsaipHint', '使用 TempMailIO ozsaip.com 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.tmioOzsaipHint');
  } else if (provider === 'gonebox') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.goneBoxHint', '使用 GoneBox 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.goneBoxHint');
  } else if (provider === 'openinbox') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.openInboxHint', '使用 OpenInbox 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.openInboxHint');
  } else if (provider === 'blinkbox') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.blinkBoxHint', '使用 BlinkBoxApp 零配置临时邮箱进行注册。');
    hintDiv.setAttribute('data-i18n', 'register.blinkBoxHint');
  } else if (provider === 'cloudmail') {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.cloudmailHint', '使用 Cloud-Mail 自部署邮箱注册。⚠️ 每次注册会创建永久账号，需手动清理。');
    hintDiv.setAttribute('data-i18n', 'register.cloudmailHint');
  } else {
    hintDiv.removeAttribute('data-i18n');
    hintDiv.textContent = _uiT('register.outlookHintFull', '使用微软邮箱进行注册，代理配置请在设置页设置。');
    hintDiv.setAttribute('data-i18n', 'register.outlookHintFull');
  }

}

function _uiT(key, fallback) {
  if (window.I18N && typeof window.I18N.t === 'function') {
    var v = window.I18N.t(key);
    if (v && v !== key) return v;
  }
  return fallback;
}

// 加载 MoeMail 域名到列表
async function loadMoeMailDomainsToList(preferredSelection) {
  const listDiv = document.getElementById('cfg-moemail-domains-list');
  if (!listDiv) return;

  listDiv.innerHTML = '<div style="text-align:center;color:var(--text-muted);font-size:12px;padding:12px;">' + _uiT('common.loading', '加载中...') + '</div>';

  try {
    const configs = await window.go.main.App.GetMoeMailConfigs();

    if (!configs || configs.length === 0) {
      listDiv.innerHTML = '<div style="text-align:center;color:var(--text-muted);font-size:12px;padding:12px;">' + _uiT('moemail.noDomainsHint', '暂无配置，请先在设置页添加') + '</div>';
      return;
    }

    let configStatus = {};
    try {
      const saved = localStorage.getItem('moemail-config-status');
      if (saved) configStatus = JSON.parse(saved);
    } catch (e) {}

    allMoeMailDomains = [];
    const domainConfigMap = {};

    for (const cfg of configs) {
      const status = configStatus[cfg.name];
      if (status && status.tested && status.success && status.domains && status.domains.length > 0) {
        for (const domain of status.domains) {
          if (!domainConfigMap[domain]) domainConfigMap[domain] = [];
          domainConfigMap[domain].push(cfg);
        }
      }
    }

    allMoeMailDomains = Object.keys(domainConfigMap).map(domain => ({
      domain: domain,
      configs: domainConfigMap[domain]
    }));

    if (allMoeMailDomains.length === 0) {
      listDiv.innerHTML = '<div style="text-align:center;color:var(--text-muted);font-size:12px;padding:12px;">暂无可用域名，请先测试配置</div>';
      return;
    }

    let html = `
      <div class="domain-mode-row">
        <div class="domain-mode-btn selected" data-domain="__random__" onclick="toggleMoeMailDomain('__random__')">
          <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/><polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/><line x1="4" y1="4" x2="9" y2="9"/></svg>
          随机
        </div>
        <div class="domain-mode-btn" data-domain="__all__" onclick="toggleMoeMailDomain('__all__')">
          <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 014-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 01-4 4H3"/></svg>
          轮询
        </div>
      </div>
      <div class="domain-chips-wrap">
    `;

    html += allMoeMailDomains.map((item) => {
      return `<div class="domain-chip" data-domain="${escapeHtml(item.domain)}" onclick="toggleMoeMailDomain('${escapeHtml(item.domain)}')" title="${item.configs.length} 个配置">${escapeHtml(item.domain)}</div>`;
    }).join('');

    html += '</div>';

    listDiv.innerHTML = html;
    selectedMoeMailDomains = normalizePreferredMoeMailSelection(preferredSelection);
    updateDomainOptionStyles();

  } catch (e) {
    console.error('加载 MoeMail 域名失败:', e);
    listDiv.innerHTML = '<div style="text-align:center;color:var(--danger);font-size:12px;padding:12px;">加载失败</div>';
  }
}

function normalizePreferredMoeMailSelection(preferredSelection) {
  const available = new Set((allMoeMailDomains || []).map(item => item.domain));
  const preferred = Array.isArray(preferredSelection) && preferredSelection.length
    ? preferredSelection
    : selectedMoeMailDomains;
  if (preferred && preferred.includes('__all__')) return ['__all__'];
  if (preferred && preferred.includes('__random__')) return ['__random__'];
  const custom = (preferred || []).filter(domain => available.has(domain));
  return custom.length ? custom : ['__random__'];
}

// 更新域名选项的视觉状态
function updateDomainOptionStyles() {
  document.querySelectorAll('.domain-mode-btn').forEach(el => {
    const domain = el.getAttribute('data-domain');
    el.classList.toggle('selected', selectedMoeMailDomains.includes(domain));
  });
  document.querySelectorAll('.domain-chip').forEach(el => {
    const domain = el.getAttribute('data-domain');
    el.classList.toggle('selected', selectedMoeMailDomains.includes(domain));
  });
}

// 切换域名选择
function toggleMoeMailDomain(domain, el) {
  const isSelected = selectedMoeMailDomains.includes(domain);

  if (domain === '__random__' || domain === '__all__') {
    if (isSelected) {
      selectedMoeMailDomains = selectedMoeMailDomains.filter(d => d !== domain);
    } else {
      selectedMoeMailDomains = [domain];
    }
  } else {
    // 点击具体域名：先清除 __random__ 和 __all__
    selectedMoeMailDomains = selectedMoeMailDomains.filter(d => d !== '__random__' && d !== '__all__');
    if (isSelected) {
      selectedMoeMailDomains = selectedMoeMailDomains.filter(d => d !== domain);
    } else {
      selectedMoeMailDomains.push(domain);
    }
  }

  updateDomainOptionStyles();
  if (typeof scheduleRegistrationConfigSave === 'function') {
    scheduleRegistrationConfigSave();
  }
}

// 全选域名
function selectAllMoeMailDomains() {
  selectedMoeMailDomains = allMoeMailDomains.map(item => item.domain);
  updateDomainOptionStyles();
  if (typeof scheduleRegistrationConfigSave === 'function') {
    scheduleRegistrationConfigSave();
  }
}

// ===== Cloud-Mail 域名加载/选择 =====
async function loadCloudMailDomainsToList() {
  const listDiv = document.getElementById('cfg-cloudmail-domains-list');
  if (!listDiv) return;

  listDiv.innerHTML = '<div style="text-align:center;color:var(--text-muted);font-size:12px;padding:12px;">' + _uiT('common.loading', '加载中...') + '</div>';

  try {
    const configs = await window.go.main.App.GetCloudMailConfigs();
    if (!configs || configs.length === 0) {
      listDiv.innerHTML = '<div style="text-align:center;color:var(--text-muted);font-size:12px;padding:12px;">' + _uiT('cloudmail.noDomainsHint', '暂无配置，请先在邮箱池页添加') + '</div>';
      return;
    }

    let configStatus = {};
    try {
      const saved = localStorage.getItem('cloudmail-config-status');
      if (saved) configStatus = JSON.parse(saved);
    } catch (e) {}

    allCloudMailDomains = [];
    const domainConfigMap = {};

    for (const cfg of configs) {
      const status = configStatus[cfg.name];
      // 只展示已通过测试的配置（与 moemail 一致）
      if (!status || !status.tested || !status.success) continue;
      // 优先用服务器自动拉到的域名，否则回退到配置里手填的
      const domains = (status.domains && status.domains.length > 0) ? status.domains : (cfg.domains || []);
      for (const domain of domains) {
        if (!domainConfigMap[domain]) domainConfigMap[domain] = [];
        domainConfigMap[domain].push(cfg);
      }
    }

    allCloudMailDomains = Object.keys(domainConfigMap).map(domain => ({
      domain: domain,
      configs: domainConfigMap[domain]
    }));

    if (allCloudMailDomains.length === 0) {
      listDiv.innerHTML = '<div style="text-align:center;color:var(--text-muted);font-size:12px;padding:12px;">' + _uiT('cloudmail.noActiveDomain', '暂无可用域名，请先测试 Cloud-Mail 配置') + '</div>';
      return;
    }

    let html = `
      <div class="domain-mode-row">
        <div class="domain-mode-btn selected" data-domain="__random__" onclick="toggleCloudMailDomain('__random__')">
          <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/><polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/><line x1="4" y1="4" x2="9" y2="9"/></svg>
          ${_uiT('register.modeRandom', '随机')}
        </div>
        <div class="domain-mode-btn" data-domain="__all__" onclick="toggleCloudMailDomain('__all__')">
          <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 014-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 01-4 4H3"/></svg>
          ${_uiT('register.modeRoundRobin', '轮询')}
        </div>
      </div>
      <div class="domain-chips-wrap">
    `;

    html += allCloudMailDomains.map((item) => {
      return `<div class="domain-chip" data-domain="${escapeHtml(item.domain)}" onclick="toggleCloudMailDomain('${escapeHtml(item.domain)}')" title="${item.configs.length} 个配置">${escapeHtml(item.domain)}</div>`;
    }).join('');

    html += '</div>';
    listDiv.innerHTML = html;
    selectedCloudMailDomains = ['__random__'];
    updateCloudMailDomainStyles();
  } catch (e) {
    console.error('加载 Cloud-Mail 域名失败:', e);
    listDiv.innerHTML = '<div style="text-align:center;color:var(--danger);font-size:12px;padding:12px;">加载失败</div>';
  }
}

function updateCloudMailDomainStyles() {
  const container = document.getElementById('cfg-cloudmail-domains-list');
  if (!container) return;
  container.querySelectorAll('.domain-mode-btn').forEach(el => {
    const d = el.getAttribute('data-domain');
    el.classList.toggle('selected', selectedCloudMailDomains.includes(d));
  });
  container.querySelectorAll('.domain-chip').forEach(el => {
    const d = el.getAttribute('data-domain');
    el.classList.toggle('selected', selectedCloudMailDomains.includes(d));
  });
}

function toggleCloudMailDomain(domain) {
  const isSelected = selectedCloudMailDomains.includes(domain);
  if (domain === '__random__' || domain === '__all__') {
    if (isSelected) {
      selectedCloudMailDomains = selectedCloudMailDomains.filter(d => d !== domain);
    } else {
      selectedCloudMailDomains = [domain];
    }
  } else {
    selectedCloudMailDomains = selectedCloudMailDomains.filter(d => d !== '__random__' && d !== '__all__');
    if (isSelected) {
      selectedCloudMailDomains = selectedCloudMailDomains.filter(d => d !== domain);
    } else {
      selectedCloudMailDomains.push(domain);
    }
  }
  updateCloudMailDomainStyles();
}

function selectAllCloudMailDomains() {
  selectedCloudMailDomains = allCloudMailDomains.map(item => item.domain);
  updateCloudMailDomainStyles();
}
