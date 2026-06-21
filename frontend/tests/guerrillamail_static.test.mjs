import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

test('registration page exposes GuerrillaMail provider', () => {
  assert.match(html, /selectEmailProvider\('guerrillamail'\)/);
  assert.match(html, /value="guerrillamail"/);
  assert.match(html, />GuerrillaMail</);
});

test('provider selection styles and hints include GuerrillaMail', () => {
  assert.match(uiJs, /'guerrillamail'/);
  assert.match(uiJs, /provider === 'guerrillamail'/);
  assert.match(uiJs, /register\.guerrillaMailHint/);
});

test('GuerrillaMail is a zero-config provider in start payload', () => {
  assert.match(appJs, /config\.emailProvider === 'moemail'/);
  assert.match(appJs, /config\.emailProvider === 'cloudmail'/);
  assert.doesNotMatch(appJs, /config\.emailProvider === 'guerrillamail'[\s\S]{0,500}throw new Error/);
});

test('GuerrillaMail translations exist', () => {
  assert.match(i18nJs, /guerrillaMail:\s*'GuerrillaMail'/);
  assert.match(i18nJs, /guerrillaMailHint/);
});
