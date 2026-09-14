# MaaCtl 的 npm 分发（npx maactl）

## 目标

让 Windows 用户不必手动下载、解压、配置 PATH，就能把 `maactl.exe` 当成普通命令使用：

```powershell
npx maactl -v
npx maactl interface --tasks -f D:\01_Projects\github\MaaMio

npm install -g maactl
maactl -v
```

约束：

1. **参数必须原样透传。** `npx maactl run task "自动挂机卖蛋" -f D:\proj --stop-after 10s` 与
   `.\maactl.exe run task "自动挂机卖蛋" -f D:\proj --stop-after 10s` 完全等价，包括中文参数、
   含空格的路径和 `-`/`--` 前缀的选项。
2. **工作目录不变。** 包装器不改 `cwd`，因此 `maactl` 默认从**调用者所在目录**读取
   `./interface.json`，与直接运行 exe 一致。
3. **退出码原样返回。** 脚本和 CI 可以按 exe 的退出码判断成败。
4. **零运行时依赖。** 只用 Node 内置模块，避免为一个转发器引入依赖树。
5. **离线可用。** 正式发布的 tarball 内自带 `maactl.exe`，安装与运行都不需要额外下载。

## 目录结构

```text
npm/
  package.json            bin/maactl、files、postinstall、os: win32
  README.md               发布到 npm 的说明（npx 用法、环境变量）
  bin/maactl.js           入口：npm 在 Windows 上生成的 maactl.cmd 指向它
  lib/binary.js           maactl.exe 的定位、下载、校验与缓存
  lib/download.js         HTTPS 下载（重定向、重试、进度、截断检测）
  lib/cli.js              解析 exe、spawn、转发参数与信号、返回退出码
  lib/env.js              环境变量读取（MAACTL_QUIET 等 1/true/yes/on 开关）
  lib/messages.js         中英文提示（跟随 MAACTL_LANG）
  scripts/install.js      postinstall：预取 exe，失败只警告
  scripts/vendor-binary.js 把本地/CI 构建的 exe 放进 vendor/ 供发布
  test/                   node:test 单元测试（不联网、不依赖真实 exe）
  test-support/           测试辅助：环境隔离、无 vendor 的包装器副本、本地 HTTP 服务
```

测试用 `isolatedPackage()` 把 `lib/` 与 `package.json` 复制到临时目录再加载，因此开发者本地
用 `npm run vendor:binary` 生成的 `vendor/maactl.exe` 不会让“需要下载”的分支测试失真。

## maactl.exe 的定位顺序

`resolveBinary()` 按以下顺序返回第一个可用文件，全部落空时才进入下载分支：

1. `MAACTL_BINARY` 指定的路径（不存在则直接报错，不回退，避免静默用错版本）；
2. 包内 `vendor/maactl.exe`（正式 tarball 携带，等于「开箱即用」）；
3. `%LOCALAPPDATA%\maactl\npm\<version>\maactl.exe`（上一次安装/运行的下载缓存）；
4. 下载 `https://github.com/<repo>/releases/download/v<version>/maactl.exe`。

缓存目录带版本号，升级包版本不会复用旧 exe；`MAACTL_VERSION` 可以同时改写下载版本和缓存目录，
方便用未发布的版本自测。下载完成后校验 PE 头（`MZ`）、体积下限，并实际执行 `--version`
确认输出里含 `maactl`；任一环节失败即删除文件，绝不缓存半成品。

## 参数与退出码转发

`bin/maactl.js` 把 `process.argv.slice(2)` 交给 `lib/cli.js`，后者用
`spawn(exe, argv, { stdio: 'inherit', cwd: process.cwd(), env: process.env })` 启动 exe：

- `stdio: 'inherit'` 让 exe 直接占用调用者的终端，因此进度条、彩色输出、`--json`、
  管道（`maactl ... | ConvertFrom-Json`）都与直接运行 exe 相同；
- 子进程退出后包装器把退出码写进 `process.exitCode`，不额外调用 `process.exit()`，
  避免截断尚未 flush 的输出；
- POSIX 下转发 `SIGINT/SIGTERM/SIGQUIT/SIGHUP`，子进程被信号杀死时包装器重新抛出同一信号；
- Windows 没有 POSIX 信号，控制台把 Ctrl+C 广播给整个进程组，exe 自己收得到，所以包装器只
  注册一次性空监听器保持存活、等待 exe 优雅退出；再按一次 Ctrl+C 会恢复默认行为，直接终止包装器。

npx 会把包名之后的参数原样传给命令，因此 `npx maactl -v` 不会被 npm 自己吞掉；遇到会拦截参数的
旧版 npx 时可用 `npx maactl -- -v` 显式分隔。

## 包体积与分发取舍

内置 MaaFramework 的 `maactl.exe` 约 33 MiB。发布时把它放进 tarball（`vendor/maactl.exe`）：

- 优点：安装与运行都不依赖 GitHub 可达性；国内网络环境最稳；`npx` 首次执行即可用；
- 代价：npm 上多一份 33 MiB 的包（npm 允许该量级，删掉 `vendor/` 后包体仅约 11 KB）。

不携带 exe 的包（从源码 `npm pack`、或自定义 `MAACTL_ASSET`）会走 postinstall 预下载，
失败只打印警告而不让 `npm install` 失败，首次运行时会再次尝试；`MAACTL_STRICT_INSTALL=1`
可以把预下载失败升级为安装失败。下载在 `MAACTL_MIRROR` 下支持镜像前缀
（`<mirror>/https://github.com/...`），也支持 `MAACTL_BINARY_URL` 直接指定地址。

## 发布流程

npm 发布由可复用工作流 `.github/workflows/npm-publish.yml` 负责，两种触发方式：

1. `workflow_call`：`.github/workflows/release.yml` 的 `npm` 任务在 `release` 任务完成后调用它，
   所以推送 `v*` tag 完成发版后会自动发布 npm 包；
2. `workflow_dispatch`：手动重发 / 补发某个已发布版本。

```powershell
gh workflow run npm-publish.yml -f tag=v0.1.0                  # 补发 / 重发
gh workflow run npm-publish.yml -f tag=v0.1.1 -f dry_run=true   # 只演练，不真正 publish
```

步骤：

1. `actions/checkout` 到该 tag（`fetch-depth: 0`，`release.py` 需要本地 tag 列表）；
2. `python .github/scripts/release.py metadata --tag <tag>` 得到 version / channel / prerelease；
3. `gh release download <tag> --pattern maactl.exe`：直接复用 Release 里那个已在 `build` 中验证过的 exe；
4. `npm version <version> --no-git-tag-version --allow-same-version` 把包版本对齐 tag，并校验
   `maactl.exe --version` 的输出里确实含有该版本号（防止发错 exe）；
5. `npm test` 跑包装器测试；
6. `node scripts/vendor-binary.js ../dist/maactl.exe` 把 exe 放入 `vendor/`（含 PE 头、体积与
   `--version` 输出校验）；
7. 查 npm 上是否已有该版本，已存在则跳过（幂等）；
8. `npm publish --access public --provenance`，正式版打 `latest`，预发布按通道打
   `alpha`/`beta`/`rc` 标签（与 GitHub Release 的 Pre-release 语义一致）。

发布成功后 registry 还要做几十秒到十几分钟的异步处理（30 MB 级的 exe 要扫描，带 provenance 的
版本还要校验 attestation）。这期间 `npm view maactl@<version>` 依旧是 404，日志里会出现
`Your package is being processed and may take a few minutes to become available.`——都属正常，
不要据此判定发布失败，也不要急着重新 dispatch，等几分钟再查 dist-tags 即可：

```powershell
# 发布后的自查
Invoke-RestMethod https://registry.npmjs.org/-/package/maactl/dist-tags
npm view maactl dist-tags --registry=https://registry.npmjs.org

# 校验 provenance 签名（默认源是 npmmirror 时必须显式指定 registry，否则取不到 TUF 公钥）
npm audit signatures --registry=https://registry.npmjs.org
```

为何不是 `on: release: published`：Release 是 `release.yml` 用内置 `GITHUB_TOKEN` 创建的，而 GitHub
不会为 `GITHUB_TOKEN` 导致的事件启动新的工作流，独立监听 Release 的工作流会永远不被触发。因此把发布
逻辑写成可复用工作流，由 `release.yml` 在 Release 建好后直接调用；`workflow_dispatch` 与自动发布走的是
同一份逻辑，手动重发不会出现行为差异。

发布需要仓库配置 `NPM_TOKEN` secret（npm Automation token，具备 publish 权限）：

```powershell
gh secret set NPM_TOKEN -b "<npm token>"
gh secret list
```

未配置时该任务只打印警告并跳过发布，不会失败；目标版本已存在于 npm 时同样跳过而不是报错（npm 不允许
覆盖已发布的版本号）。

`.github/workflows/npm.yml` 在 `npm/**` 变更时跑测试，并在 Node 22 上做一次端到端冒烟：
构建轻量版 exe → `vendor-binary.js` → `npx --package . maactl -v`。

## 本地验证

```powershell
cd npm
npm test                                    # 单元测试，离线可跑
npm run vendor:binary -- ..\maactl.exe      # 把本地构建的 exe 放进 vendor/
npx --yes --package . maactl -v
node bin\maactl.js interface --tasks -f D:\01_Projects\github\MaaMio
```

只想验证参数转发而不重新打包时，用 `MAACTL_BINARY` 直接指向本地构建：

```powershell
$env:MAACTL_BINARY = "D:\01_Projects\github\MaaCtl\maactl.exe"
node npm\bin\maactl.js -v
```
