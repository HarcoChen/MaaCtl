'use strict';

const assert = require('node:assert/strict');
const { test } = require('node:test');
const { spawnSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const pkg = require('../package.json');
const { ENV_KEYS, isolatedPackage, tempDir } = require('../test-support/helpers');

/** Run scripts/install.js inside a private copy of the wrapper, offline. */
function runInstall(root, values) {
  const env = { ...process.env };
  for (const key of ENV_KEYS) {
    delete env[key];
  }
  return spawnSync(process.execPath, [path.join(root, 'scripts', 'install.js')], {
    encoding: 'utf8',
    env: { ...env, MAACTL_LANG: 'en', ...values },
  });
}

/**
 * A MAACTL_HOME whose versioned cache directory cannot be created, because a
 * plain file sits where the directory would go. It makes ensureBinary() fail
 * without touching the network.
 */
function blockedHome() {
  const home = tempDir();
  fs.mkdirSync(path.join(home, 'npm'), { recursive: true });
  fs.writeFileSync(path.join(home, 'npm', pkg.version), 'not a directory');
  return home;
}

test('MAACTL_SKIP_DOWNLOAD skips the pre-download entirely', () => {
  const { root } = isolatedPackage({ scripts: true });
  const result = runInstall(root, { MAACTL_SKIP_DOWNLOAD: '1', MAACTL_HOME: tempDir() });

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stderr, /skipped downloading maactl\.exe/);
  assert.doesNotMatch(result.stderr, /could not pre-download/);
});

test('a failed pre-download only warns unless MAACTL_STRICT_INSTALL is set', () => {
  const { root } = isolatedPackage({ scripts: true });
  const home = blockedHome();

  const lenient = runInstall(root, { MAACTL_HOME: home });
  assert.equal(lenient.status, 0, lenient.stderr);
  assert.match(lenient.stderr, /could not pre-download maactl\.exe/);
  assert.match(lenient.stderr, /MAACTL_STRICT_INSTALL=1/, 'the hint explains how to fail the install');

  const strict = runInstall(root, { MAACTL_HOME: home, MAACTL_STRICT_INSTALL: '1' });
  assert.equal(strict.status, 1, strict.stderr);
  assert.match(strict.stderr, /install step failed/);
});
