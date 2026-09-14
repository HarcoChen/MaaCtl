'use strict';

// Locating (and if necessary fetching) the maactl.exe the wrapper hands over to.
//
// Resolution order:
//   1. MAACTL_BINARY            – an explicit path supplied by the user;
//   2. <package>/vendor/maactl.exe – the executable published inside the tarball;
//   3. <cache>/npm/<version>/maactl.exe – a previous download, reused across runs.
//
// When none of them exists the exe is downloaded from the GitHub release that
// matches the package version, verified, and cached for the next run.

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const { downloadWithRetry, formatBytes, progressReporter } = require('./download');
const env = require('./env');
const { MaactlError } = require('./errors');
const messages = require('./messages');

const PACKAGE_ROOT = path.resolve(__dirname, '..');
const PACKAGE = require(path.join(PACKAGE_ROOT, 'package.json'));

// Every asset is a Windows PE binary; anything smaller than this floor is a
// truncated download, an HTML error page or a git-lfs pointer.
const MIN_BINARY_BYTES = 1 << 20;

const EXE_NAME = 'maactl.exe';
const DEFAULT_REPO = 'TanyaShue/MaaCtl';
const DEFAULT_ASSET = 'maactl.exe';

function version() {
  return (env.value('MAACTL_VERSION') || PACKAGE.version).replace(/^v/, '');
}

function tag() {
  return `v${version()}`;
}

function repo() {
  return env.value('MAACTL_REPO') || DEFAULT_REPO;
}

function assetName() {
  return env.value('MAACTL_ASSET') || DEFAULT_ASSET;
}

/** Cache root mirroring the CLI's own layout: %LOCALAPPDATA%\maactl on Windows. */
function home() {
  const explicit = env.value('MAACTL_HOME');
  if (explicit) {
    return path.resolve(explicit);
  }
  if (process.platform === 'win32') {
    const localAppData = env.value('LOCALAPPDATA') || path.join(os.homedir(), 'AppData', 'Local');
    return path.join(localAppData, 'maactl');
  }
  const cacheHome = env.value('XDG_CACHE_HOME') || path.join(os.homedir(), '.cache');
  return path.join(cacheHome, 'maactl');
}

function cacheExePath(versionOverride) {
  return path.join(home(), 'npm', versionOverride || version(), EXE_NAME);
}

function bundledExePath() {
  return path.join(PACKAGE_ROOT, 'vendor', EXE_NAME);
}

function binaryUrl() {
  const direct = env.value('MAACTL_BINARY_URL');
  if (direct) {
    return direct;
  }
  const github = `https://github.com/${repo()}/releases/download/${tag()}/${assetName()}`;
  const mirror = env.value('MAACTL_MIRROR').replace(/\/+$/, '');
  return mirror ? `${mirror}/${github}` : github;
}

function isUsable(file) {
  try {
    return fs.statSync(file).size > 0;
  } catch {
    return false;
  }
}

/**
 * Return the path of an existing maactl.exe without touching the network, or
 * `null` when nothing is available locally.
 */
function resolveBinary() {
  const explicit = env.value('MAACTL_BINARY');
  if (explicit) {
    const resolved = path.resolve(explicit);
    if (!fs.existsSync(resolved)) {
      throw new MaactlError(messages.text('explicitMissing', resolved));
    }
    return resolved;
  }

  return [bundledExePath(), cacheExePath()].find(isUsable) || null;
}

/** Sanity-check a freshly downloaded executable by asking it for its version. */
function verifyBinary(file, { spawn = spawnSync } = {}) {
  const header = Buffer.alloc(2);
  const fd = fs.openSync(file, 'r');
  try {
    fs.readSync(fd, header, 0, 2, 0);
  } finally {
    fs.closeSync(fd);
  }
  if (header.toString('latin1') !== 'MZ') {
    throw new MaactlError(messages.text('verifyFailed', file, 'not a Windows PE executable'));
  }
  if (fs.statSync(file).size < MIN_BINARY_BYTES) {
    throw new MaactlError(messages.text('verifyFailed', file, 'file is truncated'));
  }

  const result = spawn(file, ['--version'], {
    encoding: 'utf8',
    timeout: 60_000,
    windowsHide: true,
  });
  const output = `${result.stdout || ''}${result.stderr || ''}`.trim();
  if (result.error) {
    throw new MaactlError(messages.text('verifyFailed', file, result.error.message), { cause: result.error });
  }
  if (result.status !== 0 || !/maactl/i.test(output)) {
    throw new MaactlError(
      messages.text('verifyFailed', file, `unexpected output: ${output || '(empty)'}`),
    );
  }
  return output;
}

/**
 * Download maactl.exe into the cache unless a local copy already exists.
 * `write(text)` receives human readable progress lines; pass `quiet` to
 * suppress them.
 */
async function ensureBinary({ write = () => {}, quiet = false, verify = verifyBinary } = {}) {
  const existing = resolveBinary();
  if (existing) {
    return existing;
  }

  const dest = cacheExePath();
  try {
    fs.mkdirSync(path.dirname(dest), { recursive: true });
  } catch (error) {
    throw new MaactlError(`${messages.text('downloadFailed', binaryUrl(), error.message)}\n  cache: ${dest}`, {
      cause: error,
    });
  }

  const url = binaryUrl();
  const showProgress = !quiet && Boolean(process.stderr.isTTY);
  if (!quiet) {
    write(messages.text('downloading', url));
  }

  const onProgress = showProgress ? progressReporter() : undefined;
  try {
    await downloadWithRetry(url, dest, {
      onProgress,
      onRetry: (attempt, attempts, error) => {
        if (!quiet) {
          write(`maactl: retrying download (${attempt}/${attempts - 1}) after: ${error.message}`);
        }
      },
    });
    if (onProgress) {
      process.stderr.write('\n');
    }
    try {
      verify(dest);
    } catch (error) {
      fs.rmSync(dest, { force: true });
      throw error;
    }
  } catch (error) {
    if (error instanceof MaactlError) {
      throw error;
    }
    throw new MaactlError(messages.text('downloadFailed', url, error.message), { cause: error });
  }

  const size = formatBytes(fs.statSync(dest).size);
  if (!quiet) {
    write(messages.text('downloaded', size, dest));
  }
  return dest;
}

/** Print the exe the wrapper will hand over to, when MAACTL_VERBOSE is set. */
function verbose() {
  return env.flag('MAACTL_VERBOSE');
}

module.exports = {
  PACKAGE,
  PACKAGE_ROOT,
  MIN_BINARY_BYTES,
  EXE_NAME,
  version,
  tag,
  repo,
  assetName,
  home,
  cacheExePath,
  bundledExePath,
  binaryUrl,
  resolveBinary,
  ensureBinary,
  verifyBinary,
  verbose,
};
