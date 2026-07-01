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
  assert.match(html, /域名尝试统计/);
  assert.match(html, /当前探索比例/);
  assert.match(html, /id="email-provider-stats-body"/);
  assert.match(html, /id="btn-select-selected-email-provider-stats"[^>]*disabled/);
  assert.match(html, /勾选所选邮箱渠道/);
  assert.match(html, /id="btn-select-email-provider-stats"/);
  assert.match(html, /勾选所有邮箱渠道/);
  assert.match(html, /id="btn-reset-email-provider-stats"/);
  assert.match(html, /<th[^>]*>\s*<\/th>\s*<th[^>]*>\s*渠道\s*<\/th>/);
  assert.match(html, /<td colspan="6"[^>]*>加载中\.\.\.<\/td>/);
});

test('frontend loads and resets email provider stats through dedicated api', () => {
  assert.match(appJs, /function\s+loadEmailProviderStats\s*\(/);
  assert.match(appJs, /GetEmailProviderStats\s*\(/);
  assert.match(appJs, /function\s+resetEmailProviderStats\s*\(/);
  assert.match(appJs, /ResetEmailProviderStats\s*\(/);
  assert.match(appJs, /formatEmailProviderSuccessDomains\s*\(/);
  assert.match(appJs, /formatEmailProviderDomainAttempts\s*\(/);
  assert.match(appJs, /cfg-domain-exploration-percent/);
});

test('frontend can replace selected providers with providers from stats', () => {
  assert.match(appJs, /function\s+selectEmailProvidersFromStats\s*\(/);
  assert.match(appJs, /selectEmailProvidersFromStats[\s\S]*GetEmailProviderStats\s*\(/);
  assert.match(appJs, /selectEmailProvidersFromStats[\s\S]*setEmailProviders\s*\(\s*providers\s*\)/);
  assert.match(appJs, /暂无统计渠道可勾选/);
  assert.doesNotMatch(appJs, /selectEmailProvidersFromStats[\s\S]*toggleEmailProvider\s*\(/);
});

test('frontend can replace selected providers with checked stats rows only', () => {
  assert.match(appJs, /var\s+selectedEmailProviderStatProviders\s*=\s*\[\]/);
  assert.match(appJs, /function\s+toggleEmailProviderStatSelection\s*\(/);
  assert.match(appJs, /function\s+updateSelectedEmailProviderStatsButtonState\s*\(/);
  assert.match(appJs, /function\s+selectSelectedEmailProvidersFromStats\s*\(/);
  assert.match(appJs, /selectSelectedEmailProvidersFromStats[\s\S]*setEmailProviders\s*\(\s*providers\s*\)/);
  assert.match(appJs, /已按所选统计渠道覆盖当前邮箱渠道选择/);
  assert.match(appJs, /暂无所选统计渠道可勾选/);
  assert.match(appJs, /type="checkbox"[\s\S]*toggleEmailProviderStatSelection/);
  assert.match(appJs, /data-provider="/);
  assert.match(appJs, /btn-select-selected-email-provider-stats/);
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
  assert.match(modelsTs, /domainAttempts: Record<string, number>/);
  assert.match(modelsTs, /domainExplorationPercent: number/);
});
