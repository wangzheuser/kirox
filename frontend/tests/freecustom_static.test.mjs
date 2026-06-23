import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

test('registration page exposes FreeCustom.Email provider', () => {
  assert.match(html, /toggleEmailProvider\('freecustom'\)/);
  assert.match(html, /value="freecustom"/);
  assert.match(html, />FreeCustom\.Email</);
});

test('provider selection styles and hints include FreeCustom.Email', () => {
  assert.match(uiJs, /'freecustom'/);
  assert.match(uiJs, /provider === 'freecustom'/);
  assert.match(uiJs, /register\.freeCustomHint/);
});

test('FreeCustom.Email is a zero-config provider in start payload', () => {
  assert.match(appJs, /config\.emailProviders\.includes\('moemail'\)/);
  assert.match(appJs, /config\.emailProviders\.includes\('cloudmail'\)/);
  assert.doesNotMatch(appJs, /config\.emailProviders\.includes\('freecustom'\)[\s\S]{0,500}throw new Error/);
});

test('FreeCustom.Email translations exist', () => {
  assert.match(i18nJs, /freeCustom:\s*'FreeCustom\.Email'/);
  assert.match(i18nJs, /freeCustomHint/);
});
