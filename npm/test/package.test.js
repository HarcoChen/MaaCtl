'use strict';

const assert = require('node:assert/strict');
const { test } = require('node:test');
const fs = require('node:fs');
const path = require('node:path');

const pkg = require('../package.json');

const ROOT = path.resolve(__dirname, '..');

test('the package exposes the maactl bin shim', () => {
  assert.equal(pkg.name, 'maactl');
  assert.equal(pkg.bin.maactl, 'bin/maactl.js');
  assert.ok(fs.existsSync(path.join(ROOT, pkg.bin.maactl)));
});

test('the bin shim is executable-looking and dependency free', () => {
  const source = fs.readFileSync(path.join(ROOT, pkg.bin.maactl), 'utf8');
  assert.ok(source.startsWith('#!/usr/bin/env node'), 'shebang is required for the POSIX wrapper');
  assert.equal(pkg.dependencies, undefined);
  assert.equal(pkg.devDependencies, undefined);
});

test('the package is Windows only and needs Node 22+', () => {
  assert.deepEqual(pkg.os, ['win32']);
  assert.equal(pkg.engines.node, '>=22');
});

test('every file referenced by the manifest is published', () => {
  for (const entry of ['bin/', 'lib/', 'scripts/', 'vendor/', 'README.md']) {
    assert.ok(pkg.files.includes(entry), `${entry} must be listed in files`);
  }
});

test('the postinstall hook points at an existing script', () => {
  assert.equal(pkg.scripts.postinstall, 'node scripts/install.js');
  assert.ok(fs.existsSync(path.join(ROOT, 'scripts', 'install.js')));
});

test('the version is a publishable semver', () => {
  assert.match(pkg.version, /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/);
});
