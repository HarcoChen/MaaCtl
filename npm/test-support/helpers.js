'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const PACKAGE_ROOT = path.resolve(__dirname, '..');

const ENV_KEYS = [
  'MAACTL_BINARY',
  'MAACTL_HOME',
  'MAACTL_VERSION',
  'MAACTL_BINARY_URL',
  'MAACTL_MIRROR',
  'MAACTL_REPO',
  'MAACTL_ASSET',
  'MAACTL_SKIP_DOWNLOAD',
  'MAACTL_STRICT_INSTALL',
  'MAACTL_QUIET',
  'MAACTL_VERBOSE',
  'MAACTL_LANG',
  'GH_TOKEN',
  'GITHUB_TOKEN',
  'LOCALAPPDATA',
  'XDG_CACHE_HOME',
];

function tempDir(prefix = 'maactl-npm-test-') {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

/**
 * Clear the wrapper's environment variables, apply `values`, and return a
 * function that puts the previous values back.
 */
function swapEnv(values) {
  const saved = new Map(ENV_KEYS.map((key) => [key, process.env[key]]));
  for (const key of ENV_KEYS) {
    delete process.env[key];
  }
  Object.assign(process.env, values);
  return () => {
    for (const [key, value] of saved) {
      if (value === undefined) {
        delete process.env[key];
      } else {
        process.env[key] = value;
      }
    }
  };
}

/** Run `body` with the wrapper's environment variables cleared, restoring them afterwards. */
function withEnv(values, body) {
  const restore = swapEnv(values);
  try {
    return body();
  } finally {
    restore();
  }
}

/** `withEnv` for async bodies: clears and restores around an await. */
async function withEnvAsync(values, body) {
  const restore = swapEnv(values);
  try {
    return await body();
  } finally {
    restore();
  }
}

/**
 * Load a private copy of the wrapper in a fresh directory.
 *
 * Tests that exercise `resolveBinary()` / `ensureBinary()` must not be affected
 * by a `vendor/maactl.exe` sitting in the working copy (developers create one
 * with `npm run vendor:binary`), so they run against a copy that is guaranteed
 * to have no vendored executable. Each call returns a distinct module instance.
 */
function isolatedPackage({ vendor } = {}) {
  const root = tempDir('maactl-npm-pkg-');
  fs.cpSync(path.join(PACKAGE_ROOT, 'lib'), path.join(root, 'lib'), { recursive: true });
  fs.copyFileSync(path.join(PACKAGE_ROOT, 'package.json'), path.join(root, 'package.json'));
  if (vendor) {
    fs.mkdirSync(path.join(root, 'vendor'), { recursive: true });
    fs.writeFileSync(path.join(root, 'vendor', 'maactl.exe'), vendor);
  }
  return { root, binary: require(path.join(root, 'lib', 'binary.js')) };
}

/** Serve `routes` (path -> handler) on a loopback port, then close it. */
async function withServer(routes, body) {
  const http = require('node:http');
  const server = http.createServer((request, response) => {
    const handler = routes[request.url];
    if (!handler) {
      response.writeHead(404).end('not found');
      return;
    }
    handler(request, response);
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  try {
    return await body(base, server);
  } finally {
    // Keep-alive sockets would otherwise keep server.close() pending forever.
    server.closeAllConnections();
    await new Promise((resolve) => server.close(resolve));
  }
}

module.exports = { PACKAGE_ROOT, ENV_KEYS, tempDir, withEnv, withEnvAsync, isolatedPackage, withServer };
