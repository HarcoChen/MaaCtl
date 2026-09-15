#!/usr/bin/env node
'use strict';

// Copy a locally built maactl.exe into vendor/ so `npm pack` / `npm publish`
// produce a tarball that needs no download at install time.
//
// The release workflow calls this with the exe it just built; developers can
// call it with the exe from `go build` to test the packaging flow offline.
//
//   node scripts/vendor-binary.js [path-to-exe]        # default: ../maactl.exe
//   node scripts/vendor-binary.js --check              # verify vendor/maactl.exe
//   node scripts/vendor-binary.js --list               # show the current state

const fs = require('node:fs');
const path = require('node:path');

const { bundledExePath, verifyBinary, EXE_NAME } = require('../lib/binary');
const { formatBytes } = require('../lib/download');

const ROOT = path.resolve(__dirname, '..');

function usage() {
  process.stderr.write(
    [
      `usage: node scripts/vendor-binary.js [<maactl.exe>] [--check | --list]`,
      '',
      `Copies <maactl.exe> (default ${path.join(ROOT, '..', EXE_NAME)}) to ${bundledExePath()}.`,
      'Use --check to verify the vendored executable without copying anything.',
      'Use --list to print the state of the vendored executable without copying anything.',
      '',
    ].join('\n'),
  );
}

/** Print the size and reported version of the vendored executable. */
function report(dest) {
  const version = verifyBinary(dest);
  const size = formatBytes(fs.statSync(dest).size);
  process.stdout.write(`vendored ${dest}\n  ${size}\n  ${version}\n`);
}

function main(argv) {
  const args = argv.filter((arg) => !arg.startsWith('--'));
  const check = argv.includes('--check');
  const source = path.resolve(args[0] || path.join(ROOT, '..', EXE_NAME));
  const dest = bundledExePath();

  if (argv.includes('--help') || argv.includes('-h')) {
    usage();
    return 0;
  }

  // --list is a read-only status query: it never copies and never fails on a
  // missing vendor directory, so it is answered before the copy path is set up.
  if (argv.includes('--list')) {
    if (fs.existsSync(dest)) {
      report(dest);
    } else {
      process.stdout.write(`no vendored executable at ${dest}\n`);
    }
    return 0;
  }

  if (!check) {
    if (!fs.existsSync(source)) {
      throw new Error(`source executable not found: ${source}\nbuild it first: go build -tags bundled -o maactl.exe ./cmd/maactl`);
    }
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    fs.copyFileSync(source, dest);
  }

  if (!fs.existsSync(dest)) {
    throw new Error(`no vendored executable at ${dest}`);
  }

  report(dest);
  return 0;
}

try {
  process.exitCode = main(process.argv.slice(2));
} catch (error) {
  process.stderr.write(`vendor-binary: ${error.message}\n`);
  process.exitCode = 1;
}
