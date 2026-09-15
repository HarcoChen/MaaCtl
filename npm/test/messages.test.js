'use strict';

const assert = require('node:assert/strict');
const { test } = require('node:test');

const messages = require('../lib/messages');
const { withEnv } = require('../test-support/helpers');

test('MAACTL_LANG selects the language and rejects unknown ones', () => {
  withEnv({ MAACTL_LANG: 'zh_CN' }, () => assert.equal(messages.language(), 'zh'));
  withEnv({ MAACTL_LANG: 'ZH' }, () => assert.equal(messages.language(), 'zh'));
  withEnv({ MAACTL_LANG: ' en_US ' }, () => assert.equal(messages.language(), 'en'));
  withEnv({ MAACTL_LANG: 'ja_JP' }, () => assert.equal(messages.language(), 'en'));
});

test('a POSIX locale decides when MAACTL_LANG is unset', () => {
  withEnv({ LC_ALL: 'zh_CN.UTF-8' }, () => {
    assert.equal(messages.language({ platform: 'linux' }), 'zh');
  });
  withEnv({ LANG: 'zh_TW.UTF-8' }, () => {
    assert.equal(messages.language({ platform: 'darwin' }), 'zh');
  });
  withEnv({ LC_MESSAGES: 'en_US.UTF-8' }, () => {
    assert.equal(messages.language({ platform: 'linux' }), 'en');
  });
});

test('the system locale is the last resort and always answers', () => {
  withEnv({}, () => {
    assert.ok(['zh', 'en'].includes(messages.language()));
  });
});

test('text formats the message of the selected language', () => {
  const file = 'C:\\cache\\maactl.exe';
  withEnv({ MAACTL_LANG: 'en' }, () => {
    assert.equal(messages.text('explicitMissing', file), `maactl: MAACTL_BINARY points at a missing file: ${file}`);
  });
  withEnv({ MAACTL_LANG: 'zh' }, () => {
    assert.equal(messages.text('explicitMissing', file), `maactl: MAACTL_BINARY 指向的文件不存在: ${file}`);
  });
});

test('text falls back to English for a language without a catalogue', () => {
  withEnv({ MAACTL_LANG: 'ja_JP' }, () => {
    assert.equal(messages.text('explicitMissing', 'x.exe'), 'maactl: MAACTL_BINARY points at a missing file: x.exe');
  });
});

test('text returns an empty string for an unknown key', () => {
  withEnv({ MAACTL_LANG: 'en' }, () => {
    assert.equal(messages.text('noSuchMessage'), '');
  });
});
