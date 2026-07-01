import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const taskJs = fs.readFileSync(new URL('../js/task.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

test('overview live status includes collapsible diagnostics container', () => {
  for (const id of [
    'st-diagnostics',
    'st-diagnostics-reset',
    'st-diagnostics-toggle',
    'st-diagnostics-summary',
    'st-diagnostics-details',
  ]) {
    const matches = html.match(new RegExp(`id="${id}"`, 'g')) || [];
    assert.equal(matches.length, 1, `${id} should appear exactly once`);
  }
  assert.match(html, /data-diagnostics-expanded="false"/);
  assert.ok(
    html.indexOf('id="st-diagnostics-reset"') < html.indexOf('id="st-diagnostics-toggle"'),
    'diagnostics reset button should be rendered before the expand toggle'
  );
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
  assert.match(taskJs, /function resetTaskDiagnostics\(\)/);
  assert.match(taskJs, /st-diagnostics-toggle/);
  assert.match(html, /onclick="resetTaskDiagnostics\(\)"/);
  assert.match(html, /data-i18n="common\.reset"/);
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

test('diagnostics reset clears current details and keeps old snapshot hidden on refresh', () => {
  const root = createElement();
  root.setAttribute('data-diagnostics-expanded', 'true');
  const summary = createElement();
  const details = createElement();
  const toggleText = createElement();
  const elements = {
    'st-diagnostics': root,
    'st-diagnostics-summary': summary,
    'st-diagnostics-details': details,
    'st-diagnostics-toggle-text': toggleText,
  };
  const context = {
    window: {
      addEventListener() {},
    },
    document: {
      getElementById(id) {
        return elements[id] || null;
      },
    },
    navigator: {},
    setInterval() {},
    setTimeout() {},
    console,
  };
  context.window.window = context.window;
  vm.runInNewContext(taskJs, context);

  const firstSnapshot = {
    otpFailures: { total: 3, details: { '验证码无效': 3 } },
    sendOTPDiagnostics: { provider: [{ label: 'mailtm', count: 2 }] },
    topFailures: [{ label: '验证码无效', count: 3 }],
  };

  context.renderTaskDiagnostics(firstSnapshot);
  assert.match(summary.innerHTML, /OTP失败：3/);
  assert.match(details.innerHTML, /验证码无效 3/);
  assert.match(details.innerHTML, /mailtm 2/);

  context.resetTaskDiagnostics();
  assert.doesNotMatch(summary.innerHTML, /OTP失败：3/);
  assert.doesNotMatch(details.innerHTML, /验证码无效 3/);
  assert.doesNotMatch(details.innerHTML, /mailtm 2/);

  context.renderTaskDiagnostics(firstSnapshot);
  assert.doesNotMatch(summary.innerHTML, /OTP失败：3/);
  assert.doesNotMatch(details.innerHTML, /验证码无效 3/);
  assert.doesNotMatch(details.innerHTML, /mailtm 2/);

  context.renderTaskDiagnostics({
    otpFailures: { total: 5, details: { '验证码无效': 5 } },
    sendOTPDiagnostics: { provider: [{ label: 'mailtm', count: 3 }] },
    topFailures: [{ label: '验证码无效', count: 5 }],
  });
  assert.match(summary.innerHTML, /OTP失败：2/);
  assert.match(details.innerHTML, /验证码无效 2/);
  assert.match(details.innerHTML, /mailtm 1/);
});

function createElement() {
  const attrs = {};
  return {
    innerHTML: '',
    textContent: '',
    style: {},
    getAttribute(name) {
      return attrs[name];
    },
    setAttribute(name, value) {
      attrs[name] = String(value);
    },
  };
}
