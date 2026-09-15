'use strict';

const assert = require('node:assert/strict');
const { test } = require('node:test');

const env = require('../lib/env');
const { withEnv } = require('../test-support/helpers');

test('value trims the variable and yields an empty string when it is unset', () => {
  withEnv({ MAACTL_REPO: '  someone/fork  ' }, () => {
    assert.equal(env.value('MAACTL_REPO'), 'someone/fork');
    assert.equal(env.value('MAACTL_ASSET'), '');
  });
  withEnv({ MAACTL_REPO: '   ' }, () => {
    assert.equal(env.value('MAACTL_REPO'), '', 'whitespace alone counts as unset');
  });
});

test('value reads any variable name, not just the documented ones', () => {
  withEnv({ GH_TOKEN: ' token ' }, () => {
    assert.equal(env.value('GH_TOKEN'), 'token');
  });
});

test('flag accepts 1/true/yes/on in any case', () => {
  for (const truthy of ['1', 'true', 'TRUE', 'True', 'yes', 'YES', 'on', 'On', ' on ']) {
    withEnv({ MAACTL_VERBOSE: truthy }, () => {
      assert.equal(env.flag('MAACTL_VERBOSE'), true, `${JSON.stringify(truthy)} is truthy`);
    });
  }
});

test('flag rejects every other value, including an unset variable', () => {
  for (const falsy of ['0', 'false', 'no', 'off', 'y', 'yes!']) {
    withEnv({ MAACTL_VERBOSE: falsy }, () => {
      assert.equal(env.flag('MAACTL_VERBOSE'), false, `${JSON.stringify(falsy)} is falsey`);
    });
  }
  withEnv({}, () => {
    assert.equal(env.flag('MAACTL_VERBOSE'), false);
  });
});
