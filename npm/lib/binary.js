'use strict';

// Locating (and if necessary fetching) the maactl.exe the wrapper hands over to.
//
// Resolution order:
//   1. MAACTL_BINARY            – an explicit path supplied by the user;
//   2. <package>/vendor/maactl.exe – the executable published inside the tarball;
//   3. <cache>/npm/<version>/maactl.exe – a previous download, reused across runs.
//
// When none of them exists the release archive of this platform is downloaded
// from the GitHub release that matches the package version, maactl.exe is
// unpacked out of it, and the result is verified and cached for the next run.

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const { downloadWithRetry, formatBytes, progressReporter } = require('./download');
const env = require('./env');
const { MaactlError } = require('./errors');
const messages = require('./messages');
const zip = require('./zip');

const PACKAGE_ROOT = path.resolve(__dirname, '..');
const PACKAGE = require(path.join(PACKAGE_ROOT, 'package.json'));

// Every asset is a Windows PE binary; anything smaller than this floor is a
// truncated download, an HTML error page or a git-lfs pointer.
const MIN_BINARY_BYTES = 1 << 20;

const EXE_NAME = 'maactl.exe';
const DEFAULT_REPO = 'TanyaShue/MaaCtl';
// The wrapper is Windows-only (package.json restricts "os" to win32), so it
// always takes the win-x86_64 archive of the release.
const DEFAULT_PLATFORM = 'win-x86_64';

function version() {
  return (env.value('MAACTL_VERSION') || PACKAGE.version).replace(/^v/, '');
}

function tag() {
  return `v${version()}`;
}

function repo() {
  return env.value('MAACTL_REPO') || DEFAULT_REPO;
}

function platform() {
  return env.value('MAACTL_PLATFORM') || DEFAULT_PLATFORM;
}

/**
 * The release asset the executable comes in: one archive per platform, holding
 * both the self-contained and the lite executable.
 */
function assetName() {
  return env.value('MAACTL_ASSET') || `maactl-${version()}-${platform()}.zip`;
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

/**
 * Where the release archive is unpacked. It sits next to the cached executable
 * and is deleted as soon as the executable is written out.
 */
function archivePath(exePath = cacheExePath()) {
  return path.join(path.dirname(exePath), assetName());
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
 * Delete a temporary file without letting a cleanup failure reach the caller.
 *
 * Windows keeps a file locked while a virus scanner or the search indexer reads
 * it, and an EPERM from `rmSync` inside a `finally` would replace whatever the
 * try block decided, turning a successful first run into a failed one.
 */
function removeQuietly(file) {
  try {
    fs.rmSync(file, { force: true });
  } catch {
    // A file that stays behind only costs disk space and is rewritten next time.
  }
}

/**
 * Unpack maactl.exe out of the downloaded release archive and write it to dest.
 */
function writeExecutable(archive, dest) {
  const data = fs.readFileSync(archive);
  const payload = zip.extractEntry(data, EXE_NAME);
  if (!payload) {
    throw new Error(`${archive} holds no ${EXE_NAME} (contains: ${zip.entryNames(data).join(', ') || 'nothing'})`);
  }
  fs.writeFileSync(dest, payload);
}

/**
 * Download the release archive and cache the executable inside it, unless a
 * local copy already exists. `write(text)` receives human readable progress
 * lines; pass `quiet` to suppress them.
 */
async function ensureBinary({ write = () => {}, quiet = false, verify = verifyBinary } = {}) {
  const existing = resolveBinary();
  if (existing) {
    return existing;
  }

  const dest = cacheExePath();
  const archive = archivePath(dest);
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
    await downloadWithRetry(url, archive, {
      onProgress,
      onRetry: (attempt, attempts, error) => {
        if (!quiet) {
          write(`maactl: retrying download (${attempt}/${attempts - 1}) after: ${error.message}`);
        }
      },
    });
  } catch (error) {
    if (!quiet && onProgress) {
      process.stderr.write('\n');
    }
    throw new MaactlError(messages.text('downloadFailed', url, error.message), { cause: error });
  }
  if (onProgress) {
    process.stderr.write('\n');
  }

  try {
    writeExecutable(archive, dest);
  } catch (error) {
    throw new MaactlError(messages.text('extractFailed', archive, error.message), { cause: error });
  } finally {
    // The archive is only ever needed to produce the executable; keeping it
    // would double the cache size for nothing.
    removeQuietly(archive);
  }
  try {
    verify(dest);
  } catch (error) {
    removeQuietly(dest);
    throw error;
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
  platform,
  assetName,
  home,
  cacheExePath,
  archivePath,
  bundledExePath,
  binaryUrl,
  resolveBinary,
  ensureBinary,
  verifyBinary,
  verbose,
};
