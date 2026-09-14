#!/usr/bin/env node
'use strict';

// postinstall: pre-fetch maactl.exe into the user cache.
//
// The published tarball already carries vendor/maactl.exe, so this only runs
// for installs without the bundled executable (git checkouts, `npm pack` from
// source, custom MAACTL_ASSET builds). A download failure must never break
// `npm install`: the shim retries lazily on first use. Set
// MAACTL_STRICT_INSTALL=1 to turn the warning into an error.

const { ensureBinary, resolveBinary, bundledExePath } = require('../lib/binary');
const env = require('../lib/env');
const messages = require('../lib/messages');

function write(line) {
  process.stderr.write(`${line}\n`);
}

async function install() {
  if (env.flag('MAACTL_SKIP_DOWNLOAD')) {
    write(messages.text('skippingDownload'));
    return;
  }

  // resolveBinary() may throw when MAACTL_BINARY is set to a missing path, and
  // that is a configuration error the user should see right away.
  if (resolveBinary()) {
    return; // bundled or cached executable is already in place
  }

  try {
    write(messages.text('bundledMissing', bundledExePath()));
    await ensureBinary({ write, quiet: false });
  } catch (error) {
    const strict = env.flag('MAACTL_STRICT_INSTALL');
    if (strict) {
      throw error;
    }
    write(messages.text('preDownloadFailed', error.message));
    write(messages.text('preDownloadHint'));
  }
}

install().catch((error) => {
  write(`maactl: install step failed: ${error.message}`);
  process.exitCode = 1;
});
