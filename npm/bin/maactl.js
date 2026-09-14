#!/usr/bin/env node
'use strict';

// npm installs this file as the `maactl` command on Windows
// (maactl.cmd / maactl.ps1 wrappers), so `npx maactl <args>` ends up here and
// the arguments are forwarded to maactl.exe verbatim.

const { main } = require('../lib/cli');
const { MaactlError } = require('../lib/errors');

main()
  .then((code) => {
    process.exitCode = code;
  })
  .catch((error) => {
    const message = error instanceof MaactlError ? error.message : (error && error.stack) || String(error);
    process.stderr.write(`${message}\n`);
    process.exitCode = error instanceof MaactlError ? error.exitCode : 1;
  });
