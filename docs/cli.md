# 命令行参考

本文是 `maactl` 的完整命令说明。安装方式见 [README](../README.md)。

## 通用约定

- 所有选项都以 `-` 或 `--` 开头；只有命令名和 task/node 名称使用位置参数。
- PI 默认从**当前工作目录**读取 `./interface.json`（不是从 exe 所在目录）。`-f/--interface`
  可指定 `interface.json` 文件，或包含它的项目目录。
- MaaFramework 运行库的查找顺序见 [build.md](build.md#运行库查找顺序)；`-l/--lib-dir` 始终可覆盖。
- 帮助语言跟随系统（Windows 取用户默认 UI 语言，其他系统读取 `LC_ALL`/`LC_MESSAGES`/`LANG`），
  中文环境显示中文，其余显示英文；可用 `MAACTL_LANG=zh_CN` 或 `MAACTL_LANG=en` 强制覆盖。

```powershell
# 在当前目录的 interface.json 上操作
maactl interface --show

# 指定 PI 项目目录
maactl interface --show -f D:\01_Projects\github\MaaMio

# 指定 PI 文件和 MaaFramework 运行库
maactl interface --show `
  -f D:\projects\demo\interface.json `
  -l D:\tools\maafw\bin
```

## 参数速查

全局选项（所有命令可用）：

| 参数 | 作用 |
| --- | --- |
| `-h, --help` | 显示帮助 |
| `-f, --interface` | ProjectInterface 文件或项目目录（默认 `./interface.json`） |
| `-j, --json` | 输出 JSON；运行时 sink 事件也输出 JSON |
| `-l, --lib-dir` | MaaFramework DLL 目录（默认 `./maafw/bin`） |
| `-v, --version` | 显示版本信息（仅顶层可用） |

`interface` 的操作参数：`--show/-s`、`--controllers/-c`、`--resources/-r`、`--tasks/-t`、
`--validate/-v`。

`run` 的执行选项（`run task` 与 `run node` 共用）：

| 参数 | 作用 |
| --- | --- |
| `-a, --adb-address` | ADB 设备序列号/地址（默认：唯一检测到的设备） |
| `-c, --controller` | PI 控制器名称（默认：唯一的控制器） |
| `-r, --resource` | PI 资源名称（默认：第一个兼容资源） |
| `--events` | 事件输出：`focus`（默认）/ `all` / `off` |
| `--no-agent` | 不启动 ProjectInterface agent |
| `-o, --override` / `-O, --override-file` | 最终 Pipeline override 的 JSON / 文件 |
| `--stop-after` | 运行指定时长后停止（duration，如 `10s`） |

同一个短参数在不同位置含义不同，写脚本时注意：`-v` 顶层是 `--version`、`interface` 下是
`--validate`；`-c`/`-r` 在 `run` 下是 controller/resource，在 `interface` 下是 controllers/resources。

## 查看项目

`interface` 使用操作参数，不再使用 `interface tasks` 等子命令。每次选择一个操作：`--show/-s`、
`--controllers/-c`、`--resources/-r`、`--tasks/-t` 或 `--validate/-v`。

```powershell
# PI 概览
maactl interface --show -f D:\01_Projects\github\MaaMio

# 列出控制器、资源与 task
maactl interface --controllers -f D:\01_Projects\github\MaaMio
maactl interface --resources -f D:\01_Projects\github\MaaMio
maactl interface --tasks -f D:\01_Projects\github\MaaMio

# 对应的短参数形式
maactl interface -s -f D:\01_Projects\github\MaaMio
maactl interface -c -f D:\01_Projects\github\MaaMio
maactl interface -r -f D:\01_Projects\github\MaaMio
maactl interface -t -f D:\01_Projects\github\MaaMio

# 验证 PI 是否可加载；JSON 输出方便脚本处理
maactl interface --validate -f D:\01_Projects\github\MaaMio
maactl interface --tasks -f D:\01_Projects\github\MaaMio --json
```

PI 中的 `import` 会随主 `interface.json` 一同加载。资源路径相对于该 PI 文件所在目录解析。

文本列表带有列名，并按终端显示宽度对齐中文和英文。`name` 是命令使用的名称，`label` 是 PI 中的
显示名称；任务的 `entry` 是入口节点，控制器的 `type` 是类型，资源的 `path` 是资源路径。
缺失值显示为 `-`，空列表显示“无数据”；`--json/-j` 仍输出原有 JSON 结构。

## 查看设备与资源

先用 ADB 设备列表确定要使用的设备地址：

```powershell
maactl adb devices
maactl adb devices --json
maactl win32 devices
```

加载资源并查看其元数据或可运行的 Pipeline 节点：

```powershell
# 默认加载第一个资源
maactl resource inspect -f D:\01_Projects\github\MaaMio

# 明确选择名为 base 的资源
maactl resource nodes `
  -f D:\01_Projects\github\MaaMio `
  --resource base

# 等价的快捷形式
maactl resource -i -f D:\01_Projects\github\MaaMio
maactl resource -n -r base -f D:\01_Projects\github\MaaMio
```

## 运行 task 与节点

`run task <task-name>` 会先在 PI 中查找 task，再运行 task 的 `entry` 节点；`run node <node-name>`
则直接运行指定 Pipeline 节点。两者都支持 `-t` 和 `-n` 快捷形式。

```powershell
# 运行 PI 中的指定 task。该示例任务持续运行，10 秒后自动发送停止信号。
maactl run task "自动挂机卖蛋" `
  -f D:\01_Projects\github\MaaMio `
  --stop-after 10s

# task 的快捷形式
maactl run -t "自动挂机卖蛋" `
  -f D:\01_Projects\github\MaaMio `
  --stop-after 10s

# 直接运行指定 Pipeline 节点
maactl run node "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  --stop-after 10s

# 节点的快捷形式，并显式选择 ADB 设备
maactl run -n "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  --adb-address 127.0.0.1:16384 `
  --stop-after 10s

# 显式选择 PI controller、资源和 ADB 设备
maactl run -n "签到-开始签到" `
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

运行时会注册 MaaFramework 的 tasker 与 context sink。默认 `--events focus`，只输出 PI Pipeline
`focus` 配置命中的文本，例如 `Node.Recognition.Starting: "开始签到"` 会输出：

```text
开始签到
```

```powershell
# 默认只输出 focus 文本
maactl run -n "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  --stop-after 10s

# 输出全部 MaaFramework sink 事件及其详情
maactl run -n "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  --events all `
  --stop-after 10s

# 不输出 sink 事件
maactl run -n "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  --events off `
  --stop-after 10s
```

## 覆盖 Pipeline

`--override/-o` 接受最终 Pipeline override 的 JSON；`--override-file/-O` 从文件读取相同内容。
两者不能同时使用。

```powershell
# 在命令行提供 JSON。PowerShell 中单引号可保留 JSON 双引号。
maactl run -n "签到-开始签到" `
  -f D:\01_Projects\github\MaaMio `
  -o '{"签到-开始签到":{"enabled":false}}'

# 从文件读取 override JSON
maactl run task "自动挂机卖蛋" `
  -f D:\01_Projects\github\MaaMio `
  --override-file D:\projects\overrides\run.json `
  --stop-after 10s
```

## 帮助与 JSON

`-h` 与 `--help` 等价，`maactl help <command>` 输出相同内容。

顶层帮助只列出命令与一句话说明、全局参数和示例，不展开各命令的参数；带子命令的命令帮助列出自身
参数与子命令说明；叶子命令列出全部可用参数，共享的执行参数只定义一次并标注来源（如
`Execution Flags (inherited from "maactl run")`）。`--version` 的短参数是 `-v`，与
`interface --validate` 的 `-v` 不冲突：前者只在顶层可用，后者只在 `interface` 下。

```powershell
maactl -h
maactl --help
maactl run -h
maactl run --help
maactl run task -h
maactl run task --help
maactl resource --help
maactl help run
maactl -v
maactl --version
```

`--json/-j` 为项目查询、资源查询和设备查询提供 JSON 输出。运行命令与 `--json` 一同使用时，
sink 输出也会变为 JSON。

## 已公布但尚未实现

帮助中以 `(planned)` 标注的命令、以及归入 `Planned Flags` 段的参数，属于已预留的 CLI 契约，
当前会明确返回未实现错误：

- `interface --options`、`interface --presets`
- `resource hash`
- `run preset <name>`
- `--option/-p`、`--option-file`、`--overlay`、`--dry-run`、`--explain`
