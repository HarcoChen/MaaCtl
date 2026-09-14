'use strict';

// Small helpers so every module reads the environment the same way.

const TRUTHY = new Set(['1', 'true', 'yes', 'on']);

/** Trimmed value of an environment variable ('' when unset). */
function value(name) {
  return (process.env[name] || '').trim();
}

/** True when the variable is set to 1/true/yes/on (regardless of case). */
function flag(name) {
  return TRUTHY.has(value(name).toLowerCase());
}

module.exports = { value, flag };
