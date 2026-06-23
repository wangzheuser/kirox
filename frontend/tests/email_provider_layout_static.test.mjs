import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const componentsCss = fs.readFileSync(new URL('../css/components.css', import.meta.url), 'utf8');

const providers = ['outlook', 'moemail', 'mailporary', 'cloudmail', 'mailgw', 'mailtm', 'tempmail_lol', 'emailnator', 'guerrillamail', 'inboxkitten', 'freecustom'];
const longNameProviders = ['tmio_bltiwd', 'tmio_wnbaldwy', 'tmio_bwmyga', 'tmio_ozsaip'];

test('email provider options use a uniform grid layout', () => {
  assert.match(html, /class="email-provider-grid"/);
  assert.doesNotMatch(html, /display:flex;gap:8px;flex-wrap:wrap/);
  assert.match(componentsCss, /\.email-provider-grid\s*\{/);
  assert.match(componentsCss, /grid-template-columns:\s*repeat\(auto-fit,\s*minmax\(220px,\s*1fr\)\)/);
  assert.match(html, /id="page-register"[\s\S]*class="page-scroll" style="max-width:1040px;margin:0 auto;"/);
});

test('every email provider is rendered as a non-wrapping card', () => {
  for (const provider of providers) {
    assert.match(html, new RegExp(`class="email-provider-card"[^>]+toggleEmailProvider\\('${provider}'\\)`));
    assert.match(html, new RegExp(`id="provider-${provider}" class="email-provider-content"`));
  }
  assert.match(componentsCss, /\.email-provider-card\s*\{/);
  assert.match(componentsCss, /\.email-provider-content\s*\{[\s\S]*white-space:\s*nowrap/);
});

test('long TempMailIO provider labels have enough button width to remain visible', () => {
  for (const provider of longNameProviders) {
    assert.match(html, new RegExp(`id="provider-${provider}" class="email-provider-content"`));
  }
  assert.match(componentsCss, /\.email-provider-card\s*\{[\s\S]*min-width:\s*220px/);
  assert.doesNotMatch(componentsCss, /\.email-provider-content\s*\{[\s\S]*overflow:\s*hidden/);
  assert.doesNotMatch(componentsCss, /\.email-provider-content\s*\{[\s\S]*text-overflow:\s*ellipsis/);
});

test('email provider selected state is controlled with a class', () => {
  assert.match(componentsCss, /\.email-provider-card\.is-active\s*\{/);
  assert.match(uiJs, /btn\.classList\.toggle\('is-active',\s*active\)/);
  assert.doesNotMatch(uiJs, /btn\.style\.borderColor\s*=/);
  assert.doesNotMatch(uiJs, /btn\.style\.background\s*=/);
});
