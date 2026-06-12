import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const proxyPoolJs = fs.readFileSync(new URL('../js/proxy_pool.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const wailsAppJs = fs.readFileSync(new URL('../wailsjs/go/main/App.js', import.meta.url), 'utf8');
const wailsAppDts = fs.readFileSync(new URL('../wailsjs/go/main/App.d.ts', import.meta.url), 'utf8');

test('settings page sound switch is persisted through backend APIs', () => {
  assert.match(html, /id="cfg-sound"/);
  assert.match(html, /id="cfg-verify-models"/);
  assert.match(appJs, /GetSoundEnabled\(\)/);
  assert.match(appJs, /SetSoundEnabled\(/);
  assert.doesNotMatch(appJs, /localStorage\.setItem\(['"]kiro-sound['"]/);
  assert.doesNotMatch(uiJs, /localStorage\.setItem\(['"]kiro-sound['"]/);
});

test('registration form is persisted through backend APIs instead of long-term localStorage', () => {
  assert.match(html, /id="cfg-success-target"/);
  assert.match(html, /id="cfg-reuse-failed-email"/);
  assert.match(appJs, /GetRegistrationConfig\(\)/);
  assert.match(appJs, /SetRegistrationConfig\(/);
  assert.match(appJs, /successTarget:\s*readIntegerInput\(['"]cfg-success-target['"]/);
  assert.match(appJs, /reuseFailedEmail:\s*readCheckboxInput\(['"]cfg-reuse-failed-email['"]/);
  assert.match(appJs, /writeIntegerInput\(['"]cfg-success-target['"]\s*,\s*cfg\.successTarget/);
  assert.match(appJs, /writeCheckboxInput\(['"]cfg-reuse-failed-email['"]\s*,\s*cfg\.reuseFailedEmail/);
  assert.match(appJs, /\['cfg-count',\s*'cfg-success-target'/);
  assert.match(appJs, /\['cfg-reuse-failed-email'\]/);
  assert.doesNotMatch(appJs, /localStorage\.setItem\(['"]kiro-config['"]/);
  assert.doesNotMatch(appJs, /cfg\.delay\s*\|\|/);
  assert.match(appJs, /localStorage\.removeItem\(['"]kiro-config['"]\)/);
});

test('wails wrappers expose persistent config APIs', () => {
  for (const name of ['GetSoundEnabled', 'SetSoundEnabled', 'GetVerifyModelsEnabled', 'SetVerifyModelsEnabled', 'GetRegistrationConfig', 'SetRegistrationConfig', 'GetKiroRSConfig', 'SetKiroRSConfig', 'TestKiroRSConnection']) {
    assert.match(wailsAppJs, new RegExp(`export function ${name}\\(`));
    assert.match(wailsAppDts, new RegExp(`export function ${name}\\(`));
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
    'cfg-outlook-scope',
    'cfg-page-stay-max-seconds',
    'cfg-page-stay-min-seconds',
    'cfg-proxy',
    'cfg-proxy-mode-clash',
    'cfg-proxy-mode-none',
    'cfg-proxy-mode-normal',
    'cfg-proxy-mode-pool',
    'cfg-result-output-dir',
    'cfg-sound',
    'cfg-verify-models',
  ];
  assert.deepEqual(cfgIds, expected);
});

test('proxy pool is an independent proxy mode panel', () => {
  assert.match(html, /id="cfg-proxy-mode-pool"[^>]*value="pool"/);
  assert.match(html, /id="proxy-pool-panel"/);
  assert.match(html, /https:\/\/Default\.\{uuid\}:admin2012@resin-proxy\.codeai\.de5\.net:443/);
  assert.match(appJs, /pool:\s*'多代理池'/);
  assert.match(proxyPoolJs, /https:\/\/Default\.\{uuid\}:admin2012@resin-proxy\.codeai\.de5\.net:443/);
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
