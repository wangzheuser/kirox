import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

test('registration page exposes mail.gw provider', () => {
  assert.match(html, /selectEmailProvider\('mailgw'\)/);
  assert.match(html, /value="mailgw"/);
  assert.match(html, />mail\.gw</);
});

test('provider selection styles and hints include mail.gw', () => {
  assert.match(uiJs, /'mailgw'/);
  assert.match(uiJs, /provider === 'mailgw'/);
  assert.match(uiJs, /register\.mailgwHint/);
});

test('mail.gw is a zero-config provider in start payload', () => {
  assert.match(appJs, /config\.emailProvider === 'moemail'/);
  assert.match(appJs, /config\.emailProvider === 'cloudmail'/);
  assert.doesNotMatch(appJs, /config\.emailProvider === 'mailgw'[\s\S]{0,500}throw new Error/);
});

test('mail.gw translations exist', () => {
  assert.match(i18nJs, /mailgw:\s*'mail\.gw'/);
  assert.match(i18nJs, /mailgwHint/);
});
