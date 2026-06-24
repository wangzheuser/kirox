import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const proxyPoolJs = fs.readFileSync(new URL('../js/proxy_pool.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');
const wailsAppJs = fs.readFileSync(new URL('../wailsjs/go/main/App.js', import.meta.url), 'utf8');
const wailsAppDts = fs.readFileSync(new URL('../wailsjs/go/main/App.d.ts', import.meta.url), 'utf8');

test('settings page sound switch is persisted through backend APIs', () => {
  assert.match(html, /id="cfg-sound"/);
  assert.match(appJs, /GetSoundEnabled\(\)/);
  assert.match(appJs, /SetSoundEnabled\(/);
  assert.doesNotMatch(appJs, /localStorage\.setItem\(['"]kiro-sound['"]/);
  assert.doesNotMatch(uiJs, /localStorage\.setItem\(['"]kiro-sound['"]/);
});

test('verify models setting and frontend API calls are removed', () => {
  assert.doesNotMatch(html, /id="cfg-verify-models"/);
  assert.doesNotMatch(html, /二次模型验活/);
  assert.doesNotMatch(appJs, /loadVerifyModelsEnabled|saveVerifyModelsEnabled/);
  assert.doesNotMatch(appJs, /GetVerifyModelsEnabled|SetVerifyModelsEnabled/);
});

test('registration form is persisted through backend APIs instead of long-term localStorage', () => {
  assert.match(html, /id="cfg-success-target"/);
  assert.match(html, /id="cfg-reuse-failed-email"/);
  assert.match(appJs, /GetRegistrationConfig\(\)/);
  assert.match(appJs, /SetRegistrationConfig\(/);
  assert.match(appJs, /successTarget:\s*readIntegerInput\(['"]cfg-success-target['"]/);
  assert.match(appJs, /reuseFailedEmail:\s*readCheckboxInput\(['"]cfg-reuse-failed-email['"]/);
  assert.match(appJs, /emailProviders:\s*getSelectedEmailProviders\(\)/);
  assert.doesNotMatch(appJs, /emailProvider:\s*selectedEmailProvider/);
  assert.match(appJs, /writeIntegerInput\(['"]cfg-success-target['"]\s*,\s*cfg\.successTarget/);
  assert.match(appJs, /writeCheckboxInput\(['"]cfg-reuse-failed-email['"]\s*,\s*cfg\.reuseFailedEmail/);
  assert.match(appJs, /\['cfg-count',\s*'cfg-success-target'/);
  assert.match(appJs, /\['cfg-reuse-failed-email'\]/);
  assert.doesNotMatch(appJs, /localStorage\.setItem\(['"]kiro-config['"]/);
  assert.doesNotMatch(appJs, /cfg\.delay\s*\|\|/);
  assert.doesNotMatch(appJs, /migrateLegacyRegistrationConfigIfNeeded/);
});

test('registration email providers are multi-select only', () => {
  assert.doesNotMatch(html, /type="radio"\s+name="email-provider"/);
  assert.match(html, /type="checkbox"\s+name="email-provider"/);
  assert.match(html, /onclick="toggleEmailProvider\('outlook'\);\s*return false;"/);
  assert.match(uiJs, /var selectedEmailProviders = \['outlook'\]/);
  assert.match(uiJs, /function setEmailProviders\(providers, options\)/);
  assert.match(uiJs, /function toggleEmailProvider\(provider, options\)/);
  assert.match(uiJs, /selectedEmailProviders\.includes\('moemail'\)/);
  assert.match(uiJs, /selectedEmailProviders\.includes\('cloudmail'\)/);
  assert.match(appJs, /cfg\.emailProviders/);
  assert.doesNotMatch(appJs, /cfg\.emailProvider\b/);
});

test('wails wrappers expose persistent config APIs', () => {
  for (const name of ['GetSoundEnabled', 'SetSoundEnabled', 'GetRegistrationConfig', 'SetRegistrationConfig', 'GetKiroRSConfig', 'SetKiroRSConfig', 'TestKiroRSConnection', 'GetOutlookGraphRegistrationEmailMode', 'SetOutlookGraphRegistrationEmailMode']) {
    assert.match(wailsAppJs, new RegExp(`export function ${name}\\(`));
    assert.match(wailsAppDts, new RegExp(`export function ${name}\\(`));
  }
  for (const name of ['GetVerifyModelsEnabled', 'SetVerifyModelsEnabled']) {
    assert.doesNotMatch(wailsAppJs, new RegExp(`export function ${name}\\(`));
    assert.doesNotMatch(wailsAppDts, new RegExp(`export function ${name}\\(`));
  }
});

test('settings cfg controls have backend persistence coverage or explicit non-setting ownership', () => {
  const settingsStart = html.indexOf('id="page-settings"');
  const settingsEnd = html.indexOf('id="page-logs"', settingsStart);
  const settingsHtml = html.slice(settingsStart, settingsEnd);
  const cfgIds = [...settingsHtml.matchAll(/id="(cfg-[^"]+)"/g)].map(match => match[1]).sort();
  const expected = [
    'cfg-clash-api-secret',
    'cfg-clash-api-url',
    'cfg-clash-proxy',
    'cfg-clash-proxy-group',
    'cfg-clash-skip-test',
    'cfg-clash-test-timeout',
    'cfg-clash-test-url',
    'cfg-data-dir',
    'cfg-email-proxy',
    'cfg-kill-switch',
    'cfg-kiro-rs-auto-sync',
    'cfg-kiro-rs-key',
    'cfg-kiro-rs-url',
    'cfg-outlook-graph-registration-email-mode',
    'cfg-outlook-scope',
    'cfg-proxy',
    'cfg-proxy-mode-clash',
    'cfg-proxy-mode-none',
    'cfg-proxy-mode-normal',
    'cfg-proxy-mode-pool',
    'cfg-result-output-dir',
    'cfg-sound',
  ];
  assert.deepEqual(cfgIds, expected);
});


test('page stay settings and APIs are removed', () => {
  assert.doesNotMatch(html, new RegExp('cfg-page-' + 'stay'));
  assert.doesNotMatch(html, new RegExp('模拟页面' + '停留'));
  assert.doesNotMatch(appJs, new RegExp('Page' + 'Stay|' + 'page' + 'Stay|' + 'cfg-page-' + 'stay'));
  assert.doesNotMatch(wailsAppJs, new RegExp('Page' + 'Stay'));
  assert.doesNotMatch(wailsAppDts, new RegExp('Page' + 'Stay'));
});

test('proxy pool is an independent proxy mode panel', () => {
  assert.match(html, /id="cfg-proxy-mode-pool"[^>]*value="pool"/);
  assert.match(html, /id="proxy-pool-panel"/);
  assert.match(appJs, /pool:\s*'多代理池'/);
  assert.match(appJs, /mode === 'pool'/);

  const normalPanelStart = html.indexOf('id="proxy-normal-panel"');
  const poolPanelStart = html.indexOf('id="proxy-pool-panel"');
  const clashPanelStart = html.indexOf('id="proxy-clash-panel"');
  assert.ok(normalPanelStart >= 0, 'proxy-normal-panel should exist');
  assert.ok(poolPanelStart >= 0, 'proxy-pool-panel should exist');
  assert.ok(clashPanelStart >= 0, 'proxy-clash-panel should exist');
  assert.ok(normalPanelStart < poolPanelStart, 'pool panel should follow normal panel');
  assert.ok(poolPanelStart < clashPanelStart, 'pool panel should be independent before clash panel');
});

test('proxy examples and visible proxy inputs are sanitized', () => {
  for (const source of [html, proxyPoolJs, i18nJs]) {
    assert.doesNotMatch(source, new RegExp('Default\\.\\{uuid\\}:admin' + '2012', 'i'));
    assert.doesNotMatch(source, new RegExp('resin-' + 'proxy\\.codeai\\.de5\\.net', 'i'));
    assert.doesNotMatch(source, /http:\/\/user:pass@/i);
  }

  assert.match(html, /<input(?=[^>]*id="cfg-proxy")(?=[^>]*type="password")[^>]*>/);
  assert.match(html, /<input(?=[^>]*id="cfg-email-proxy")(?=[^>]*type="password")[^>]*>/);
  assert.match(html, /<input(?=[^>]*id="cfg-clash-api-secret")(?=[^>]*type="password")[^>]*>/);
  assert.match(html, /https:\/\/user\.\{uuid\}:\*\*\*@proxy\.example\.com:443/);
  assert.match(proxyPoolJs, /https:\/\/user\.\{uuid\}:\*\*\*@proxy\.example\.com:443/);
  assert.match(proxyPoolJs, /type="password" value="' \+ escapeProxyHtml\(p\.url\)/);
});

test('Outlook Graph registration email strategy is persisted through backend APIs', () => {
  assert.match(html, /id="cfg-outlook-graph-registration-email-mode"/);
  assert.match(appJs, /loadOutlookGraphRegistrationEmailMode/);
  assert.match(appJs, /saveOutlookGraphRegistrationEmailMode/);
  assert.match(appJs, /GetOutlookGraphRegistrationEmailMode/);
  assert.match(appJs, /SetOutlookGraphRegistrationEmailMode/);
  assert.match(wailsAppJs, /export function GetOutlookGraphRegistrationEmailMode\(\)/);
  assert.match(wailsAppJs, /export function SetOutlookGraphRegistrationEmailMode\(arg1\)/);
  assert.match(wailsAppDts, /export function GetOutlookGraphRegistrationEmailMode\(\):Promise<string>/);
  assert.match(wailsAppDts, /export function SetOutlookGraphRegistrationEmailMode\(arg1:string\):Promise<Record<string, any>>/);
});

test('registration OTP timeout defaults to sixty seconds in UI config', () => {
  assert.match(html, /id="cfg-otp-timeout" value="60"/);
  assert.match(appJs, /otpTimeout:\s*readIntegerInput\(['"]cfg-otp-timeout['"],\s*60,\s*30\)/);
  assert.match(appJs, /writeIntegerInput\(['"]cfg-otp-timeout['"],\s*cfg\.otpTimeout,\s*60\)/);
  assert.match(appJs, /otpTimeout:\s*60/);
});
