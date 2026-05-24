import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const repoRoot = path.resolve(new URL('../..', import.meta.url).pathname.replace(/^\/(.:)/, '$1'));

const checkedFiles = [
  'app.go',
  'internal/core/config.go',
  'internal/core/registrar.go',
  'internal/task/coordinator.go',
  'internal/storage/storage.go',
  'frontend/index.html',
  'frontend/js/app.js',
  'frontend/wailsjs/go/main/App.js',
  'frontend/wailsjs/go/main/App.d.ts'
];

const forbidden = [
  'OutlookRegister' + 'DomainOverride',
  'BuildOutlook' + 'RegistrationEmail',
  'outlook_register_domain' + '_override',
  'cfg-outlook-register' + '-domain',
  'Outlook 注册邮箱' + '后缀覆盖'
];

test('Outlook registration domain override is removed from production code', () => {
  for (const relative of checkedFiles) {
    const source = fs.readFileSync(path.join(repoRoot, relative), 'utf8');
    for (const token of forbidden) {
      assert.equal(source.includes(token), false, `${relative} should not contain ${token}`);
    }
  }
});

test('legacy domain override tests are removed', () => {
  assert.equal(fs.existsSync(path.join(repoRoot, 'internal/storage/outlook_register_domain_test.go')), false);
});
