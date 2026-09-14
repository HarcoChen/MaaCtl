'use strict';

// Thin launcher: hand every argument over to maactl.exe untouched, inherit the
// stdio of the calling shell, and relay the exit status back to npm/npx.

const { spawn } = require('node:child_process');

const { resolveBinary, ensureBinary, verbose } = require('./binary');
const env = require('./env');
const { MaactlError } = require('./errors');
const messages = require('./messages');

// Signals worth relaying on POSIX hosts. On Windows the console broadcasts
// Ctrl+C to the whole process group, so the Go process receives it directly.
const POSIX_SIGNALS = ['SIGINT', 'SIGTERM', 'SIGQUIT', 'SIGHUP'];

function describeSpawnError(file, error) {
  const reason = error && error.message ? error.message : String(error);
  return new MaactlError(messages.text('spawnFailed', file, reason), { cause: error });
}

/**
 * Run `exe` with `argv`, inheriting stdio.
 *
 * Resolves with the numeric exit code of the child. When the child is killed
 * by a signal on POSIX the shim re-raises that signal so the caller sees the
 * same failure it would have seen without the wrapper.
 */
function runBinary(exe, argv, options = {}) {
  const spawnImpl = options.spawn || spawn;
  const proc = options.process || process;
  const platform = options.platform || process.platform;
  const child = spawnImpl(exe, argv, {
    stdio: options.stdio || 'inherit',
    cwd: options.cwd || process.cwd(),
    env: options.env || process.env,
    windowsHide: false,
  });

  return new Promise((resolve, reject) => {
    const listeners = [];
    const cleanup = () => {
      for (const [signal, handler] of listeners) {
        proc.removeListener(signal, handler);
      }
      listeners.length = 0;
    };

    child.once('error', (error) => {
      cleanup();
      reject(describeSpawnError(exe, error));
    });
    child.once('exit', (code, signal) => {
      cleanup();
      if (signal) {
        try {
          proc.kill(proc.pid, signal);
        } catch {
          // Re-raising is a courtesy; fall through to a generic failure code.
        }
        resolve(1);
        return;
      }
      resolve(code === null || code === undefined ? 1 : code);
    });

    const forward = (signal) => {
      if (child.exitCode !== null || child.signalCode !== null) {
        return;
      }
      try {
        child.kill(signal);
      } catch {
        // The child already exited; its exit handler reports the status.
      }
    };

    if (platform === 'win32') {
      // Windows has no POSIX signals: the console broadcasts Ctrl+C/Ctrl+Break
      // to the whole process group, so maactl.exe already received it. Swallowing
      // the first signal keeps the shim alive long enough to report the child's
      // exit code; a second Ctrl+C falls back to the default behaviour and kills
      // the shim, which is the escape hatch when the child will not stop.
      const swallowOnce = () => cleanup();
      for (const signal of ['SIGINT', 'SIGBREAK']) {
        try {
          proc.on(signal, swallowOnce);
          listeners.push([signal, swallowOnce]);
        } catch {
          // Signal not supported on this platform.
        }
      }
    } else {
      for (const signal of POSIX_SIGNALS) {
        const handler = () => forward(signal);
        try {
          proc.on(signal, handler);
          listeners.push([signal, handler]);
        } catch {
          // Signal not supported on this platform.
        }
      }
    }
  });
}

/**
 * Resolve (downloading once if needed) and run maactl.exe with `argv`.
 *
 * `argv` is forwarded verbatim, so `npx maactl run task "自动挂机卖蛋" -f D:\proj`
 * reaches the executable exactly as typed.
 */
async function main(argv = process.argv.slice(2), options = {}) {
  const write = options.write || ((line) => process.stderr.write(`${line}\n`));
  const resolve = options.resolveBinary || resolveBinary;
  const ensure = options.ensureBinary || ensureBinary;
  const quiet = options.quiet ?? env.flag('MAACTL_QUIET');

  let exe = options.binary || (await resolve());
  if (!exe) {
    try {
      exe = await ensure({ write, quiet });
    } catch (error) {
      const detail = error instanceof MaactlError ? error.message : String(error && error.message);
      throw new MaactlError(`${detail}\n${messages.text('resolveHint')}`, { cause: error });
    }
  }

  if (verbose() && !options.binary) {
    write(messages.text('usingBinary', exe));
  }

  return runBinary(exe, argv, options);
}

module.exports = { main, runBinary };
