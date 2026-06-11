import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const taskJs = fs.readFileSync(new URL('../js/task.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');

test('task action buttons use unique overview and register ids', () => {
  assert.doesNotMatch(html, /id="btn-start"/);
  assert.doesNotMatch(html, /id="btn-stop"/);
  for (const id of ['ov-btn-start', 'ov-btn-stop', 'reg-btn-start', 'reg-btn-stop']) {
    const matches = html.match(new RegExp(`id="${id}"`, 'g')) || [];
    assert.equal(matches.length, 1, `${id} should appear exactly once`);
  }
});

test('task action state updates overview and register buttons together', () => {
  assert.match(appJs, /function setTaskActionState\(\s*state\s*\)/);
  for (const id of ['ov-btn-start', 'reg-btn-start', 'ov-btn-stop', 'reg-btn-stop']) {
    assert.match(appJs, new RegExp(`setButtonDisabled\\('${id}'`));
  }
  assert.match(taskJs, /setTaskActionState\('starting'\)/);
  assert.match(taskJs, /setTaskActionState\('stopping'\)/);
});

test('task shortcuts reference unique register and stop buttons', () => {
  assert.match(uiJs, /getElementById\('reg-btn-start'\)/);
  assert.match(uiJs, /\['ov-btn-stop',\s*'reg-btn-stop'\]/);
  assert.doesNotMatch(uiJs, /getElementById\('btn-start'\)/);
  assert.doesNotMatch(uiJs, /getElementById\('btn-stop'\)/);
});
