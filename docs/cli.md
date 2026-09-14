# 命令行参考

本文是 `maactl` 的完整命令说明，对应第二版命令行设计
（[pi-cli-design.md](pi-cli-design.md)）。安装方式见 [README](../README.md)。

## 通用约定

- 只有命令名和必需的名称（task、node、preset）使用位置参数；其余一律是 `-`/`--` 选项。
- PI 默认从**进程启动目录**读取 `./interface.json`。`-f/--interface` 可指定文件，或包含
  `interface.json` 的项目目录。相对路径都以该文件所在目录解析。
- 帮助语言跟随系统（`MAACTL_LANG=zh_CN|en` 可覆盖）；PI 里的 `$label` 由 `--lang` 解析，
  默认与帮助语言一致。
- MaaFramework 运行库查找顺序见 [build.md](build.md#运行库查找顺序)；`-l/--lib-dir` 始终可覆盖。

```powershell
maactl pi info -f D:\01_Projects\github\MaaMio
maactl run task "签到" -f D:\01_Projects\github\MaaMio --stop-after 30s
maactl device adb --json
```

### 全局选项

| 参数 | 作用 |
| --- | --- |
| `-f, --interface <path>` | PI 文件或项目目录（默认：`./interface.json`） |
| `-l, --lib-dir <dir>` | MaaFramework DLL 目录（默认：`./maafw/bin`，bundled 构建用内嵌运行库） |
| `-j, --json` | 输出 JSON；运行时 sink 事件也变为 JSON 行 |
| `--lang <code>` | 解析 PI `$label` 的语言，如 `zh_cn`、`en_us`（默认：跟随系统） |
| `--config <path>` | 客户端配置文件（默认自动发现，见下） |
| `--no-config` | 不读取客户端配置文件 |
| `--log-dir <dir>` | MaaFramework 日志目录 |
| `--verbose` | 输出选择来源与合并细节 |
| `-h, --help` / `-v, --version` | 帮助 / 版本（`-v` 只在顶层是版本） |

短参数没有第二种含义：`-r`/`-c` 始终是 resource/controller，`-t`/`-n` 只属于 `run` 的
快捷形式，`-o` 只属于 `--override`；`--option`、`--option-file`、`--override-file`、
`--preset` 只有长参数。

### 退出码

| 码 | 含义 |
| --- | --- |
| `0` | 成功 |
| `1` | 框架初始化等内部错误 |
| `2` | 参数错误 / PI 校验失败 / 选择歧义 |
| `3` | 资源加载或 hash 校验失败 |
| `4` | 控制器发现或连接失败 |
| `5` | pretask 失败 |
| `6` | 任务执行失败 |
| `7` | 超时（`--timeout`） |
| `8` | 被中断（Ctrl+C） |

### 客户端配置文件

`maactl` 会读取生态通行的 `config/maa_pi_config.json`，用于记住控制器、资源、设备与配置项取值：

1. `--config <path>`；
2. `<PI 目录>/config/maa_pi_config.json`；
3. `<当前目录>/config/maa_pi_config.json`。

识别 `controller`、`resource`、`adb.*`、`win32.*`、`option`、`task[].name/option/enabled`；
未知字段（如 MFAA 的 `__key`）忽略。文件存在但无法解析时会报错，而不是静默丢弃；
用 `--no-config` 完全跳过，用 `config show` 查看生效值。

## 查看项目：`pi`

`pi`（别名 `interface`）只读取 PI，不加载资源、不连接控制器，因此没有设备也能用。

```text
maactl pi info       [--json]
maactl pi validate   [--strict] [--json]
maactl pi controllers [--type <Adb|Win32|MacOS|PlayCover|Gamepad|Linux>]
maactl pi resources   [-c <controller>]
maactl pi tasks       [-c <controller>] [-r <resource>] [--group <name>] [--all]
maactl pi groups
maactl pi options     [-c <controller>] [-r <resource>] [-t <task>] [--all]
maactl pi presets
maactl pi settings
```

```powershell
maactl pi info -f D:\01_Projects\github\MaaMio
maactl pi tasks -f D:\01_Projects\github\MaaMio -c Android --all
maactl pi options -f D:\projects\demo -t 常规作战 -c Windows -r Official
maactl pi validate -f D:\projects\demo --strict
```

- `pi info` 输出名称、版本、协议版本、语言、各类数量，以及是否声明 agent / pretask / telemetry
  （maactl 不实现遥测上报）。
- `pi validate` 检查可解析性、名称唯一性、引用完整性与磁盘文件；`--strict` 把提示性问题
  （当前平台无法创建的控制器、缺失的资源路径或语言文件）也视为失败。`--json` 返回
  `{"issues":[{"level","path","message"}],"strict":false}`。
- `pi tasks`/`pi options` 默认隐藏与所选 controller/resource 不匹配的项，`--all` 会列出并说明原因。
- `pi options` 按 `global_option → resource.option → controller.option → task.option` 顺序展开
  配置项树，并展开被选中 case 激活的子配置项。

## 查看设备：`device`

```powershell
maactl device adb            # 地址、ADB 路径、建议的截图/输入方式
maactl device win32          # 窗口名、类名、句柄
maactl device adb --json
```

这里的地址、名称、类名、句柄可以原样传给 `run` 的 `-a/--name/--win32-*`。
旧命令 `maactl adb devices` 与 `maactl win32 devices` 仍然可用（隐藏命令，会打印迁移提示）。

## 查看资源：`resource`

只加载资源，不创建控制器、不执行 Pipeline，因此也能用来检查打包产物。

```text
maactl resource inspect [-r <PI 资源名> | --path <目录> ...] [--overlay <目录> ...]
maactl resource nodes   （同上）
maactl resource hash    （同上）[--verify]
```

```powershell
maactl resource inspect -f D:\01_Projects\github\MaaMio
maactl resource nodes -r base -f D:\01_Projects\github\MaaMio --json
maactl resource hash --path D:\projects\pkg\resource --verify
```

`--path` 与 `-r/--resource` 二选一：前者直接给资源根目录（相对当前目录），后者用 PI 的
`resource.path`（相对 PI 目录）。`--overlay` 在基础路径之后加载，可重复。
`resource hash --verify` 在 PI 模式下与 `resource.hash` 比较，不一致时以退出码 3 失败。

## 运行：`run`

```text
maactl run task   <task-name>   [flags]
maactl run preset <preset-name> [flags]
maactl run node   <node-name>   [flags]
maactl run -t <task-name>  [flags]     # 等价 run task
maactl run -n <node-name>  [flags]     # 等价 run node
```

### 执行选项

| 参数 | 作用 |
| --- | --- |
| `-r, --resource <name>` | PI 资源名（默认：配置文件 → 第一个兼容资源） |
| `-c, --controller <name>` | PI 控制器名（默认：配置文件 → 唯一的控制器） |
| `-a, --adb-address <serial>` | ADB 设备地址（默认：配置文件 → 唯一检测到的设备） |
| `--name <name>` | ADB 设备名（MaaToolkit 报告的名称） |
| `--adb-path <path>` | 覆盖 adb 可执行文件；地址未被 MaaToolkit 发现时也能直接建控制器 |
| `--win32-handle/--win32-class/--win32-window` | Win32 窗口选择（默认：命令行 → 配置 → PI 正则 → 唯一窗口） |
| `--win32-screencap/--win32-mouse/--win32-keyboard` | 覆盖 Win32 截图/输入方式 |
| `--gamepad-type <Xbox360\|DualShock4>` | 虚拟手柄类型 |
| `--option <name>=<value>` | 配置项取值，可重复，语法见下 |
| `--option-file <path>` | 配置项取值 JSON 文件（结构同 `preset.task[].option`） |
| `--preset <name>` | 把该 preset 里此 task 的取值应用到本次运行（仅 `run task`） |
| `-o, --override <json>` / `--override-file <path>` | 最终 Pipeline override，二者互斥 |
| `--overlay <dir>` | 在所选资源之后追加加载的资源根目录，可重复 |
| `--path <dir>` | `run node` 的资源根目录，替代 PI 资源，可重复 |
| `--events <focus\|all\|off>` | 事件输出，默认 `focus` |
| `--focus-display <list>` | 关注哪些 focus 渠道，默认 `log`；可写 `log,toast,notification,dialog,modal` 或 `all` |
| `--dry-run` | 只做选择、资源加载与覆盖计算，不连接控制器、不跑 pretask、不执行节点 |
| `--explain` | 打印选择结果与每一层 override（可配合 `--dry-run`） |
| `--timeout <duration>` | 超过时长后 `PostStop` 并以退出码 7 结束 |
| `--stop-after <duration>` | 运行指定时长后停止并视为成功（调试用） |
| `--require-resource-hash` | `resource.hash` 不匹配时以退出码 3 失败（默认仅告警） |
| `--no-agent` / `--agent-log <term\|off\|dir>` | 是否启动 agent、agent 输出去向 |
| `--continue-on-error` | `run preset` 中某个 task 失败后继续 |

### `run task`

```powershell
# 用配置文件里的控制器/资源/设备直接跑（MaaMio 示例）
maactl run task "签到" -f D:\01_Projects\github\MaaMio --stop-after 30s

# 只看最终 override，不执行
maactl run task "签到" -f D:\01_Projects\github\MaaMio --dry-run --explain

# 显式选择，并覆盖配置项
maactl run task "常规作战" -f D:\projects\demo `
  -c Android -r Official -a 127.0.0.1:16384 `
  --option 作战关卡="3-9 厄险（百灵百验鸟）" `
  --option 复现次数=x3 `
  --option 战斗划火柴=普通划火柴,蓄力划火柴 `
  --events all --timeout 30m
```

任务名按 `name` 精确匹配 → 当前语言下的 `label` → 大小写不敏感的 `name` 顺序解析。

### `run preset`

按 preset 的 `task` 数组顺序运行 `enabled != false` 的 task；前一个失败即停止（除非
`--continue-on-error`）。命令行 `--option` 只覆盖该 task 实际引用的同名配置项。

```powershell
maactl run preset 刷日常 -f D:\projects\demo --events off
```

### `run node`

直接以节点名作为任务入口。资源可以来自 PI（`-r`）或 `--path`：

```powershell
maactl run node "签到-开始签到" -f D:\01_Projects\github\MaaMio --events all
maactl run node Login --path D:\pkg\resource --events all
```

### 配置项取值语法

```text
--option 复现次数=x3                          # select / switch：单个 case 名
--option 刷完全部体力=No                       # switch 也接受 y/n、yes/no
--option 战斗划火柴=普通划火柴,蓄力划火柴        # checkbox：逗号分隔
--option '战斗划火柴=["普通划火柴","连续划火柴"]' # checkbox：JSON 数组
--option 自定义关卡.章节号=4                    # input / hotkey：name.field=value
```

`checkbox` 的多个 case 按 `cases` 定义顺序合并，与输入顺序无关，并校验
`min_count`/`max_count`。密码字段（`inputs[].password = true`）不能用 `--option` 传明文，
只能用 `--option-file` / 配置文件 / `{"env":"NAME"}` 引用，输出中一律掩码为 `******`。

### 选择与覆盖顺序

```text
资源：resource.path[0..n] → controller.attach_resource_path[*] → --overlay[*]
覆盖：global_option → resource.option → controller.option → task.option
      → task.pipeline_override → --override / --override-file
```

同一节点内的同名顶层字段由后者整体替换（数组整体替换）。`resource.hash` 在加载完
`resource.path` 后、加载 `attach_resource_path` 之前校验。

### pretask 与 agent

- PI 声明 `pretask` 时，会在创建控制器**之前**按顺序执行；CWD 为 `interface.json` 所在目录；
  `pretask.option` 引用的配置项取值会序列化成单行 JSON 追加为最后一个参数；非零退出码以
  退出码 5 结束。
- 声明 `agent` 时，会在资源加载完成后启动并连接，注入协议 v2.5.0 约定的
  `PI_INTERFACE_VERSION`、`PI_CLIENT_NAME`、`PI_CLIENT_VERSION`、`PI_CLIENT_LANGUAGE`、
  `PI_CLIENT_MAAFW_VERSION`、`PI_VERSION`、`PI_CONTROLLER`、`PI_RESOURCE`
  （后两者为已解析 i18n 的单行 JSON）。输出默认转发到终端并加 `[agent]` 前缀。

### 事件与 focus

默认 `--events focus`：只输出 PI Pipeline `focus` 命中的文本，例如 `签到-开始签到` 的
`Node.Recognition.Starting: "开始签到"` 会输出：

```text
开始签到
```

`--focus-display` 决定哪些 `display` 渠道会输出（默认只有 `log`）；CLI 是日志终端，
`modal` 只会打印内容，不会阻塞等待。`--events all` 输出全部 sink 事件及其详情，
`--events off` 不输出。加 `--json` 时 sink 事件变为 JSON 行。

## 查看客户端配置：`config`

```powershell
maactl config path -f D:\01_Projects\github\MaaMio   # 配置文件路径
maactl config show -f D:\01_Projects\github\MaaMio   # 生效的控制器/资源/设备/配置项取值
```

服务端式的密码取值只显示 `{"env":"NAME"}`，不回显明文。

## 帮助与 JSON

`-h` 与 `--help` 等价，`maactl help <command>` 输出相同内容；`maactl version` 等价 `maactl -v`。
帮助按「选项 / 执行选项（与子命令共用）/ 继承选项」分段，共享的执行选项只打印一次。

```powershell
maactl -h
maactl run -h
maactl run task -h
maactl help pi tasks
maactl -v
```

`--json/-j` 对查询命令输出结构化结果；对 `run` 则 sink 事件变成 JSON 行，并在结束时输出摘要：

```json
{
  "interface": "D:\\01_Projects\\github\\MaaMio\\interface.json",
  "resource": "base",
  "resource_paths": ["D:\\01_Projects\\github\\MaaMio\\resource\\base"],
  "controller": "Android",
  "task": "签到",
  "entry": "签到-开始签到",
  "status": "success",
  "elapsed_ms": 12034,
  "selections": [
    { "name": "复现次数", "layer": "task.option", "source": "cli", "value": "x3" }
  ],
  "effective_override": { "SetReplaysTimes": { "expected": "3" } }
}
```

运行期输出分流：进度信息与错误走 stderr，focus 文本与最终摘要走 stdout，方便
`maactl run ... > focus.log` 只收集任务输出。

## 迁移对照（第一版 → 第二版）

| 旧写法 | 新写法 |
| --- | --- |
| `maactl interface --show` | `maactl pi info` |
| `maactl interface --controllers/--resources/--tasks/--validate` | `maactl pi controllers/resources/tasks/validate` |
| `maactl interface --options/--presets` | `maactl pi options/presets`（已实现） |
| `maactl adb devices` | `maactl device adb`（旧命令保留迁移提示） |
| `maactl win32 devices` | `maactl device win32` |
| `maactl resource -i` / `maactl resource -n` | `maactl resource inspect` / `maactl resource nodes` |
| `maactl run task <name>` / `run -t` | 不变 |
| `maactl run node <name>` / `run -n` | 不变 |
| `--option/-p`（旧版计划） | `--option`（无短参数）；`-p` 不再使用 |
| `--override/-o` | 不变；`--override-file` 不再有 `-O` |

> 注意：第一版里 `-v` 在 `interface` 下表示 `--validate`，`-c`/`-r` 在 `interface` 下表示
> controllers/resources。第二版取消了这些重载：`--validate` 变成 `pi validate`，
> `--controllers/--resources` 变成 `pi controllers/resources`。
