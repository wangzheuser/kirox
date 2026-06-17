import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const accountPoolJs = fs.readFileSync(new URL('../js/account_pool.js', import.meta.url), 'utf8');
const buildJs = fs.readFileSync(new URL('../build.js', import.meta.url), 'utf8');
const wailsAppJs = fs.readFileSync(new URL('../wailsjs/go/main/App.js', import.meta.url), 'utf8');
const wailsAppDts = fs.readFileSync(new URL('../wailsjs/go/main/App.d.ts', import.meta.url), 'utf8');

test('account pool nav is directly below subscription nav', () => {
  const subscriptionNav = html.indexOf("switchPage('subscription')");
  const accountPoolNav = html.indexOf("switchPage('account-pool')");

  assert.notEqual(subscriptionNav, -1, 'subscription nav should exist');
  assert.notEqual(accountPoolNav, -1, 'account pool nav should exist');
  assert.ok(accountPoolNav > subscriptionNav, 'account pool nav should come after subscription');
  assert.match(html, /data-page="account-pool"[\s\S]*账号池/);
});

test('account pool page exposes list, import, export and clipboard controls', () => {
  assert.match(html, /id="page-account-pool"/);
  assert.match(html, /id="account-pool-table-body"/);
  assert.match(html, /id="account-pool-output-dir"/);
  assert.match(html, /onclick="openAccountPoolImportModal\(\)"/);
  assert.match(html, /onclick="exportAccountPoolToClipboard\(\)"/);
  assert.match(html, /onclick="loadAccountPool\(\)"/);
  assert.match(html, /id="account-pool-import-modal"/);
  assert.match(html, /id="account-pool-import-data"/);
  assert.match(html, /onclick="readAccountPoolClipboard\(\)"/);
  assert.match(html, /onclick="importAccountPoolJSON\(\)"/);
});

test('account pool exposes kiro.rs sync status and mode selection', () => {
  assert.match(html, /同步状态/);
  assert.match(html, /id="account-pool-sync-modal"/);
  assert.match(html, /只同步未同步/);
  assert.match(html, /全量同步/);
  assert.match(accountPoolJs, /SyncAccountPoolToKiroRS\(['"]unsynced['"]\)/);
  assert.match(accountPoolJs, /SyncAccountPoolToKiroRS\(['"]all['"]\)/);
  assert.match(wailsAppJs, /export function SyncAccountPoolToKiroRS\(arg1\)/);
  assert.match(wailsAppDts, /export function SyncAccountPoolToKiroRS\(arg1:string\)/);
});

test('account pool script is loaded and copied by frontend build', () => {
  assert.match(html, /<script src="js\/account_pool\.js"><\/script>/);
  assert.match(buildJs, /'account_pool\.js'/);
});

test('account pool page is registered in app navigation', () => {
  assert.match(appJs, /account-pool['"]\s*:\s*['"]账号池['"]/);
  assert.match(appJs, /pageId === 'account-pool'[\s\S]*loadAccountPool\(\)/);
});

test('wails wrappers expose account pool APIs', () => {
  for (const name of ['ListAccountPool', 'ImportAccountPoolJSON', 'ExportAccountPoolJSON']) {
    assert.match(wailsAppJs, new RegExp(`export function ${name}\\(`));
    assert.match(wailsAppDts, new RegExp(`export function ${name}\\(`));
  }
});

test('kiro.rs sync toast reports locally removed rejected accounts', () => {
  assert.match(accountPoolJs, /removedRejected/);
  assert.match(accountPoolJs, /已删除本地失效账号/);
});
