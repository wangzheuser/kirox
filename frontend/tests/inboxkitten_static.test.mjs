import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

test('registration page exposes InboxKitten provider', () => {
  assert.match(html, /toggleEmailProvider\('inboxkitten'\)/);
  assert.match(html, /value="inboxkitten"/);
  assert.match(html, />InboxKitten</);
});

test('provider selection styles and hints include InboxKitten', () => {
  assert.match(uiJs, /'inboxkitten'/);
  assert.match(uiJs, /provider === 'inboxkitten'/);
  assert.match(uiJs, /register\.inboxKittenHint/);
});

test('InboxKitten is a zero-config provider in start payload', () => {
  assert.match(appJs, /config\.emailProviders\.includes\('moemail'\)/);
  assert.match(appJs, /config\.emailProviders\.includes\('cloudmail'\)/);
  assert.doesNotMatch(appJs, /config\.emailProviders\.includes\('inboxkitten'\)[\s\S]{0,500}throw new Error/);
});

test('InboxKitten translations exist', () => {
  assert.match(i18nJs, /inboxKitten:\s*'InboxKitten'/);
  assert.match(i18nJs, /inboxKittenHint/);
});
