import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const taskJs = fs.readFileSync(new URL('../js/task.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

test('overview live status includes collapsible diagnostics container', () => {
  for (const id of [
    'st-diagnostics',
    'st-diagnostics-toggle',
    'st-diagnostics-summary',
    'st-diagnostics-details',
  ]) {
    const matches = html.match(new RegExp(`id="${id}"`, 'g')) || [];
    assert.equal(matches.length, 1, `${id} should appear exactly once`);
  }
  assert.match(html, /data-diagnostics-expanded="false"/);
});

test('task status renderer displays total plus detail counts for diagnostics', () => {
  assert.match(taskJs, /function renderTaskDiagnostics\(\s*diagnostics\s*\)/);
  assert.match(taskJs, /function formatDiagnosticGroup\(/);
  assert.match(taskJs, /OTPFailures|otpFailures/);
  assert.match(taskJs, /验证码发送失败/);
  assert.match(taskJs, /验证码无效/);
  assert.match(taskJs, /验证码超时/);
  assert.match(taskJs, /join\(' \/ '\)/);
});

test('diagnostics toggle and translations are wired', () => {
  assert.match(taskJs, /function toggleTaskDiagnostics\(\)/);
  assert.match(taskJs, /st-diagnostics-toggle/);
  for (const key of [
    'diagnosticsTitle',
    'diagnosticsExpand',
    'diagnosticsCollapse',
    'diagnosticsOtp',
    'diagnosticsPostRegistration',
    'diagnosticsNetwork',
    'diagnosticsGraph',
  ]) {
    assert.match(i18nJs, new RegExp(`${key}:`));
  }
});
