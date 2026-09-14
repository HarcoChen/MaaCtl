# maactl (npm)

通过 `npx` 运行 MaaCtl 的命令行客户端。这个包只是一个很薄的转发器：它负责找到
`maactl.exe`（自带、缓存或按需下载），把命令行参数原样传给它，并把退出码回传给 shell。

```powershell
# 直接运行，无需全局安装
npx maactl --version
npx maactl adb devices
npx maactl interface --tasks -f D:\01_Projects\github\MaaMio

# 若某些 npx 版本吞掉了参数，用 -- 显式分隔
npx maactl -- run task "自动挂机卖蛋" -f D:\01_Projects\github\MaaMio --stop-after 10s
```

`npx maactl ...` 之后的所有参数都会原样传给 `maactl.exe`，包括 `-f/--interface`、
`--adb-address`、`--json` 等；工作目录也不会被改变，因此默认的 `./interface.json`
仍然相对于你执行命令的目录解析。

## 平台

只提供 Windows amd64 的 `maactl.exe`（自带 MaaFramework 运行库）。包内声明了
`"os": ["win32"]`，在其他系统上 npm 会以 `EBADPLATFORM` 拒绝安装。

## maactl.exe 的来源

按以下顺序查找，第一个命中的会被使用：

1. 环境变量 `MAACTL_BINARY` 指向的文件；
2. 包内的 `vendor/maactl.exe`（正式发布的 tarball 会携带）；
3. 缓存 `%LOCALAPPDATA%\maactl\npm\<version>\maactl.exe`；
4. 都找不到时，从对应版本的 GitHub Release 下载并校验后缓存（下次直接复用）。

## 环境变量

| 变量 | 作用 |
| --- | --- |
| `MAACTL_BINARY` | 指定要执行的 `maactl.exe`，跳过查找与下载。 |
| `MAACTL_HOME` | 覆盖缓存根目录，默认 `%LOCALAPPDATA%\maactl`。 |
| `MAACTL_VERSION` | 覆盖下载时使用的版本，默认取包的 `version`。 |
| `MAACTL_BINARY_URL` | 直接指定下载地址，完全跳过 GitHub 拼接。 |
| `MAACTL_MIRROR` | 镜像前缀，最终地址为 `<mirror>/https://github.com/<repo>/releases/download/<tag>/maactl.exe`。 |
| `MAACTL_REPO` / `MAACTL_ASSET` | 覆盖仓库（默认 `TanyaShue/MaaCtl`）与资产名（默认 `maactl.exe`）。 |
| `MAACTL_SKIP_DOWNLOAD=1` | 安装阶段不预下载，改为首次运行时下载。 |
| `MAACTL_STRICT_INSTALL=1` | 预下载失败时让 `npm install` 直接失败。 |
| `MAACTL_QUIET=1` | 静默模式，不输出下载进度。 |
| `MAACTL_VERBOSE=1` | 输出实际使用的 `maactl.exe` 路径。 |
| `MAACTL_LANG` | 包装器的提示语言（`zh_CN`/`en`），与 CLI 自身一致。 |

## 从源码使用

```powershell
# 直接用 shim，无需安装
node npm/bin/maactl.js --help

# 或在本地安装为全局命令
cd npm
npm install -g .
maactl --version
```

完整的命令说明见 [MaaCtl 主仓库](https://github.com/TanyaShue/MaaCtl#readme)。
