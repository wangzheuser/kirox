import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const componentsCss = fs.readFileSync(new URL('../css/components.css', import.meta.url), 'utf8');

const providers = ['outlook', 'moemail', 'mailporary', 'cloudmail', 'mailgw', 'mailtm', 'tempmail_lol', 'emailnator', 'guerrillamail', 'inboxkitten', 'freecustom'];

test('email provider options use a uniform grid layout', () => {
  assert.match(html, /class="email-provider-grid"/);
  assert.doesNotMatch(html, /display:flex;gap:8px;flex-wrap:wrap/);
  assert.match(componentsCss, /\.email-provider-grid\s*\{/);
  assert.match(componentsCss, /grid-template-columns:\s*repeat\(4,\s*minmax\(0,\s*1fr\)\)/);
});

test('every email provider is rendered as a non-wrapping card', () => {
  for (const provider of providers) {
    assert.match(html, new RegExp(`class="email-provider-card"[^>]+toggleEmailProvider\\('${provider}'\\)`));
    assert.match(html, new RegExp(`id="provider-${provider}" class="email-provider-content"`));
  }
  assert.match(componentsCss, /\.email-provider-card\s*\{/);
  assert.match(componentsCss, /\.email-provider-content\s*\{[\s\S]*white-space:\s*nowrap/);
});

test('email provider selected state is controlled with a class', () => {
  assert.match(componentsCss, /\.email-provider-card\.is-active\s*\{/);
  assert.match(uiJs, /btn\.classList\.toggle\('is-active',\s*active\)/);
  assert.doesNotMatch(uiJs, /btn\.style\.borderColor\s*=/);
  assert.doesNotMatch(uiJs, /btn\.style\.background\s*=/);
});
