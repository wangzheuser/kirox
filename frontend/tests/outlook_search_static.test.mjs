import { readFileSync } from 'node:fs';
import test from 'node:test';
import assert from 'node:assert/strict';

const html = readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const js = readFileSync(new URL('../js/accounts.js', import.meta.url), 'utf8');

test('Outlook page exposes keyword search input', () => {
  assert.match(html, /id="outlook-search-input"/);
  assert.match(html, /搜索邮箱关键词/);
});

test('Outlook render applies status filter and keyword filter', () => {
  assert.match(js, /outlookSearchKeyword/);
  assert.match(js, /setOutlookSearchKeyword/);
  assert.match(js, /toLowerCase\(\)\.includes\(keyword/);
  assert.match(js, /outlookFilteredAccounts\s*=\s*filtered/);
});

test('Outlook supports batch reset of selected emails', () => {
  assert.match(html, /id="outlook-batch-reset-btn"/);
  assert.match(js, /batchResetOutlookAccountStatuses/);
  assert.match(js, /ResetOutlookAccountStatusesByEmails/);
});
