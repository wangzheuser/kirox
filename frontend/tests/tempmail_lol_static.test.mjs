import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

test('registration page exposes TempMail.lol provider', () => {
  assert.match(html, /toggleEmailProvider\('tempmail_lol'\)/);
  assert.match(html, /value="tempmail_lol"/);
  assert.match(html, />TempMail\.lol</);
});

test('provider selection styles and hints include TempMail.lol', () => {
  assert.match(uiJs, /'tempmail_lol'/);
  assert.match(uiJs, /provider === 'tempmail_lol'/);
  assert.match(uiJs, /register\.tempmailLolHint/);
});

test('TempMail.lol is a zero-config provider in start payload', () => {
  assert.match(appJs, /config\.emailProviders\.includes\('moemail'\)/);
  assert.match(appJs, /config\.emailProviders\.includes\('cloudmail'\)/);
  assert.doesNotMatch(appJs, /config\.emailProviders\.includes\('tempmail_lol'\)[\s\S]{0,500}throw new Error/);
});

test('TempMail.lol translations exist', () => {
  assert.match(i18nJs, /tempmailLol:\s*'TempMail\.lol'/);
  assert.match(i18nJs, /tempmailLolHint/);
});
