'use strict';

const assert = require('node:assert/strict');
const { test } = require('node:test');
const { spawnSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');

const { ENV_KEYS, isolatedPackage } = require('../test-support/helpers');

/** Run scripts/vendor-binary.js inside a private copy of the package. */
function runVendor(root, args) {
  const env = { ...process.env };
  for (const key of ENV_KEYS) {
    delete env[key];
  }
  return spawnSync(process.execPath, [path.join(root, 'scripts', 'vendor-binary.js'), ...args], {
    encoding: 'utf8',
    env,
  });
}

test('--list reports the empty vendor directory without copying anything', () => {
  const { root } = isolatedPackage({ scripts: true });
  const result = runVendor(root, ['--list']);

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /no vendored executable at /);
  assert.equal(fs.existsSync(path.join(root, 'vendor')), false, '--list must not create vendor/');
  assert.equal(result.stderr, '');
});

test('the usage text documents --list', () => {
  const { root } = isolatedPackage({ scripts: true });
  const result = runVendor(root, ['--help']);

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stderr, /--list/);
});
