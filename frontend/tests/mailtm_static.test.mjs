import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

test('registration page exposes mail.tm provider', () => {
  assert.match(html, /toggleEmailProvider\('mailtm'\)/);
  assert.match(html, /value="mailtm"/);
  assert.match(html, />mail\.tm</);
});

test('provider selection styles and hints include mail.tm', () => {
  assert.match(uiJs, /'mailtm'/);
  assert.match(uiJs, /provider === 'mailtm'/);
  assert.match(uiJs, /register\.mailtmHint/);
});

test('mail.tm is a zero-config provider in start payload', () => {
  assert.match(appJs, /config\.emailProviders\.includes\('moemail'\)/);
  assert.match(appJs, /config\.emailProviders\.includes\('cloudmail'\)/);
  assert.doesNotMatch(appJs, /config\.emailProviders\.includes\('mailtm'\)[\s\S]{0,500}throw new Error/);
});

test('mail.tm translations exist', () => {
  assert.match(i18nJs, /mailtm:\s*'mail\.tm'/);
  assert.match(i18nJs, /mailtmHint/);
});
