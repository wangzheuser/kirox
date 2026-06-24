import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

const providers = [
  ['fce_areueally', 'fceAreueally', 'FreeCustom areueally'],
  ['fce_junkstopper', 'fceJunkstopper', 'FreeCustom junkstopper'],
  ['fce_ditpay', 'fceDitpay', 'FreeCustom ditpay'],
];

test('registration page exposes FreeCustom fixed-domain providers', () => {
  for (const [value, key, label] of providers) {
    assert.match(html, new RegExp(`toggleEmailProvider\\('${value}'\\)`));
    assert.match(html, new RegExp(`value="${value}"`));
    assert.match(html, new RegExp(`register\\.${key}`));
    assert.match(i18nJs, new RegExp(`${key}:\\s*'${label.replace('.', '\\.')}'`));
  }
});

test('FreeCustom fixed-domain providers have selection styles and hints', () => {
  for (const [value, key] of providers) {
    assert.match(uiJs, new RegExp(`'${value}'`));
    assert.match(uiJs, new RegExp(`provider === '${value}'`));
    assert.match(uiJs, new RegExp(`register\\.${key}Hint`));
  }
});

test('FreeCustom fixed-domain providers are zero-config in start payload', () => {
  for (const [value] of providers) {
    assert.doesNotMatch(appJs, new RegExp(`config\\.emailProviders\\.includes\\('${value}'\\)[\\s\\S]{0,500}throw new Error`));
  }
});
