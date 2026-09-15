'use strict';

// Minimal HTTPS downloader for the maactl release archive.
//
// Node built-ins only: the npm package ships no runtime dependencies. GitHub
// release downloads answer with a 302 to objects.githubusercontent.com, so
// redirects are followed manually and the Authorization header (when a
// GH_TOKEN is provided to dodge rate limits) is dropped on cross-host hops.

const fs = require('node:fs');
const http = require('node:http');
const https = require('node:https');

const MAX_REDIRECTS = 5;
const REQUEST_TIMEOUT_MS = 60_000;
// A slow-but-alive transfer is fine; a silent socket is not. If no bytes arrive
// for this long the download is aborted with a readable error.
const STALL_TIMEOUT_MS = 120_000;

const REDIRECT_CODES = new Set([301, 302, 303, 307, 308]);

function userAgent() {
  const pkg = require('../package.json');
  return `maactl-npm/${pkg.version} (node/${process.versions.node}; ${process.platform})`;
}

function githubToken() {
  return (process.env.GH_TOKEN || process.env.GITHUB_TOKEN || '').trim();
}

function baseHeaders(url) {
  const headers = {
    'user-agent': userAgent(),
    accept: 'application/octet-stream',
  };
  const token = githubToken();
  if (token && /(^|\.)github\.com$/i.test(new URL(url).hostname)) {
    headers.authorization = `Bearer ${token}`;
  }
  return headers;
}

function requestOnce(url, headers) {
  return new Promise((resolve, reject) => {
    const target = new URL(url);
    const transport = target.protocol === 'http:' ? http : https;
    const request = transport.get(target, { headers, timeout: REQUEST_TIMEOUT_MS }, (response) => {
      response.setTimeout(STALL_TIMEOUT_MS);
      response.on('timeout', () => {
        response.destroy(new Error(`no data received for ${STALL_TIMEOUT_MS} ms`));
      });
      resolve(response);
    });
    request.on('timeout', () => request.destroy(new Error(`request timed out after ${REQUEST_TIMEOUT_MS} ms`)));
    request.on('error', reject);
  });
}

function writeBody(response, dest, onProgress) {
  return new Promise((resolve, reject) => {
    const total = Number(response.headers['content-length'] || 0);
    let received = 0;
    let finished = false;
    const settle = (fn, value) => {
      if (!finished) {
        finished = true;
        fn(value);
      }
    };
    const output = fs.createWriteStream(dest);

    response.on('data', (chunk) => {
      received += chunk.length;
      if (onProgress) {
        onProgress(received, total);
      }
    });
    response.on('error', (error) => settle(reject, error));
    output.on('error', (error) => settle(reject, error));
    output.on('finish', () => {
      if (total > 0 && received !== total) {
        settle(reject, new Error(`truncated download: received ${received} of ${total} bytes`));
        return;
      }
      settle(resolve, received);
    });
    response.pipe(output);
  });
}

/**
 * Download `url` into `dest` (written to `dest.part` first, renamed on
 * success). `onProgress(received, total)` is called for every chunk.
 */
async function download(url, dest, { onProgress } = {}) {
  const partial = `${dest}.part`;
  const headers = baseHeaders(url);
  let current = url;

  try {
    for (let hop = 0; hop <= MAX_REDIRECTS; hop += 1) {
      const response = await requestOnce(current, headers);
      const status = response.statusCode || 0;

      if (REDIRECT_CODES.has(status)) {
        const location = response.headers.location;
        response.resume();
        if (!location) {
          throw new Error(`HTTP ${status} without a Location header`);
        }
        const next = new URL(location, current);
        if (next.hostname !== new URL(current).hostname) {
          delete headers.authorization;
        }
        current = next.href;
        continue;
      }

      if (status !== 200) {
        response.resume();
        throw new Error(`HTTP ${status} ${response.statusMessage || ''}`.trim());
      }

      await writeBody(response, partial, onProgress);
      fs.renameSync(partial, dest);
      return dest;
    }
    throw new Error(`too many redirects (limit ${MAX_REDIRECTS})`);
  } catch (error) {
    try {
      fs.rmSync(partial, { force: true });
    } catch {
      // Best effort: a stale .part file only costs disk space.
    }
    throw error;
  }
}

/** Try `download` up to `attempts` times, backing off between attempts. */
async function downloadWithRetry(url, dest, options = {}) {
  const attempts = options.attempts || 3;
  const delay = options.retryDelayMs ?? 1_000;
  let lastError;
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      return await download(url, dest, options);
    } catch (error) {
      lastError = error;
      if (attempt < attempts && options.onRetry) {
        options.onRetry(attempt, attempts, error);
      }
      if (attempt < attempts) {
        await new Promise((resolve) => setTimeout(resolve, delay * attempt));
      }
    }
  }
  throw lastError;
}

/** Human readable byte count, e.g. 34.2 MiB. */
function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B';
  }
  const units = ['B', 'KiB', 'MiB', 'GiB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

/** Render a single-line progress indicator to stderr. */
function progressReporter(stream = process.stderr) {
  let lastLength = 0;
  return (received, total) => {
    const line = total > 0
      ? `  ${formatBytes(received)} / ${formatBytes(total)} (${Math.floor((received / total) * 100)}%)`
      : `  ${formatBytes(received)}`;
    const padding = ' '.repeat(Math.max(0, lastLength - line.length));
    lastLength = line.length;
    stream.write(`\r${line}${padding}`);
  };
}

module.exports = { download, downloadWithRetry, formatBytes, progressReporter, userAgent };
