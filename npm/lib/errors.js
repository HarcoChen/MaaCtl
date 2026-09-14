'use strict';

/**
 * Error type used for every failure raised by the npm wrapper itself.
 *
 * The wrapper never relays these to maactl.exe: they describe problems with
 * locating, downloading or starting the executable.
 */
class MaactlError extends Error {
  constructor(message, { exitCode = 1, cause } = {}) {
    super(message, cause ? { cause } : undefined);
    this.name = 'MaactlError';
    this.exitCode = exitCode;
  }
}

module.exports = { MaactlError };
