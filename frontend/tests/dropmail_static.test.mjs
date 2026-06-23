import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

test('registration page exposes DropMail provider', () => {
  assert.match(html, /toggleEmailProvider\('dropmail'\)/);
  assert.match(html, /value="dropmail"/);
  assert.match(html, />DropMail</);
});

test('provider selection styles and hints include DropMail', () => {
  assert.match(uiJs, /'dropmail'/);
  assert.match(uiJs, /provider === 'dropmail'/);
  assert.match(uiJs, /register\.dropMailHint/);
});

test('DropMail is a zero-config provider in start payload', () => {
  assert.match(appJs, /config\.emailProviders\.includes\('moemail'\)/);
  assert.match(appJs, /config\.emailProviders\.includes\('cloudmail'\)/);
  assert.doesNotMatch(appJs, /config\.emailProviders\.includes\('dropmail'\)[\s\S]{0,500}throw new Error/);
});

test('DropMail translations exist', () => {
  assert.match(i18nJs, /dropMail:\s*'DropMail'/);
  assert.match(i18nJs, /dropMailHint/);
});
