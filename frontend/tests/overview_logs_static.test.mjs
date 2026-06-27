import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');
const taskJs = fs.readFileSync(new URL('../js/task.js', import.meta.url), 'utf8');

function htmlBetween(startNeedle, endNeedle) {
  const start = html.indexOf(startNeedle);
  const end = html.indexOf(endNeedle, start);
  assert.ok(start >= 0, `missing ${startNeedle}`);
  assert.ok(end > start, `missing ${endNeedle} after ${startNeedle}`);
  return html.slice(start, end);
}

test('logs are embedded at the bottom of overview page', () => {
  const overviewHtml = htmlBetween('id="page-overview"', 'id="page-info"');
  const logBoxMatches = html.match(/id="unified-log-box"/g) || [];

  assert.equal(logBoxMatches.length, 1, 'unified log box should appear exactly once');
  assert.match(overviewHtml, /class="logs-panel"/);
  assert.match(overviewHtml, /id="unified-log-box"/);
  assert.ok(
    overviewHtml.indexOf('id="st-diagnostics"') < overviewHtml.indexOf('id="unified-log-box"'),
    'overview logs should be rendered after live status diagnostics'
  );
});

test('standalone logs menu and page are removed', () => {
  assert.doesNotMatch(html, /data-page="logs"/);
  assert.doesNotMatch(html, /switchPage\('logs'\)/);
  assert.doesNotMatch(html, /id="page-logs"/);
  assert.doesNotMatch(html, /data-i18n="nav\.logs"/);
});

test('unreachable logs page metadata is removed but log panel translations stay', () => {
  assert.doesNotMatch(appJs, /\blogs:\s*'运行日志'/);
  assert.doesNotMatch(i18nJs, /nav:\s*\{[^}]*\blogs:/);
  assert.doesNotMatch(i18nJs, /page:\s*\{[^}]*\blogs:/);
  assert.match(i18nJs, /logs:\s*\{\s*title:/);
  assert.match(taskJs, /_tkT\('logs\.empty'/);
});
