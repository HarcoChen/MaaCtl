# MaaCtl

`maactl` 是 [MaaFramework](https://github.com/MaaXYZ/MaaFramework) 的命令行客户端。它加载 ProjectInterface v2（PI）项目，查看控制器、资源和任务，并在指定 ADB 设备上运行 PI task 或 Pipeline 节点。

当前版本支持 ADB 任务执行。所有选项都以 `-` 或 `--` 开头；只有命令名和 task/node 名称使用位置参数。

## 项目结构

```text
cmd/maactl/            程序入口（main 包，仅解析参数并调用 cli）
internal/
  cli/                 命令树：root、adb/win32、interface、resource、run、execute
  pi/                  ProjectInterface v2 的数据模型、加载与查找
  event/               sink 事件输出与 focus 文本渲染
  help/                帮助渲染器（分区、继承来源、planned 标记）
  i18n/                帮助语言检测与本地化文本
  maafw/               MaaFramework 运行库定位与初始化
  output/              文本与 JSON 输出辅助
  table/               终端宽度对齐的表格渲染
tests/                 跨包测试（CLI 端到端与 PI 加载）
docs/                  设计文档
maafw/                 本地 MaaFramework 运行库（不入库）
```

`internal/` 内的包只在本模块可用；`tests/` 通过 `internal/cli` 等导出接口运行 CLI，因此不需要把内部实现暴露给外部。

## 构建

```powershell
go build -o maactl.exe ./cmd/maactl

# 运行全部测试（包括 tests/ 下的 CLI 端到端测试）
go test ./...
```

默认从**当前工作目录**读取 PI，而不是从 `maactl.exe` 所在目录读取：

```text
./interface.json    # ProjectInterface v2 文件
```

MaaFramework 运行库优先从当前目录的 `./maafw/bin/` 加载；为兼容已有部署，找不到时也会尝试 `maactl.exe` 周边的 `maafw/bin`。`--interface`（`-f`）可指定 `interface.json` 文件或包含它的项目目录。`--lib-dir`（`-l`）可指定其它 MaaFramework 运行库目录。

```powershell
# 在当前目录的 interface.json 上操作
./maactl.exe interface --show

# 指定 PI 项目目录
./maactl.exe interface --show -f D:\01_Projects\github\MaaMio

# 指定 PI 文件和 MaaFramework 运行库
./maactl.exe interface --show `
  -f D:\projects\demo\interface.json `
  -l D:\tools\maafw\bin
```

## 查看项目

`interface` 使用操作参数，不再使用 `interface tasks` 等子命令。每次选择一个操作：`--show/-s`、`--controllers/-c`、`--resources/-r`、`--tasks/-t` 或 `--validate/-v`。

```powershell
# PI 概览
./maactl.exe interface --show -f D:\01_Projects\github\MaaMio

# 列出控制器、资源与 task
./maactl.exe interface --controllers -f D:\01_Projects\github\MaaMio
./maactl.exe interface --resources -f D:\01_Projects\github\MaaMio
./maactl.exe interface --tasks -f D:\01_Projects\github\MaaMio

# 对应的短参数形式
./maactl.exe interface -s -f D:\01_Projects\github\MaaMio
./maactl.exe interface -c -f D:\01_Projects\github\MaaMio
./maactl.exe interface -r -f D:\01_Projects\github\MaaMio
./maactl.exe interface -t -f D:\01_Projects\github\MaaMio

# 验证 PI 是否可加载；JSON 输出方便脚本处理
./maactl.exe interface --validate -f D:\01_Projects\github\MaaMio
./maactl.exe interface --tasks -f D:\01_Projects\github\MaaMio --json
```

PI 中的 `import` 会随主 `interface.json` 一同加载。资源路径相对于该 PI 文件所在目录解析。

文本列表带有列名，并按终端显示宽度对齐中文和英文。`name` 是命令使用的名称，`label` 是 PI 中的显示名称；任务的 `entry` 是入口节点，控制器的 `type` 是类型，资源的 `path` 是资源路径。缺失值显示为 `-`，空列表显示“无数据”；`--json/-j` 仍输出原有 JSON 结构。

## 查看设备与资源

先用 ADB 设备列表确定要使用的设备地址：

```powershell
./maactl.exe adb devices
./maactl.exe adb devices --json
./maactl.exe win32 devices
```

加载资源并查看其元数据或可运行的 Pipeline 节点：

```powershell
# 默认加载第一个资源
./maactl.exe resource inspect -f D:\01_Projects\github\MaaMio

# 明确选择名为 base 的资源
./maactl.exe resource nodes `
  -f D:\01_Projects\github\MaaMio `
  --resource base

# 等价的快捷形式
./maactl.exe resource -i -f D:\01_Projects\github\MaaMio
./maactl.exe resource -n -r base -f D:\01_Projects\github\MaaMio
```

## 运行 task 与节点

`run task <task-name>` 会先在 PI 中查找 task，再运行 task 的 `entry` 节点。`run node <node-name>` 则直接运行指定 Pipeline 节点。两者都支持 `-t` 和 `-n` 快捷形式。

```powershell
# 运行 PI 中的指定 task。该示例任务持续运行，10 秒后自动发送停止信号。
./maactl.exe run task "自动挂机卖蛋" `
  -f D:\01_Projects\github\MaaMio `
  --stop-after 10s

# task 的快捷形式
./maactl.exe run -t "自动挂机卖蛋" `
  -f D:\01_Projects\github\MaaMio `
  --stop-after 10s

# 直接运行指定 Pipeline 节点
./maactl.exe run node "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  --stop-after 10s

# 节点的快捷形式，并显式选择 ADB 设备
./maactl.exe run -n "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  --adb-address 127.0.0.1:16384 `
  --stop-after 10s

# 显式选择 PI controller、资源和 ADB 设备
./maactl.exe run -n "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  --controller Android `
  --resource base `
  --adb-address 127.0.0.1:16384 `
  --stop-after 10s
```

未指定选择参数时，CLI 使用以下规则：

- controller：PI 中只有一个 controller 时自动选择；多个 controller 时要求 `--controller/-c`。
- resource：选择第一个与 controller 兼容的资源；可通过 `--resource/-r` 指定。
- ADB 设备：检测到一个设备时自动选择；未检测到设备或检测到多个设备时要求 `--adb-address/-a`。

`--stop-after` 适合验证会持续运行的任务；正常的有限 task 不需要该参数。

## 事件与 focus 输出

运行时会注册 MaaFramework 的 tasker 与 context sink。默认 `--events focus`，只输出 PI Pipeline `focus` 配置命中的文本，例如 `Node.Recognition.Starting: "开始签到"` 会输出：

```text
开始签到
```

```powershell
# 默认只输出 focus 文本
./maactl.exe run -n "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  --stop-after 10s

# 输出全部 MaaFramework sink 事件及其详情
./maactl.exe run -n "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  --events all `
  --stop-after 10s

# 不输出 sink 事件
./maactl.exe run -n "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  --events off `
  --stop-after 10s
```

## 覆盖 Pipeline

`--override/-o` 接受最终 Pipeline override 的 JSON；`--override-file/-O` 从文件读取相同内容。两者不能同时使用。

```powershell
# 在命令行提供 JSON。PowerShell 中单引号可保留 JSON 双引号。
./maactl.exe run -n "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  -o '{"签到-开始签到":{"enabled":false}}'

# 从文件读取 override JSON
./maactl.exe run task "自动挂机卖蛋" `
  -f D:\01_Projects\github\MaaMio `
  --override-file D:\projects\overrides\run.json `
  --stop-after 10s
```

## 帮助与 JSON

`-h` 与 `--help` 等价，`maactl help <command>` 输出相同内容。帮助语言跟随系统：Windows 取用户默认 UI 语言，其他系统读取 `LC_ALL`/`LC_MESSAGES`/`LANG`；中文显示中文，其余显示英文。可用环境变量 `MAACTL_LANG=zh_CN` 或 `MAACTL_LANG=en` 强制覆盖。

顶层帮助只列出命令与一句话说明、全局参数和示例，不再展开各命令的参数；带子命令的命令帮助列出自身参数与子命令说明；叶子命令列出全部可用参数，共享的执行参数只定义一次并标注来源（如 `Execution Flags (inherited from "maactl run")`）。`--version` 没有短参数。

```powershell
./maactl.exe -h
./maactl.exe --help
./maactl.exe run -h
./maactl.exe run --help
./maactl.exe run task -h
./maactl.exe run task --help
./maactl.exe resource --help
./maactl.exe help run
./maactl.exe --version
```

`--json/-j` 为项目查询、资源查询和设备查询提供 JSON 输出。运行命令与 `--json` 一同使用时，sink 输出也会变为 JSON。

## 已公布但尚未实现

帮助中以 `(planned)` 标注的命令、以及归入 `Planned Flags` 段的参数，属于已预留的 CLI 契约，当前会明确返回未实现错误：

- `interface --options`、`interface --presets`
- `resource hash`
- `run preset <name>`
- `--option/-p`、`--option-file`、`--overlay`、`--dry-run`、`--explain`

## 发布

推送符合 SemVer 的 tag（`v<major>.<minor>.<patch>[-alpha.N|-beta.N|-rc.N]`，匹配 `v[0-9]*`）会触发 `.github/workflows/release.yml`：

1. `version`：校验 tag 是否符合 SemVer，计算是否预发布、预发布通道和上一个正式版 tag；
2. `build`：在 Windows runner 上运行 `go test ./...`，并用 `-ldflags "-X main.version=<tag>"` 构建 `dist/maactl.exe`；
3. `changelog`：生成当前版本与上一个正式版之间的更新日志，按 feat/fix/perf/refactor/docs 等分组并附 commit 链接；
4. `release`：等以上两个任务完成后统一创建 GitHub Release。正式版发布为 Latest；`-alpha`/`-beta`/`-rc` 等预发布版本标记为 Pre-release（标题带通道名），不会成为 Latest。重复执行会更新已有 Release 并覆盖 exe。

本地验证版本注入：

```powershell
go build -ldflags "-X main.version=1.2.3-beta.1" -o maactl.exe ./cmd/maactl
./maactl.exe --version   # maactl version 1.2.3-beta.1
```
