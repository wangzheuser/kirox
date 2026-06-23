import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

test('registration page exposes Emailnator provider', () => {
  assert.match(html, /toggleEmailProvider\('emailnator'\)/);
  assert.match(html, /value="emailnator"/);
  assert.match(html, />Emailnator</);
});

test('provider selection styles and hints include Emailnator', () => {
  assert.match(uiJs, /'emailnator'/);
  assert.match(uiJs, /'mailgw'/);
  assert.match(uiJs, /provider === 'emailnator'/);
  assert.match(uiJs, /register\.emailnatorHint/);
});

test('Emailnator is a zero-config provider in start payload', () => {
  assert.match(appJs, /config\.emailProviders\.includes\('moemail'\)/);
  assert.match(appJs, /config\.emailProviders\.includes\('cloudmail'\)/);
  assert.doesNotMatch(appJs, /config\.emailProviders\.includes\('emailnator'\)[\s\S]{0,500}throw new Error/);
});

test('Emailnator translations exist', () => {
  assert.match(i18nJs, /emailnator:\s*'Emailnator'/);
  assert.match(i18nJs, /emailnatorHint/);
});
