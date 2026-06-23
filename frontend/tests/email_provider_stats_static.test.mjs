import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const taskJs = fs.readFileSync(new URL('../js/task.js', import.meta.url), 'utf8');
const wailsAppJs = fs.readFileSync(new URL('../wailsjs/go/main/App.js', import.meta.url), 'utf8');
const wailsAppDts = fs.readFileSync(new URL('../wailsjs/go/main/App.d.ts', import.meta.url), 'utf8');
const modelsTs = fs.readFileSync(new URL('../wailsjs/go/models.ts', import.meta.url), 'utf8');

test('register page contains persistent email provider stats card', () => {
  assert.match(html, /id="email-provider-stats-card"/);
  assert.match(html, /验证码收码次数/);
  assert.match(html, /注册成功数/);
  assert.match(html, /注册成功域名统计/);
  assert.match(html, /id="email-provider-stats-body"/);
  assert.match(html, /id="btn-reset-email-provider-stats"/);
});

test('frontend loads and resets email provider stats through dedicated api', () => {
  assert.match(appJs, /function\s+loadEmailProviderStats\s*\(/);
  assert.match(appJs, /GetEmailProviderStats\s*\(/);
  assert.match(appJs, /function\s+resetEmailProviderStats\s*\(/);
  assert.match(appJs, /ResetEmailProviderStats\s*\(/);
  assert.match(appJs, /formatEmailProviderSuccessDomains\s*\(/);
});

test('task completion refreshes email provider stats', () => {
  assert.match(taskJs, /loadEmailProviderStats\s*\(/);
  assert.match(taskJs, /_prevRunning\s*&&\s*!s\.running/);
});

test('wails bindings expose email provider stats api and model', () => {
  assert.match(wailsAppJs, /export function GetEmailProviderStats\(\)/);
  assert.match(wailsAppJs, /export function ResetEmailProviderStats\(\)/);
  assert.match(wailsAppDts, /GetEmailProviderStats\(\):Promise<Array<storage\.EmailProviderStat>>/);
  assert.match(wailsAppDts, /ResetEmailProviderStats\(\):Promise<Record<string, any>>/);
  assert.match(modelsTs, /export class EmailProviderStat/);
  assert.match(modelsTs, /successDomains: Record<string, number>/);
});
