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

  pi info                       PI 概览（名称、版本、数量、语言、能力开关）
  pi validate                  校验 PI（可加载 / 引用完整 / 约束满足）
  pi controllers               列出控制器
  pi resources                 列出资源
  pi tasks                     列出任务（分组、可用性、启用状态）
  pi groups                    列出任务分组
  pi options                   列出（可实例化的）配置项树
  pi presets                   列出预设
  pi settings                  列出设置页分区

  device adb                   列出 MaaToolkit 发现的 ADB 设备
  device win32                 列出 MaaToolkit 发现的桌面窗口

  resource inspect             加载资源并显示路径、hash、节点数
  resource nodes               列出资源中的 Pipeline 节点
  resource hash                打印资源 hash，可校验 resource.hash

  run task <task-name>         运行 PI task（可用 --preset 套用预设取值）
  run preset <preset-name>     按 preset 顺序运行启用的 task
  run node <node-name>         直接运行 Pipeline 节点
  run -t <task-name>           快捷形式（等价 run task）
  run -n <node-name>           快捷形式（等价 run node）

  config show                  显示将生效的客户端配置及来源
  config path                  显示配置文件的解析路径

  version                      显示版本（等价 maactl -v）
```

命名说明：

- `pi` 有别名 `interface`，保留旧用户肌肉记忆；`interface` 不再接受 `--show` 之类的动作开关。
- `device adb` / `device win32` 取代旧的 `adb devices` / `win32 devices`；旧命令保留一个版本，
  隐藏并打印迁移提示。
- `run -t/-n` 保留，因为它们是纯便捷形式，且与子命令语义完全一致；`run preset`
  没有短参数，因为 `-p`/`--preset` 已经是「把预设取值应用到单个 task」的执行选项。

## 4. 全局选项

| 参数 | 作用 |
| --- | --- |
| `-f, --interface <path>` | PI 文件或包含 `interface.json` 的目录；默认**进程启动目录**的 `./interface.json` |
| `-l, --lib-dir <dir>` | MaaFramework 运行库目录；默认 `./maafw/bin`，其次 exe 相邻目录 |
| `-j, --json` | 结构化 JSON 输出；运行中 sink 事件也变为 JSON 行 |
| `--lang <code>` | 解析 PI `$label` 的语言，如 `zh_cn`/`en_us`；默认跟随系统（`MAACTL_LANG` 可覆盖） |
| `--config <path>` | 客户端配置文件；默认自动发现 `<PI 目录>/config/maa_pi_config.json` |
| `--no-config` | 不读取客户端配置文件 |
| `--log-dir <dir>` | MaaFramework 日志目录 |
| `--verbose` | 输出选择来源、加载路径与合并层级 |
| `-h, --help` / `-v, --version` | 帮助 / 版本（`-v` 只在顶层是版本） |

**短参数分配是不重叠的**：`-f/-l/-j/-h/-v` 属于全局；`-r/-c` 属于 `run`/`pi`/`resource` 的
「resource/controller」语义；`-t/-n` 属于 `run` 的快捷形式；`-o` 是 `--override`。
`--option`、`--option-file`、`--override-file`、`--preset` 只在长参数里出现，
避免同一字母被指到两件事上。

## 5. `pi` 子命令

所有 `pi` 子命令只读取 PI（加上可选的配置文件与 `--lang`），**不加载资源、不连接控制器**，
因此可以在没有设备时使用。

| 命令 | 关键选项 | 说明 |
| --- | --- | --- |
| `pi info` | | 名称/显示名/版本/协议版本/控制器数/资源数/任务数/分组数/预设数/语言列表/是否声明 agent、pretask、telemetry |
| `pi validate` | `--strict` | 默认只报错误；`--strict` 把「资源路径不存在」「hash 不匹配」「当前平台不支持的控制器」也视为错误 |
| `pi controllers` | `--type <Adb\|Win32\|...>` | 显示 `name/label/type/是否可运行/引用它的资源数` |
| `pi resources` | `--controller <name>` | 显示 `name/label/path/hash/controller 限制/是否与所选控制器兼容` |
| `pi tasks` | `--controller` `--resource` `--group` `--all` | 显示 `name/label/entry/group/option 数`；默认隐藏与当前 controller/resource 不兼容的 task，`--all` 显示并标注 `reason` |
| `pi groups` | | 分组 `name/label/default_expand/任务数` |
| `pi options` | `--task` `--resource` `--controller` `--all` | 按 `global_option → resource → controller → task` 顺序列出会被激活的 option 树（含类型、默认值、层级、来源），`--all` 列出全部定义 |
| `pi presets` | | 预设 `name/label/任务数`；`--json` 返回完整快照 |
| `pi settings` | | 分区 `name/label/option 列表` |

`pi tasks`/`pi options` 的可用性判断遵循协议：

- `task.controller`/`task.resource` 为空表示不限制；
- `option.controller`/`option.resource` 为空表示不限制；
- 被过滤掉的项在文本里不会静默消失，`--all` 会带 `unavailable: <原因>` 列出。

## 6. `run` 子命令

`run task|preset|node` 共享同一套执行选项：

| 参数 | 作用 |
| --- | --- |
| `-r, --resource <name>` | PI 资源名（默认：配置文件 → 第一个与控制器兼容的资源） |
| `-c, --controller <name>` | PI 控制器名（默认：配置文件 → 唯一的控制器） |
| `-a, --adb-address <serial>` | ADB 设备地址（默认：配置文件 → 唯一检测到的设备） |
| `--name <name>` | ADB 设备名（MaaToolkit 报告的名称） |
| `--adb-path <path>` | 覆盖 ADB 可执行文件路径 |
| `--win32-handle/--win32-class/--win32-window` | Win32 窗口选择（默认：配置文件 → PI `win32` 正则 → 唯一窗口） |
| `--win32-screencap/--win32-mouse/--win32-keyboard` | 覆盖 Win32 截图/输入方式 |
| `--gamepad-type <Xbox360\|DualShock4>` | 虚拟手柄类型 |
| `--option <name>=<value>` | 设置配置项取值；可重复（语法见 §7） |
| `--option-file <path>` | 配置项取值 JSON 文件（结构同 `preset.task[].option`） |
| `-p, --preset <name>` | 载入预设的任务启用状态与配置项取值 |
| `-o, --override <json>` / `--override-file <path>` | 最终 Pipeline override，优先级最高，二者互斥 |
| `--overlay <dir>` | 在所选资源之后追加加载的资源根目录，可重复 |
| `--events <focus\|all\|off>` | 事件输出，默认 `focus` |
| `--focus-display <list>` | 关注哪些 focus 渠道，默认 `log`；可写 `log,toast,notification,dialog,modal` 或 `all`（CLI 中 `modal` 不阻塞） |
| `--dry-run` | 只做选择、资源加载与覆盖计算，不连接控制器、不跑 pretask、不执行节点 |
| `--explain` | 打印/输出选择、各层 option 与最终 override（配合 `--dry-run` 只算不跑） |
| `--timeout <duration>` | 总时限；超时 `PostStop` 并以退出码 7 结束 |
| `--stop-after <duration>` | 运行指定时长后停止并视为成功（调试用） |
| `--no-agent` / `--agent-log <term\|off\|dir>` | 与旧版一致 |
| `--require-resource-hash` | `resource.hash` 不匹配时直接失败（默认仅告警） |

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

## 8. 输出、事件与退出码

- 查询类命令：文本为对齐表格（`--json` 为结构化 JSON）。
- 运行类命令：进度与 focus 文本走 stdout；`--json` 时最终摘要也走 stdout，sink 事件为 JSON 行。
- `run` 的成功摘要（`--json`）：

```json
{
  "interface": "D:\\01_Projects\\github\\MaaMio\\interface.json",
  "resource": "base",
  "controller": "Android",
  "entry": "签到-开始签到",
  "task_id": 3,
  "status": "Succeeded",
  "elapsed_ms": 12034,
  "resource_hash": "…",
  "effective_override": { "…": {} },
  "selections": [
    { "name": "复现次数", "source": "cli", "value": "x3" }
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
| `option` | 全局配置项取值（结构同 §7.5 的严格 JSON） |
| `task[].name` + `task[].option` | 按任务保存的配置项取值 |
| `task[].enabled` | 任务勾选状态（`run preset` 时作为补充） |

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

## 11. 实现分期

| 阶段 | 内容 | 交付 |
| --- | --- | --- |
| P1 | 本文档 | 设计评审基线 |
| P2 | PI 数据模型 + 加载/import 合并 + i18n | `pi.Load` 覆盖 v2.10.1 全字段，单测覆盖合并顺序 |
| P3 | option 求值 + preset + 分层 override | `pi.Resolve`，可用 sample interface.json 与 MaaMio 验证 |
| P4 | 新命令树（`pi`/`device`/`resource`/`run`/`config`） | 短参数不再重载，帮助分层 |
| P5 | 运行期（pretask、`PI_*`、hash、explain/dry-run、退出码） | 端到端可跑 MaaMio 的签到任务 |
| P6 | README / docs/cli.md / 迁移说明 / 测试 | 文档与代码一致 |

## 12. 与旧版对照（迁移）

| 旧写法 | 新写法 |
| --- | --- |
| `maactl interface --show` | `maactl pi info` |
| `maactl interface --controllers/--resources/--tasks/--validate` | `maactl pi controllers/resources/tasks/validate` |
| `maactl interface --options/--presets` | `maactl pi options/presets` |
| `maactl adb devices` | `maactl device adb`（旧命令保留迁移提示） |
| `maactl win32 devices` | `maactl device win32` |
| `maactl resource -i` / `resource -n` | `maactl resource inspect` / `resource nodes` |
| `maactl run task <name>` / `run -t` | 不变 |
| `maactl run node <name>` / `run -n` | 不变 |
| `--option/-p`（旧版计划） | `--option`（无短参数）；`-p` 改为 preset |
| `--override/-o`、`--override-file/-O` | `-o, --override` 不变；`-O` 不再使用，`--override-file` 只用长参数 |

> 短参数冲突提醒：新版里 `-p` 是 **preset**，`-o` 是 **override**；`--option`、`--option-file`、
> `--override-file` 都不设短参数，避免再把同一字母指到两件事上。旧设计文档里 `-p` 表示 option、
> `-O` 表示 override-file，均已废弃。
