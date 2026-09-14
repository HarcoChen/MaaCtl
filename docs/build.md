# 构建与打包

本文面向开发者：如何从源码构建 `maactl`、如何把 MaaFramework 打包进 exe，以及运行库的查找顺序。
只想使用 `maactl` 的话看 [README](../README.md) 即可。

## 构建两种 exe

同一个源码树通过 `bundled` 编译标签产出两种可执行文件：

```powershell
# 自带 MaaFramework（自包含，开箱即用）：先生成 payload，再用 -tags bundled 构建
go run ./tools/packmaafw
go build -tags bundled -o maactl.exe ./cmd/maactl

# 轻量版：不内嵌，运行时从 ./maafw/bin 或 --lib-dir 加载
go build -o maactl-lite.exe ./cmd/maactl

# 运行全部测试（两种构建各跑一遍）
go test ./...
go test -tags bundled ./...

# npm 转发器（npx maactl）的测试，不联网、不依赖 exe
cd npm; npm test; cd ..
```

自带 MaaFramework 的构建约 31 MiB，轻量版约 5 MiB；两者的命令行行为完全一致，区别只在运行库来源。

## 运行库查找顺序

`maactl` 按以下顺序定位 MaaFramework 运行库，第一个命中的生效：

1. `--lib-dir`（`-l`）显式指定的目录；
2. 内嵌在 exe 中的自带运行库（仅 `-tags bundled` 构建）；
3. 当前目录的 `./maafw/bin/`；
4. `maactl.exe` 周边的 `maafw/bin`。

也就是说：自包含构建默认使用自带运行库，除非用户显式指定 `--lib-dir`；轻量版则按上述路径查找。
`--interface`（`-f`）只影响 PI 的加载位置，与运行库无关。

## 自带 MaaFramework（打包）

`tools/packmaafw` 从 MaaFramework 官方 release 压缩包里**只取 `bin/` 目录**（docs、sample、
include、tools、symbol 全部丢弃），重新压缩成 `internal/maafw/bundled/payload/bin.zip` 并写下
`version.txt`。带上 `-tags bundled` 构建时，这个 payload 会被 `go:embed` 进 exe。

```powershell
# 1. 生成 payload
#    本地优先：assets/ 里匹配 platform/version 的压缩包会被直接使用，不会联网下载
go run ./tools/packmaafw

# 指定其它压缩包（路径或 URL）
go run ./tools/packmaafw -archive D:\downloads\MAA-win-x86_64-v5.13.0.zip

# 换版本/平台（本地没有对应压缩包时才会下载，并缓存到 assets/）
go run ./tools/packmaafw -version v5.14.0 -platform win-x86_64

# 2. 构建两种 exe
go build -tags bundled -o maactl.exe ./cmd/maactl      # 携带 DLL，约 31 MiB
go build -o maactl-lite.exe ./cmd/maactl               # 不携带 DLL，约 5 MiB
```

打包工具的压缩包来源优先级：`-archive` 指定的路径 > `assets/`（可用 `-assets` 换目录）里匹配的
压缩包 > 下载。也就是说，**本地已有压缩包时不会联网**；只有本地没有时，才按 `maafw.version`
（或 `-version`）下载 `MAA-<platform>-<version>.zip` 并缓存到 `assets/`（不入库）。下载与 API 查询
均支持 `GH_TOKEN`/`GITHUB_TOKEN`（提高速率限制），连接卡住超过 60 秒会超时重试，4xx 直接失败。

运行时，内嵌的 payload 会解包到用户缓存目录并复用：

```text
%LOCALAPPDATA%\maactl\maafw\<version>\bin\      # Windows
~/.cache/maactl/maafw/<version>/bin/            # Linux（macOS 为 ~/Library/Caches）
```

`MaaFramework.dll` 最后写入，所以“该文件存在”即代表解包完整，中途中断会在下次运行时重做。
自带版本会显示在版本信息里：

```powershell
./maactl.exe --version       # maactl version 0.1.1 (MaaFramework v5.13.0)
./maactl-lite.exe --version  # maactl version 0.1.1
```

升级 MaaFramework 只需改 `maafw.version`（或加 `-version`）后重新打包与构建；`--lib-dir`（`-l`）
始终可覆盖自带运行库。带 `-tags bundled` 但未先生成 payload 时，构建仍会成功，但运行时会输出
明确告警并回退到 `./maafw/bin`。

## 版本注入与本地验证

版本号由链接期注入，正式发布由 CI 用 tag 填入：

```powershell
go run ./tools/packmaafw
go build -tags bundled -ldflags "-X main.version=1.2.3-beta.1" -o maactl.exe ./cmd/maactl
go build -ldflags "-X main.version=1.2.3-beta.1" -o maactl-lite.exe ./cmd/maactl
./maactl.exe --version             # maactl version 1.2.3-beta.1 (MaaFramework v5.13.0)
./maactl-lite.exe --version        # maactl version 1.2.3-beta.1
```

## 验证 npm 包装器

不需要发布就能验证包装器（细节见 [npm-package.md](npm-package.md)）：

```powershell
cd npm
npm test                                   # 单元测试，离线可跑
npm run vendor:binary -- ..\maactl.exe      # 把本地构建的 exe 放进 vendor/，模拟发布包
npx --yes --package . maactl -v
```

也可以跳过打包，直接让包装器使用本地 exe：

```powershell
$env:MAACTL_BINARY = "D:\01_Projects\github\MaaCtl\maactl.exe"
node npm\bin\maactl.js -v
```
