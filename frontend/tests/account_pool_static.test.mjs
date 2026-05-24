import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
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
