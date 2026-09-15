'use strict';

// The wrapper only prints text for error and progress paths, so the message
// catalogue stays small. Language follows MAACTL_LANG, the same variable the
// CLI itself honours; anything not starting with "zh" falls back to English.

const env = require('./env');

const MESSAGES = {
  zh: {
    downloading: (url) => `maactl: 首次使用，正在下载 maactl.exe\n  ${url}`,
    downloaded: (size, dest) => `maactl: 已下载 ${size} 到 ${dest}`,
    downloadFailed: (url, reason) =>
      `maactl: 下载 maactl.exe 失败\n  ${url}\n  原因: ${reason}`,
    extractFailed: (archive, reason) =>
      `maactl: 解压 release 压缩包失败\n  ${archive}\n  原因: ${reason}`,
    verifyFailed: (file, reason) => `maactl: 下载的 maactl.exe 校验失败 (${file})\n  原因: ${reason}`,
    explicitMissing: (file) => `maactl: MAACTL_BINARY 指向的文件不存在: ${file}`,
    spawnFailed: (file, reason) => `maactl: 无法启动 maactl.exe (${file})\n  原因: ${reason}`,
    resolveHint: () =>
      [
        'maactl: 找不到 maactl.exe。可用的解决办法：',
        '  1. 设置 MAACTL_BINARY 指向本地已有的 maactl.exe；',
        '  2. 检查网络后重试，或设置 MAACTL_MIRROR 使用镜像下载；',
        '  3. 手动下载 release 里的 maactl-<版本>-win-x86_64.zip，解压出 maactl.exe 放到 %LOCALAPPDATA%\\maactl\\npm\\<version>\\ 下。',
      ].join('\n'),
    skippingDownload: () => 'maactl: 已跳过 maactl.exe 下载 (MAACTL_SKIP_DOWNLOAD)',
    bundledMissing: (file) => `maactl: 未找到内置的 ${file}，改为下载 maactl.exe`,
    preDownloadFailed: (reason) => `maactl: 预下载 maactl.exe 失败 (${reason})`,
    preDownloadHint: () =>
      'maactl: 改为首次运行时下载；设置 MAACTL_STRICT_INSTALL=1 可让安装在此处直接失败。',
    usingBinary: (file) => `maactl: 使用 ${file}`,
  },
  en: {
    downloading: (url) => `maactl: downloading maactl.exe for first use\n  ${url}`,
    downloaded: (size, dest) => `maactl: downloaded ${size} to ${dest}`,
    downloadFailed: (url, reason) => `maactl: could not download maactl.exe\n  ${url}\n  reason: ${reason}`,
    extractFailed: (archive, reason) => `maactl: could not unpack the release archive\n  ${archive}\n  reason: ${reason}`,
    verifyFailed: (file, reason) => `maactl: the downloaded maactl.exe failed verification (${file})\n  reason: ${reason}`,
    explicitMissing: (file) => `maactl: MAACTL_BINARY points at a missing file: ${file}`,
    spawnFailed: (file, reason) => `maactl: could not start maactl.exe (${file})\n  reason: ${reason}`,
    resolveHint: () =>
      [
        'maactl: maactl.exe could not be located. Things to try:',
        '  1. set MAACTL_BINARY to an existing maactl.exe;',
        '  2. check your network and retry, or set MAACTL_MIRROR to use a mirror;',
        '  3. download maactl-<version>-win-x86_64.zip from the release page, unpack maactl.exe, and place it in %LOCALAPPDATA%\\maactl\\npm\\<version>\\.',
      ].join('\n'),
    skippingDownload: () => 'maactl: skipped downloading maactl.exe (MAACTL_SKIP_DOWNLOAD)',
    bundledMissing: (file) => `maactl: no bundled ${file}; fetching maactl.exe instead`,
    preDownloadFailed: (reason) => `maactl: could not pre-download maactl.exe (${reason})`,
    preDownloadHint: () =>
      'maactl: it will be fetched on first run instead; set MAACTL_STRICT_INSTALL=1 to fail the install here.',
    usingBinary: (file) => `maactl: using ${file}`,
  },
};

function language({ platform = process.platform } = {}) {
  const explicit = env.value('MAACTL_LANG').toLowerCase();
  if (explicit) {
    return explicit.startsWith('zh') ? 'zh' : 'en';
  }
  if (platform !== 'win32') {
    const posix = `${env.value('LC_ALL')}${env.value('LC_MESSAGES')}${env.value('LANG')}`.toLowerCase();
    if (posix) {
      return posix.includes('zh') ? 'zh' : 'en';
    }
  }
  let locale = '';
  try {
    locale = Intl.DateTimeFormat().resolvedOptions().locale || '';
  } catch {
    locale = '';
  }
  return locale.toLowerCase().startsWith('zh') ? 'zh' : 'en';
}

/** Return the localized message for `key`, formatted with `args`. */
function text(key, ...args) {
  const catalogue = MESSAGES[language()] || MESSAGES.en;
  const entry = catalogue[key] || MESSAGES.en[key];
  return typeof entry === 'function' ? entry(...args) : String(entry ?? '');
}

module.exports = { text, language };
