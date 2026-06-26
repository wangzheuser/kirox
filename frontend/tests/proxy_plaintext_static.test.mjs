import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const proxyPoolJs = fs.readFileSync(new URL('../js/proxy_pool.js', import.meta.url), 'utf8');

function inputTagById(source, id) {
  const escapedId = id.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const match = source.match(new RegExp(`<input\\b[^>]*\\bid="${escapedId}"[^>]*>`));
  assert.ok(match, `missing input #${id}`);
  return match[0];
}

function assertInputType(source, id, type) {
  const tag = inputTagById(source, id);
  assert.match(tag, new RegExp(`\\btype="${type}"(?=\\s|>|/)`));
}

test('proxy url fields are shown in plaintext', () => {
  assertInputType(html, 'cfg-proxy', 'text');
  assertInputType(html, 'cfg-email-proxy', 'text');
});

test('proxy pool url editors are shown in plaintext', () => {
  assert.doesNotMatch(proxyPoolJs, /<input\s+type="password"[^>]*(?:updateProxyEntryURL|savePendingProxyRow)/);
  assert.match(proxyPoolJs, /<input\s+type="text"[^>]*updateProxyEntryURL/);
  assert.match(proxyPoolJs, /<input\s+type="text"[^>]*savePendingProxyRow/);
});
