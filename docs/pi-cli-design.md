# maactl ProjectInterface CLI 设计（第二版）

本文是 `maactl` 的**现行** CLI 设计，取代早期的
[maafw-cli-design.md](maafw-cli-design.md)。目标是完整落实 ProjectInterface v2
协议（对齐到 **v2.10.1**），并把命令面收敛成一致、可脚本化、可测试的形态。

- 协议来源：[MaaFramework docs/zh_cn/3.3-ProjectInterfaceV2协议.md](../maafw/docs/zh_cn/3.3-ProjectInterfaceV2协议.md)、
  [3.1-任务流水线协议.md](../maafw/docs/zh_cn/3.1-任务流水线协议.md)、
  [interface.schema.json](../maafw/tools/interface.schema.json)。
- 协议版本：`interface_version` 固定为 `2`；Client 侧扩展能力版本记为 `PI_INTERFACE_VERSION = v2.10.1`。

## 1. 为什么要重新设计

第一版 CLI 能跑通「选控制器 → 加载资源 → 跑 task」，但设计已经落后于协议和实际使用：

| 问题 | 具体表现 |
| --- | --- |
| 短参数重载 | `-v` 顶层是 `--version`、`interface` 下是 `--validate`；`-c`/`-r` 在 `run` 下是 controller/resource，在 `interface` 下是 controllers/resources；`-i` 在 `resource` 下是 inspect |
| 用布尔选项当子命令 | `interface --show/--controllers/...` 一次只能选一个，帮助里要手写「只能选一个」；`--options`/`--presets` 长期挂着「未实现」 |
| 协议覆盖不全 | 完全没有 `option`、`preset`、`group`、`global_option`、`setting`、`languages`（i18n）、`resource.hash`、`pretask`、`telemetry`、`hotkey`、`checkbox` 的 `min_count`/`max_count` |
| `import` 合并规则错误 | 只追加了 `task`/`controller`/`resource`，且顺序不符合协议；协议要求合并 `task`/`option`/`preset`/`group`/`pretask`/`global_option`/`setting` |
| 无覆盖计算 | 没有 `global_option → resource.option → controller.option → task.option` 的分层合并，`task.pipeline_override` 与 CLI 覆盖也没有统一出口 |
| 无用户配置 | 不读取生态里通行的 `config/maa_pi_config.json`，每次都要手填 `-r`/`-c`/`--adb-address` |
| 无预演能力 | 没有 `--dry-run`/`--explain`，无法只看「最终会下发什么」 |
| 退出码不稳定 | 所有失败都是 `1`，脚本无法区分参数错误、资源错误、控制器错误、任务失败 |
| 运行期缺口 | `pretask` 未实现；启动 agent 子进程时不注入 `PI_*` 环境变量 |

## 2. 设计原则

1. **子命令而不是动作开关。** 每一个「动作」是一个子命令，天然互斥，帮助页可以逐层展开。
2. **短参数只表达一个概念。** 全局只有 `-f/-l/-j/-h/-v`；`-r/-c/-t/-n/-p` 只在 `run`（及对应查询）内出现，含义固定。
3. **协议优先。** PI 里的每个能力要么实现，要么在 `pi validate` 里明确报「已知未支持」，不允许静默忽略。
4. **选择有来源、可解释。** 所有默认值（协议默认、配置文件、命令行）都记录来源，`--explain` 能打印出来。
5. **机器可读优先。** 所有查询命令支持 `--json`；运行结果给出结构化摘要；退出码稳定。
6. **预演与执行分离。** 覆盖计算、资源加载、控制器连接、任务执行是四件事，`--dry-run` 只做前两件。

## 3. 命令总览

```text
maactl [global flags] <group> [subcommand] [arguments] [flags]

  pi, if, interface              ProjectInterface 声明（只读，不需要设备）
    info, i                      PI 概览（名称、版本、数量、语言、能力开关）
    validate, v                 校验 PI（可加载 / 引用完整 / 约束满足）
    controllers, c              列出控制器
    tasks, t                    列出任务（分组、可用性、启用状态）
    groups, g                   列出任务分组
    options, o                  列出（可实例化的）配置项树
    presets, p                  列出预设
    settings, s                 列出设置页分区

  resource, res                  资源（声明的与已加载的，全在一处）
    list, l                     列出 PI 声明的资源（不加载）
    inspect, i                  加载资源并显示路径、hash、节点数
    nodes, n                    列出资源中的 Pipeline 节点
    hash, h                     打印资源 hash，可校验 resource.hash

  device, dev                    设备与窗口（MaaToolkit 发现）
    adb, a                      列出 ADB 设备
    window, w, win32            列出桌面窗口

  run, r                         执行
    task, t <task-name>         运行 PI task（可用 -p/--preset 套用预设取值）
    preset, p <preset-name>     按 preset 顺序运行启用的 task
    node, n <node-name>         直接运行 Pipeline 节点
    -t/-n                       快捷形式（等价 run task / run node）

  config, cfg                    客户端配置（只读）
    show, s                     显示将生效的配置及来源
    path, p                     显示配置文件的解析路径

  version, ver                   显示版本（等价 maactl -v）
```

命名说明：

- **顶层分组按「只读查询 → 执行 → 配置」排列**：`pi`、`resource`、`device` 都是查询，相邻放置；
  `run` 是唯一的执行命令；`config` 是客户端状态。
- **同一对象的所有查询都在同一个分组里**：资源声明的 `list` 与已加载资源的 `inspect/nodes/hash`
  都在 `resource` 下（第一版把 `pi resources` 放在 pi、把 `resource inspect` 放在最后，已修正）。
- `pi` 保留别名 `interface`；它不再接受 `--show` 之类的动作开关。
- `device adb` / `device window` 取代旧的 `adb devices` / `win32 devices`；旧命令隐藏保留一版，
  调用时打印迁移提示。
- `run -t/-n` 是纯便捷形式，与子命令语义完全一致；`run preset` 没有短参数，因为 `-p`/`--preset`
  已经是「把预设取值应用到单个 task」的执行选项。

### 3.1 短别名规则

每个命令、每个选项都有短形式（cobra 生成的 `completion` 子树及其 `--no-descriptions` 除外），
短形式可以是 1–3 个字母：

- **命令别名**用 cobra 原生别名实现，形如 `maactl pi t`、`maactl resource l`、`maactl r -t 签到`。
  同一父命令下别名不重复，帮助的 `Commands:` 段直接列出 `名字, 别名`。
- **选项短形式**有两种：
  - 单字母用 pflag 的 shorthand：`-j`、`-r`、`-c`、`-a`、`-t`、`-n`、`-p`、`-o`、`-e`、`-x`、`-k`；
  - 多字母（pflag 不支持）用 `-if`、`-lib`、`-opt`、`-pa`、`-wh` 这类助记别名，由 `maactl`
    在解析前统一归一化为长参数，全部在帮助中列出。
- 多字母别名在全命令树内唯一，并且只在 `--` 之前生效；`--` 之后的参数原样传递。
- **不重复占用**：同一字母不会在两条命令里表示两件事（第一版的 `-v` = version/validate、
  `-i` = interface/inspect 已取消）。

## 4. 全局选项

| 短形式 | 长形式 | 作用 |
| --- | --- | --- |
| `-f` / `-if` | `--interface <path>` | PI 文件或包含 `interface.json` 的目录；默认**进程启动目录**的 `./interface.json` |
| `-l` / `-lib` | `--lib-dir <dir>` | MaaFramework 运行库目录；默认 `./maafw/bin`，其次 exe 相邻目录 |
| `-j` | `--json` | 结构化 JSON 输出；运行中 sink 事件也变为 JSON 行 |
| `-lg` | `--lang <code>` | 解析 PI `$label` 的语言，如 `zh_cn`/`en_us`；默认跟随系统（`MAACTL_LANG` 可覆盖） |
| `-cfg` | `--config <path>` | 客户端配置文件；默认自动发现 `<PI 目录>/config/maa_pi_config.json` |
| `-nocfg` | `--no-config` | 不读取客户端配置文件 |
| `-log` | `--log-dir <dir>` | MaaFramework 日志目录 |
| `-vb` | `--verbose` | 输出选择来源、加载路径与合并层级 |
| `-h` | `--help` | 帮助 |
| `-v` | `--version` | 版本（只在顶层是版本） |

## 5. `pi` 子命令

所有 `pi` 子命令只读取 PI（加上可选的配置文件与 `--lang`），**不加载资源、不连接控制器**，
因此可以在没有设备时使用。

| 命令 | 关键选项 | 说明 |
| --- | --- | --- |
| `pi info` / `i` | | 名称/显示名/版本/协议版本/控制器数/资源数/任务数/分组数/预设数/语言列表/是否声明 agent、pretask、telemetry |
| `pi validate` / `v` | `-st/--strict` | 默认只报错误；`-st` 把「资源路径不存在」「当前平台不支持的控制器」也视为错误（hash 校验不在此处，由运行期 `-rh/--require-resource-hash` 控制） |
| `pi controllers` / `c` | `-ty/--type <Adb\|Win32\|...>` | 显示 `name/label/type/是否可运行/引用它的资源数` |
| `pi tasks` / `t` | `-c` `-r` `-gr/--group` `-all/--all` | 显示 `name/label/entry/group/option 数`；默认隐藏与当前 controller/resource 不兼容的 task，`-all` 显示并标注 `reason` |
| `pi groups` / `g` | | 分组 `name/label/default_expand/任务数` |
| `pi options` / `o` | `-t/--task` `-r` `-c` `-all` | 按 `global_option → resource → controller → task` 顺序列出会被激活的 option 树（含类型、默认值、层级），`-all` 额外列出不适用与未被引用的定义 |
| `pi presets` / `p` | | 预设 `name/label/任务数`；`--json` 返回完整快照 |
| `pi settings` / `s` | | 分区 `name/label/option 列表` |

资源查询全部在 `resource` 组：`list`（声明，不加载）与 `inspect`/`nodes`/`hash`（加载后），
共享 `-r/--resource`（PI 资源名）与 `-pa/--path`（资源根目录）二选一。

`pi tasks`/`pi options` 的可用性判断遵循协议：

- `task.controller`/`task.resource` 为空表示不限制；
- `option.controller`/`option.resource` 为空表示不限制；
- 被过滤掉的项在文本里不会静默消失，`--all` 会带 `unavailable: <原因>` 列出。

## 6. `run` 子命令

`run task|preset|node` 共享同一套执行选项，帮助里按用途分组打印
（快捷方式 / 目标选择 / 配置项与覆盖 / 资源 / 输出 / 运行控制）：

| 分组 | 短形式 | 长形式 | 作用 |
| --- | --- | --- | --- |
| 快捷 | `-t` / `-n` | `--task` / `--node` | 等价于 `run task` / `run node` |
| 目标 | `-r` | `--resource <name>` | PI 资源名（默认：配置文件 → 第一个与控制器兼容的资源） |
| 目标 | `-c` | `--controller <name>` | PI 控制器名（默认：配置文件 → 唯一的控制器） |
| 目标 | `-a` | `--adb-address <serial>` | ADB 设备地址（默认：配置文件 → 唯一检测到的设备） |
| 目标 | `-nm` | `--name <name>` | ADB 设备名（MaaToolkit 报告的名称） |
| 目标 | `-ap` | `--adb-path <path>` | 覆盖 ADB 可执行文件路径 |
| 目标 | `-wh` `-wc` `-ww` | `--win32-handle/class/window` | Win32 窗口选择（默认：配置文件 → PI `win32` 正则 → 唯一窗口） |
| 目标 | `-ws` `-wm` `-wk` | `--win32-screencap/mouse/keyboard` | 覆盖 Win32 截图/输入方式 |
| 目标 | `-mw` `-mid` `-ms` `-mi` | `--macos-window/window-id/screencap/input` | macOS 窗口选择与方式 |
| 目标 | `-pca` `-pcu` | `--playcover-address/uuid` | PlayCover（macOS）服务地址与应用标识 |
| 目标 | `-ls` `-lv` | `--linux-socket/vk` | Linux（wlroots）Wayland socket 与按键码类型 |
| 目标 | `-gt` | `--gamepad-type <Xbox360\|DualShock4>` | 虚拟手柄类型 |
| 配置项 | `-opt` | `--option <name>=<value>` | 设置配置项取值；可重复（语法见 §7） |
| 配置项 | `-of` | `--option-file <path>` | 配置项取值 JSON 文件（结构同 `preset.task[].option`） |
| 配置项 | `-p` | `--preset <name>` | 载入该 preset 在此 task 上的取值（仅 `run task`） |
| 配置项 | `-o` / `-ovf` | `--override <json>` / `--override-file <path>` | 最终 Pipeline override，优先级最高，二者互斥 |
| 资源 | `-pa` | `--path <dir>` | `run node` 的资源根目录，替代 PI 资源，可重复 |
| 资源 | `-ol` | `--overlay <dir>` | 在所选资源之后追加加载的资源根目录，可重复 |
| 资源 | `-rh` | `--require-resource-hash` | `resource.hash` 不匹配时直接失败（默认仅告警） |
| 输出 | `-e` | `--events <focus\|all\|off>` | 事件输出，默认 `focus` |
| 输出 | `-fd` | `--focus-display <list>` | 关注哪些 focus 渠道，默认 `log`；可写 `log,toast,notification,dialog,modal` 或 `all`（CLI 中 `modal` 不阻塞） |
| 控制 | `-dr` | `--dry-run` | 只做选择、资源加载与覆盖计算，不连接控制器、不跑 pretask、不执行节点 |
| 控制 | `-x` | `--explain` | 打印选择、各层 option 与最终 override（配合 `-dr` 只算不跑） |
| 控制 | `-to` | `--timeout <duration>` | 总时限；超时 `PostStop` 并以退出码 7 结束 |
| 控制 | `-sa` | `--stop-after <duration>` | 运行指定时长后停止并视为成功（调试用） |
| 控制 | `-na` / `-al` | `--no-agent` / `--agent-log <term\|off\|dir>` | agent 启动与输出去向 |
| 控制 | `-k` | `--continue-on-error` | `run preset` 中某个 task 失败后继续 |

### 6.1 `run task <task-name>`

1. 载入 PI、配置文件；
2. 解析 controller（命令行 → 配置 → 唯一项）；
3. 解析 resource（命令行 → 配置 → 第一个与 controller 兼容项）；
4. 检查 `task.controller` / `task.resource` 限制，不满足即失败（没有 `--force`，避免用错资源跑专服任务）；
5. 计算 option 覆盖（§7），合并 `task.pipeline_override` 与 CLI override；
6. 非 `--dry-run`：执行 pretask → 加载资源（校验 hash）→ 启动并连接 agent（注入 `PI_*`）→ 连接控制器 → `PostTask(entry, override)`。

任务名解析顺序：`name` 精确匹配 → `label`（当前语言，解析后）精确匹配 → 大小写不敏感`name`匹配。

### 6.2 `run preset <preset-name>`

按 `preset.task` 数组顺序运行其中 `enabled != false` 的 task：

- 每个 task 单独计算 option（preset 值 > 默认值；命令行 `--option` 只覆盖该 task 实际引用的同名 option）；
- 前一个 task 失败即停止，`--continue-on-error` 改为继续并最后汇总非零退出；
- `--dry-run` 只打印将要按顺序执行的 task 与各自 override。

### 6.3 `run node <node-name>`

直接把节点名作为 `PostTask` 的 entry。资源来源可以是 PI 资源（`-r`）或 `--path` 指定的资源根目录
（可重复，可叠加 `--overlay`）。加载后通过 `GetNodeList` 校验节点存在，不存在即失败。

## 7. Option 语义

### 7.1 激活与过滤

按协议，任何不满足当前 `controller` / `resource` 限制的 option（含嵌套 `option.option`）
都不参与合并；它本身及其子配置项产生的 `pipeline_override` 全部丢弃。

### 7.2 取值来源（优先级从低到高）

```text
option.default_case / inputs[].default / hotkeys[].default
  → 配置文件全局 option → 配置文件 task[].option
  → preset.task[].option
  → --option-file
  → --option
```

**同一个 option 名只有一个取值**：若 `global_option` 与 `task.option` 都引用了同一个 option，用户在
命令行/预设中选的值对两处都生效，层级只决定「在哪里、以什么优先级合并 override」，不会各取一套值。

- `select` / `switch`：单个 `case.name`。`switch` 只接受协议承认的 Yes/No 名称集合
  （`Yes|yes|Y|y` 与 `No|no|N|n`），大小写归一。
- `checkbox`：`case.name` 数组。选中的 case 按 **`cases` 定义顺序**（不是用户输入顺序）合并，
  并校验 `min_count`/`max_count`。
- `input`：字段名 → 字符串值。缺省用 `default`；无 `default` 且未提供即失败；
  按 `verify` 正则校验，失败时用 `pattern_msg`（若有）报错。
- `hotkey`：字段名 → 快捷键字符串（如 `Ctrl+Shift+A`，末段为主键）。合并时按所选控制器的
  `type` 转成虚拟按键码整数。

### 7.3 嵌套选项

选中 case 后，其 `case.option` 列表中的 option 被激活，递归求值；顺序是「父选项先合并、子选项后合并」。
子选项可以继续嵌套，深度不限。

### 7.4 override 合并

```text
global_option（按声明顺序）
→ resource.option
→ controller.option
→ task.option
→ task.pipeline_override
→ --override / --override-file
```

合并规则沿用 MaaFramework 资源覆盖语义：**按 Pipeline 节点名合并节点对象，节点内的同名顶层字段
由后者整体替换（数组整体替换，不做元素级合并）**。

`input`/`hotkey` 的 `pipeline_override` 是模板：字符串里的 `{字段名}` 按 `pipeline_type`
转成 `string`/`int`/`bool`；hotkey 支持 `{名}`、`{名}.primary`、`{名}.modifier1`、`{名}.modifier2`，
替换结果是整数按键码。

### 7.5 命令行写法

```text
# select / switch：单个 case 名
--option 复现次数=x3
--option 刷完全部体力=No

# checkbox：逗号分隔，或 JSON 数组
--option 战斗划火柴=普通划火柴,蓄力划火柴
--option '战斗划火柴=["普通划火柴","连续划火柴"]'

# input / hotkey：name.field=value
--option 自定义关卡.章节号=4
--option 自定义关卡.超时时间=30000
--option 战斗按键.FightCombo=E
```

`--option-file` 使用与 preset 相同的严格 JSON：

```json
{
  "作战关卡": "3-9 厄险（百灵百验鸟）",
  "复现次数": "x3",
  "刷完全部体力": "No",
  "战斗划火柴": ["普通划火柴", "蓄力划火柴"],
  "自定义关卡": { "章节号": "4", "超时时间": "30000" }
}
```

### 7.6 密码字段

`inputs[].password == true` 的字段：

- 禁止 `default`（协议要求）；
- 只能通过 `--option-file`、配置文件或环境变量引用 `{"password": {"env": "NAME"}}` 提供；
- 出现在 `--option`、日志、`--explain`、JSON 输出中一律掩码为 `******`；
- 写入配置文件时必须加密（见 §9）。

## 8. 帮助、输出与退出码

### 8.1 帮助布局

帮助只保留一个标题行、一段说明、一段用法、命令表、按用途分组的选项、一个示例：

```text
run (r): 运行 task、preset 或节点

<一两句说明>

用法：
  maactl run [flags]
  maactl run <command> [flags]

命令：
  task, t <task-name>      运行声明的 task
  ...

快捷方式：
  -t, --task <string>  等价于 "run task <name>"

目标选择：
  -r, --resource <string>   ...
...
全局选项：
  -f, -if, --interface <string>  ...

示例：
  maactl run -t 签到 -if D:\MaaMio -sa 30s
```

规则：

- 一行一个选项，左侧列是 `-短, -别名, --长 <类型>`，右侧是说明与（有意义的）默认值；
- 相同用途的选项在一个分组里，**保持声明顺序**而不是字母序；
- 命令列表直接写明 `名字, 别名`，不需要另查别名表；
- 全局选项只在末尾打印一次；子命令帮助与父命令帮助的选项分组完全一致，
  不再区分「shared」「inherited」，也不再重复打印多个副本。

### 8.2 输出与事件

- 查询类命令：文本为对齐表格（`--json` 为结构化 JSON）。
- 运行类命令：进度与错误走 stderr；focus 文本与最终摘要走 stdout（方便
  `maactl run ... > focus.log`）；加 `--json` 时 sink 事件为 JSON 行，最终摘要也是 JSON。
- `run` 的摘要（`--json`）：

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
  "effective_override": { "…": {} },
  "selections": [
    { "name": "复现次数", "layer": "task.option", "source": "cli", "value": "x3" }
  ]
}
```

退出码固定为：

| 码 | 含义 |
| --- | --- |
| `0` | 成功 |
| `1` | 框架初始化等内部错误 |
| `2` | 参数错误 / PI 校验失败 / 选择歧义 |
| `3` | 资源加载或 hash 校验失败 |
| `4` | 控制器发现或连接失败 |
| `5` | pretask 失败 |
| `6` | 任务执行失败 |
| `7` | 超时 |
| `8` | 被中断（Ctrl+C） |

## 9. 客户端配置文件

`maactl` 读取生态通行的 `maa_pi_config.json`（MaaPiCli / MFAA 格式），默认位置：

1. `--config <path>`；
2. `<PI 目录>/config/maa_pi_config.json`；
3. `<当前目录>/config/maa_pi_config.json`。

识别的字段：

| 字段 | 含义 |
| --- | --- |
| `controller` | 默认控制器名 |
| `resource` | 默认资源名 |
| `adb.address` / `adb.adb_path` / `adb.screencap` / `adb.input` | ADB 连接默认值 |
| `win32.*` | Win32 窗口选择与方式默认值 |
| `macos.*` | macOS 窗口选择与方式默认值 |
| `playcover.*` | PlayCover 服务地址与应用标识 |
| `linux.*` | Linux（wlroots）Wayland socket 等 |
| `option` | 全局配置项取值（结构同 §7.5 的严格 JSON） |
| `task[].name` + `task[].option` | 按任务保存的配置项取值 |
| `task[].enabled` | 任务勾选状态（仅记录与展示；`run preset` 不使用，见 §11） |

未知字段（如 MFAA 的 `__key`）忽略。`--no-config` 完全跳过；`--explain` 与
`config show` 会打印每个默认值的来源（`cli` / `config` / `preset` / `default`）。

密码字段在配置文件中必须是 `{"password": {"env": "NAME"}}` 或加密后的值；`maactl` 本身
不实现写回，`config show` 只做掩码展示。

## 10. 协议落实清单（对照 PI v2.10.1）

| 协议能力 | 版本 | maactl 处理 |
| --- | --- | --- |
| `interface_version` / 基础元数据 | v2 | 读取并在 `pi info` 展示 |
| `languages` + `$` i18n | v2 | `--lang` 解析 `label`/`description`；agent 环境变量使用解析后的文本 |
| `import` | v2.2.0 | 按协议合并 `task`/`option`/`preset`/`group`/`pretask`/`global_option`/`setting` |
| `controller.attach_resource_path` | v2.2.0 | 在 `resource.path` 之后、hash 校验之后依次加载 |
| `checkbox` / `option.controller` / `option.resource` / 各级 option / `focus.display` / `preset` | v2.3.0 | 全部实现 |
| option 适用性过滤 | v2.3.1 | 未激活 option 不产生 override |
| `group` / `task.group` | v2.4.0 | `pi groups` / `pi tasks --group` |
| Agent `PI_*` 环境变量 | v2.5.0 | 启动 agent 时注入（见下） |
| `resource.hash` | v2.6.0 | `resource inspect` 展示；运行前校验，默认告警，`--require-resource-hash` 失败 |
| `pretask`（含 `controller`/`resource` 过滤、`option` 传参） | v2.7.0 / v2.8.1 | 在创建控制器前顺序执行；option 序列化为单行 JSON 追加为最后一个参数 |
| `setting` | v2.8.0 | `pi settings` 展示；`global_option`/`setting` 可来自 `import` |
| `hotkey` | v2.8.0 | `--option`/`--option-file` 取值，按控制器类型转按键码 |
| `telemetry` | v2.9.0 | **不实现**（CLI 不上报遥测）；`pi info` 标注「已声明，未启用」 |
| `focus.trace` | v2.9.1 | 不实现遥测，读入后忽略 |
| `password` 输入字段 | v2.10.0 | 掩码、禁止 `default`、拒绝明文日志 |
| `checkbox.min_count`/`max_count` | v2.10.1 | 校验 |

`PI_*` 环境变量（v2.5.0）：`PI_INTERFACE_VERSION=v2.10.1`、`PI_CLIENT_NAME=MaaCtl`、
`PI_CLIENT_VERSION`、`PI_CLIENT_LANGUAGE`、`PI_CLIENT_MAAFW_VERSION`、`PI_VERSION`、
`PI_CONTROLLER`、`PI_RESOURCE`（后两者为已解析 i18n 的单行 JSON）。

## 11. 实现状态

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| P1 | 本文档 | ✅ |
| P2 | PI 数据模型 + 加载/import 合并 + i18n | ✅ |
| P3 | option 求值 + preset + 分层 override | ✅ |
| P4 | 新命令树（`pi`/`resource`/`device`/`run`/`config`） | ✅ |
| P5 | 运行期（pretask、`PI_*`、hash、explain/dry-run、退出码） | ✅ |
| P6 | README / docs / 测试 | ✅ |
| P7 | 命令与选项短别名、分组重排、帮助精简 | ✅ |

校验与计划外补充：`pi validate`（结构化报告）、`pi options` 配置项树、
`resource hash --verify`、客户端配置读取（`config show/path`）也一并实现。

尚未实现（有意为之）：

- `telemetry` / `focus.trace` 遥测上报（CLI 不上报遥测，`pi info` 仅提示已声明）；
- `display: modal` 的阻塞确认（CLI 只打印，不等待输入）；
- 写回客户端配置文件（`config set`）——配置文件由用户的 GUI 客户端负责写入；
- `run task` / `run preset` 会接受 `-pa`/`--path` 但静默忽略（该选项只在 `run node` 生效）；
  改成报错会改变退出码，属 BREAKING，待决策；
- `resource list` 会接受从 `resource` 组继承的 `-r`/`-pa`/`-ol` 但静默忽略（只读 `-c`）；
  改成报错同样属 BREAKING，待决策；
- 客户端配置里的 `task[].enabled` 不影响 `run preset`（当前仅记录与展示；若改为兜底会改变执行行为）；
- `controller.permission_required` 目前只在声明为真时打一条 stderr 提示，未实际提权；
- Linux（wlroots）下 `use_win32_vk_code` 的热键键码映射未实现（当前会在运行期失败）；
- `-e all` 看不到 `Resource.Loading` 与连接期 `Controller.Action` 事件（sink 注册时机晚于资源加载与连接）；
- 未知 option 字段（例如 `--option X.tokne=y` 这类拼写错误）被静默忽略，未报错；
- `pi options --all` 的 `reason` 在 controller 与 resource 同时不适用时只报前一条
  （`controller must be one of …`），而 `run --explain` 的 `Skipped.Reason` 会两条都报。
  两者对齐会改变 `pi options` 的对外文案，属待决策（`internal/pi/plan.go` 的 `planOption`
  与 `internal/pi/option.go` 的 `applicabilityReason` 是这两处实现）；
- npm 下载产物无哈希或签名锚点（需 release 侧先产出 SHA-256 清单）；`MAACTL_MIRROR` 允许明文 `http`。

## 12. 与旧版对照（迁移）

| 旧写法 | 新写法 |
| --- | --- |
| `maactl interface --show` | `maactl pi info`（`maactl pi i`） |
| `maactl interface --controllers/--tasks/--validate` | `maactl pi controllers/tasks/validate`（`c`/`t`/`v`） |
| `maactl interface --resources` | `maactl resource list`（`maactl resource l`） |
| `maactl interface --options/--presets` | `maactl pi options/presets` |
| `maactl adb devices` | `maactl device adb`（旧命令保留迁移提示） |
| `maactl win32 devices` | `maactl device window` |
| `maactl resource -i` / `resource -n` | `maactl resource inspect` / `resource nodes` |
| `maactl run task <name>` / `run -t` | 不变 |
| `maactl run node <name>` / `run -n` | 不变 |
| `--option/-p`（旧版计划） | `--option` / `-opt`；`-p` 改为 `--preset` |
| `--override-file/-O` | `--override-file` / `-ovf` |

短形式对照（常用）：

| 长形式 | 短形式 | 长形式 | 短形式 |
| --- | --- | --- | --- |
| `--interface` | `-f` / `-if` | `--lib-dir` | `-l` / `-lib` |
| `--json` | `-j` | `--lang` | `-lg` |
| `--config` | `-cfg` | `--no-config` | `-nocfg` |
| `--all` | `-all` | `--strict` | `-st` |
| `--group` | `-gr` | `--type` | `-ty` |
| `--path` | `-pa` | `--overlay` | `-ol` |
| `--verify` | `-vf` | `--option` | `-opt` |
| `--option-file` | `-of` | `--override-file` | `-ovf` |
| `--events` | `-e` | `--focus-display` | `-fd` |
| `--dry-run` | `-dr` | `--explain` | `-x` |
| `--timeout` | `-to` | `--stop-after` | `-sa` |
| `--no-agent` | `-na` | `--agent-log` | `-al` |
| `--require-resource-hash` | `-rh` | `--continue-on-error` | `-k` / `-coe` |
| `--name` | `-nm` | `--adb-path` | `-ap` |

> 短形式不再重复占用：`-v` 只在顶层是 `--version`（`interface --validate` 已改为 `pi validate`），
> `-r`/`-c` 始终是 resource/controller，`-i` 不再表示 interface 或 inspect。
