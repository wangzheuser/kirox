import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const layoutCss = fs.readFileSync(new URL('../css/layout.css', import.meta.url), 'utf8');

test('register page exposes a visible vertical scrollbar', () => {
  assert.match(layoutCss, /#page-register\.active\s*\{[\s\S]*overflow-y:\s*auto/);
  assert.match(layoutCss, /#page-register\.active\s*\{[\s\S]*scrollbar-width:\s*thin/);
  assert.match(layoutCss, /#page-register\.active::-webkit-scrollbar\s*\{[\s\S]*display:\s*block/);
  assert.match(layoutCss, /#page-register\.active::-webkit-scrollbar\s*\{[\s\S]*width:\s*10px/);
});
