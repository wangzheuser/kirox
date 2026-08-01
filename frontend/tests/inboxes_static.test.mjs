import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

test('registration page exposes Inboxes provider', () => {
  assert.match(html, /toggleEmailProvider\('inboxes'\)/);
  assert.match(html, /value="inboxes"/);
  assert.match(html, />Inboxes</);
});

test('provider selection and hints include Inboxes', () => {
  assert.match(uiJs, /'inboxes'/);
  assert.match(uiJs, /provider === 'inboxes'/);
  assert.match(uiJs, /register\.inboxesHint/);
});

test('Inboxes is a zero-config provider in start payload', () => {
  assert.match(appJs, /config\.emailProviders\.includes\('moemail'\)/);
  assert.match(appJs, /config\.emailProviders\.includes\('cloudmail'\)/);
  assert.doesNotMatch(appJs, /config\.emailProviders\.includes\('inboxes'\)[\s\S]{0,500}throw new Error/);
});

test('Inboxes translations exist', () => {
  assert.match(i18nJs, /inboxes:\s*'Inboxes'/);
  assert.match(i18nJs, /inboxesHint/);
});
