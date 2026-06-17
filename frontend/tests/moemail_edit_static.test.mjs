import { readFileSync } from 'node:fs';
import test from 'node:test';
import assert from 'node:assert/strict';

const html = readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const js = readFileSync(new URL('../js/moemail.js', import.meta.url), 'utf8');

test('MoeMail list and modal expose edit controls', () => {
  assert.match(js, /编辑/);
  assert.match(js, /beginEditMoeMailConfig/);
  assert.match(html, /moemail-inline-cancel-edit-btn/);
});

test('MoeMail edit flow has edit state, save, cancel and status migration', () => {
  assert.match(js, /moemailEditingIndex\s*=\s*-1/);
  assert.match(js, /saveEditedMoeMailConfig/);
  assert.match(js, /cancelMoeMailEdit/);
  assert.match(js, /moemailConfigStatus\[oldName\]/);
  assert.match(js, /delete\s+moemailConfigStatus\[oldName\]/);
});
