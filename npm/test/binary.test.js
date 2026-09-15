'use strict';

const assert = require('node:assert/strict');
const { test } = require('node:test');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const binary = require('../lib/binary');
const { tempDir, withEnv, isolatedPackage } = require('../test-support/helpers');

test('binaryUrl builds the GitHub release URL of the platform archive', () => {
  withEnv({}, () => {
    assert.equal(
      binary.binaryUrl(),
      `https://github.com/TanyaShue/MaaCtl/releases/download/v${binary.version()}/maactl-${binary.version()}-win-x86_64.zip`,
    );
  });
});

test('assetName follows the package version and the platform', () => {
  withEnv({}, () => {
    assert.equal(binary.platform(), 'win-x86_64');
    assert.equal(binary.assetName(), `maactl-${binary.version()}-win-x86_64.zip`);
  });
  withEnv({ MAACTL_VERSION: 'v9.9.9', MAACTL_PLATFORM: 'win-aarch64' }, () => {
    assert.equal(binary.assetName(), 'maactl-9.9.9-win-aarch64.zip');
  });
});

test('binaryUrl honours version, repo, asset and mirror overrides', () => {
  withEnv({ MAACTL_VERSION: '1.2.3-beta.1', MAACTL_REPO: 'someone/fork', MAACTL_ASSET: 'maactl-1.2.3-win-x86_64.zip' }, () => {
    assert.equal(
      binary.binaryUrl(),
      'https://github.com/someone/fork/releases/download/v1.2.3-beta.1/maactl-1.2.3-win-x86_64.zip',
    );
  });
  withEnv({ MAACTL_VERSION: 'v9.9.9', MAACTL_MIRROR: 'https://ghproxy.example/' }, () => {
    assert.equal(
      binary.binaryUrl(),
      'https://ghproxy.example/https://github.com/TanyaShue/MaaCtl/releases/download/v9.9.9/maactl-9.9.9-win-x86_64.zip',
    );
  });
  withEnv({ MAACTL_BINARY_URL: 'https://example.test/custom.zip', MAACTL_MIRROR: 'https://mirror.test' }, () => {
    assert.equal(binary.binaryUrl(), 'https://example.test/custom.zip');
  });
});

test('archivePath keeps the downloaded archive next to the cached executable', () => {
  withEnv({}, () => {
    const dir = path.join(os.tmpdir(), 'maactl-cache', 'npm', '1.2.3');
    assert.equal(binary.archivePath(path.join(dir, 'maactl.exe')), path.join(dir, binary.assetName()));
  });
});

test('home() follows LOCALAPPDATA on Windows and XDG_CACHE_HOME elsewhere', () => {
  withEnv({ LOCALAPPDATA: 'C:\\Users\\tester\\AppData\\Local' }, () => {
    if (process.platform === 'win32') {
      assert.equal(binary.home(), path.resolve(path.join('C:\\Users\\tester\\AppData\\Local', 'maactl')));
    }
  });
  withEnv({ XDG_CACHE_HOME: path.join(os.tmpdir(), 'xdg-cache') }, () => {
    if (process.platform !== 'win32') {
      assert.equal(binary.home(), path.join(os.tmpdir(), 'xdg-cache', 'maactl'));
    }
  });
  withEnv({ MAACTL_HOME: path.join(os.tmpdir(), 'maactl-home') }, () => {
    assert.equal(binary.home(), path.resolve(path.join(os.tmpdir(), 'maactl-home')));
  });
});

test('cacheExePath is versioned so upgrades never reuse an old executable', () => {
  withEnv({ MAACTL_HOME: 'C:\\cache' }, () => {
    assert.equal(
      binary.cacheExePath('1.2.3'),
      path.join(path.resolve('C:\\cache'), 'npm', '1.2.3', 'maactl.exe'),
    );
  });
});

test('resolveBinary honours MAACTL_VERSION when picking the cache directory', () => {
  const home = tempDir();
  const { binary: subject } = isolatedPackage();
  withEnv({ MAACTL_HOME: home, MAACTL_VERSION: 'v9.9.9' }, () => {
    const cached = subject.cacheExePath();
    assert.equal(cached, path.join(home, 'npm', '9.9.9', 'maactl.exe'));
    fs.mkdirSync(path.dirname(cached), { recursive: true });
    fs.writeFileSync(cached, 'MZ fake');
    assert.equal(subject.resolveBinary(), cached);
  });
});

test('resolveBinary returns null when nothing is available locally', () => {
  const { binary: subject } = isolatedPackage();
  withEnv({ MAACTL_HOME: tempDir() }, () => {
    assert.equal(subject.resolveBinary(), null);
  });
});

test('resolveBinary prefers the executable bundled in the package', () => {
  const home = tempDir();
  const { root, binary: subject } = isolatedPackage({ vendor: 'MZ vendored' });
  withEnv({ MAACTL_HOME: home }, () => {
    const cached = subject.cacheExePath();
    fs.mkdirSync(path.dirname(cached), { recursive: true });
    fs.writeFileSync(cached, 'MZ cached');
    assert.equal(subject.resolveBinary(), path.join(root, 'vendor', 'maactl.exe'));
  });
});

test('resolveBinary prefers MAACTL_BINARY and reports missing paths', () => {
  const dir = tempDir();
  const exe = path.join(dir, 'custom.exe');
  fs.writeFileSync(exe, 'MZ');

  withEnv({ MAACTL_BINARY: exe }, () => {
    assert.equal(binary.resolveBinary(), exe);
  });
  withEnv({ MAACTL_BINARY: path.join(dir, 'absent.exe') }, () => {
    assert.throws(() => binary.resolveBinary(), /absent\.exe/);
  });
});

test('resolveBinary skips empty files', () => {
  const home = tempDir();
  const { binary: subject } = isolatedPackage();
  withEnv({ MAACTL_HOME: home }, () => {
    const cached = subject.cacheExePath();
    fs.mkdirSync(path.dirname(cached), { recursive: true });
    fs.writeFileSync(cached, '');
    assert.equal(subject.resolveBinary(), null);
  });
});

test('verifyBinary rejects non-PE files', () => {
  const dir = tempDir();
  const bad = path.join(dir, 'maactl.exe');
  fs.writeFileSync(bad, 'not an executable at all');
  assert.throws(() => binary.verifyBinary(bad), /not a Windows PE executable/);
});

test('verifyBinary accepts an executable that reports its version', () => {
  const dir = tempDir();
  const fake = path.join(dir, 'maactl.exe');
  fs.writeFileSync(fake, 'MZ'.padEnd(binary.MIN_BINARY_BYTES, '\0'));

  const calls = [];
  binary.verifyBinary(fake, {
    spawn: (file, args) => {
      calls.push([file, args]);
      return { status: 0, stdout: 'maactl version 1.2.3 (MaaFramework v5.13.0)' };
    },
  });

  assert.deepEqual(calls, [[fake, ['--version']]]);
});

test('verifyBinary surfaces a failing executable', () => {
  const dir = tempDir();
  const fake = path.join(dir, 'maactl.exe');
  fs.writeFileSync(fake, 'MZ'.padEnd(binary.MIN_BINARY_BYTES, '\0'));

  assert.throws(
    () => binary.verifyBinary(fake, { spawn: () => ({ status: 1, stderr: 'boom' }) }),
    /unexpected output: boom/,
  );
});
