import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const appJs = fs.readFileSync(new URL('../js/app.js', import.meta.url), 'utf8');
const uiJs = fs.readFileSync(new URL('../js/ui.js', import.meta.url), 'utf8');
const i18nJs = fs.readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');

const providers = [
  ['mailcatch', 'mailCatch', 'MailCatch'],
  ['tempmailo', 'tempMailo', 'TempMailo'],
  ['generator_email', 'generatorEmail', 'Generator.Email'],
  ['mailtowin', 'mailToWin', 'MailToWin'],
  ['mail2me', 'mail2Me', 'Mail2Me'],
  ['pickmemail', 'pickMeMail', 'PickMeMail'],
  ['maximail', 'maxiMail', 'MaxiMail'],
  ['emlpro', 'emlPro', 'EmlPro'],
  ['freeml', 'freeML', 'FreeML'],
  ['emlhub', 'emlHub', 'EmlHub'],
  ['emltmp', 'emlTmp', 'EmlTmp'],
  ['mailpwr', 'mailPwr', 'MailPwr'],
  ['tenmail', 'tenMail', '10Mail'],
  ['dropmail_me', 'dropMailMe', 'DropMail.me'],
  ['mimimail', 'mimiMail', 'MimiMail'],
  ['pickmail', 'pickMail', 'PickMail'],
  ['spymail', 'spyMail', 'SpyMail'],
  ['yomail', 'yoMail', 'YoMail'],
];

test('registration page exposes new zero-config providers', () => {
  for (const [value, key, label] of providers) {
    assert.match(html, new RegExp(`selectEmailProvider\\('${value}'\\)`));
    assert.match(html, new RegExp(`value="${value}"`));
    assert.match(html, new RegExp(`register\\.${key}`));
    assert.match(i18nJs, new RegExp(`${key}:\\s*'${label.replace('.', '\\.')}'`));
  }
});

test('provider selection styles and hints include new providers', () => {
  for (const [value, key] of providers) {
    assert.match(uiJs, new RegExp(`'${value}'`));
    assert.match(uiJs, new RegExp(`provider === '${value}'`));
    assert.match(uiJs, new RegExp(`register\\.${key}Hint`));
  }
});

test('new providers are zero-config in start payload', () => {
  for (const [value] of providers) {
    assert.doesNotMatch(appJs, new RegExp(`config\\.emailProvider === '${value}'[\\s\\S]{0,500}throw new Error`));
  }
});
