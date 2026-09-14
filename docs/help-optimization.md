# MaaCtl Help 优化方案

> 状态：**已实现**（P0/P1 已落地，实施说明与偏差见 §10；P2 未做）。
> 对象：`help.go`、各 `newXxxCommand` 的 `Short/Long/Example`、`README.md` 帮助章节。

## 1. 现状与证据

当前 `help.go` 的 `setFullHelp` 用一个自定义 `HelpFunc` 递归打印整个命令树：每到一个命令就打印 `UseLine()` 和它的全部本地旗标，根帮助因此把 `run`、`run task`、`run node` 的 14 个共享旗标各打印一遍（共约 45 行），整个根帮助 83 行。

复现命令：

```powershell
.\maactl.exe -h
.\maactl.exe run -h
.\maactl.exe run task -h
.\maactl.exe resource -h
.\maactl.exe resource nodes -h
.\maactl.exe adb devices -h
```

### 1.1 问题清单

| # | 级别 | 问题 | 证据 |
| --- | --- | --- | --- |
| H1 | P0 | **命令描述（Short）完全不显示**。递归列表只有 `UseLine()`，看不出命令用途。 | `maactl --help` 中只有 `maactl adb [flags]`、`maactl resource inspect [flags]`，没有 "Inspect ADB devices"、"Show loaded resource metadata" |
| H2 | P0 | **Long 文本是死内容**。自定义 `HelpFunc` 从不打印 `cmd.Long`。 | `run` 的 Long（"Run supports task, node and (planned) preset..."）在任何 `-h` 中都不可见 |
| H3 | P0 | **根帮助递归展开所有叶子的旗标**，`run` 共享旗标重复 3 次，信息密度极低（README 承诺"紧凑列表"）。 | 上述 83 行输出 |
| H4 | P0 | **继承旗标被错标为 "Global flags"**，同一旗标在不同入口的呈现不一致。 | `resource nodes -h` 把 `-r/--resource` 打印在 "Global flags:" 块；而 `resource -h` 里同一旗标又显示为 `Inherited flags: --resource (see parent command)` |
| H5 | P0 | **`adb devices` 重复定义 `-j/--json`**（与根持久旗标同义），导致该命令帮助里 global 块缺少 `-j`，与 `maactl -h` 不一致。 | `adb devices -h` 的 Global flags 只有 `-h/-f/-l`，`-j` 单独出现在命令区 |
| H6 | P0 | **planned 项呈现方式不统一**：命令靠 `Short` 后缀 `(planned)` 被字符串嗅探；参数用 `planned:` 前缀；`interface --options/--presets` 没有分组说明。planned 命令自身也没有描述。 | `maactl run preset <name> [flags] (planned)` 后无用途说明 |
| H7 | P1 | 根帮助包含 `completion bash/fish/powershell/zsh` 及其旗标，噪音大（设计文档从未提及 completion）。 | `maactl -h` 输出 |
| H8 | P1 | 没有任何示例、没有 footer 提示，也没有 "interface 恰好选择一个动作" 之类的约束说明。 | 全部 help 输出 |
| H9 | P1 | `-v` 语义跨命令冲突：根 `-v`=version，`interface -v`=validate，还占用了未来 `--verbose` 的常见短参。 | `maactl -v` vs `maactl interface -v` |
| H10 | P1 | 文档与实现漂移：设计文档写 `--interface/-i`，实现是 `-f`；设计文档写 `--events text\|jsonl\|off`，实现是 `focus\|all\|off`；`--lang/--log-dir/--verbose`、`run node --resource-path` 未实现也未标注。 | `docs/maafw-cli-design.md` vs 实际输出 |
| H11 | P2 | 旗标 usage 不换行，窄终端下 `--option`、`--overlay` 等长文本溢出。 | pflag `FlagUsages()` 默认不换行 |
| H12 | P2 | 未知旗标报错只打印 `Error: unknown flag`，没有 `--help` 提示（`SilenceUsage: true`）。 | `maactl run --bogus` |

### 1.2 与其他文件的相互影响

- `interface_test.go` 的 `TestInterfaceHelpUsesFlags` 断言**根帮助包含**所有 `--action` 旗标。若根帮助改为概览式，该测试必须同步改（断言迁移到 `maactl interface -h`）。
- `README.md` "帮助与 JSON" 章节承诺"顶层帮助列出所有命令、子命令及参数含义，全局参数只列一次"——现状 H1/H3/H5 与承诺不符，需一并改写。

## 2. 设计原则（Help Style Guide）

1. **三层密度**：
   - 根命令：一行一个子命令（名字 + 短描述）+ 全局旗标 + 示例，不展开子命令旗标；
   - 有子命令的命令：自身描述与旗标 + 一层子命令列表（带描述）+ 示例；
   - 叶子命令：完整旗标（本命令 + 继承的执行旗标 + 全局旗标）+ 示例。
2. **描述必备**：每个可见命令都有 `Short`（动宾短语、无句号）；顶层命令和 `run`/`interface`/`resource` 叶子有 `Long`；`Short/Long` 必须出现在 help 中。
3. **分区固定顺序**：Usage → 描述(Long) → Commands/Actions → Flags(本命令) → Execution Flags(继承) → Planned Flags → Global Flags → Examples → footer 提示。
4. **Global 只指根持久旗标**；其他祖先的旗标必须注明来源（`Inherited from "maactl resource"`），任何入口下呈现一致。
5. **planned 项**一律 `(planned)` 标记并归入独立分区；用 `cmd.Annotations`/`flag.Annotations` 标记，禁止再从 `Short` 字符串嗅探。
6. 旗标文案规范：小写短句、枚举可选取值、说明默认值与"可重复"，新文案示例见 §5。
7. 帮助输出写 stdout、退出码 0、不初始化 MaaFramework、不枚举设备。

## 3. 目标帮助形态（示意）

### 3.1 `maactl --help`（概览，目标 ≤ 30 行）

```text
MaaFramework and ProjectInterface command-line client

MaaCtl loads ProjectInterface v2 projects, inspects MaaFramework resources,
and runs Pipeline tasks.

Usage:
  maactl [command]

Commands:
  adb         Inspect ADB devices
  win32       Inspect Win32 desktop windows
  interface   Inspect and validate a ProjectInterface
  resource    Load and inspect PI resources
  run         Run PI tasks or Pipeline nodes
  completion  Generate a shell completion script

Global Flags:
  -f, --interface string   ProjectInterface file or project directory (default: ./interface.json)
  -l, --lib-dir string     MaaFramework DLL directory (default: ./maafw/bin)
  -j, --json               emit JSON on stdout; run also switches sink events to JSON
  -h, --help               help for maactl
      --version            print version information

Examples:
  maactl interface --show -f D:\projects\demo
  maactl run task "自动挂机卖蛋" -f D:\projects\demo --stop-after 10s
  maactl adb devices --json

Use "maactl <command> --help" for more information about a command.
```

### 3.2 `maactl run --help`（共享旗标只出现一次）

```text
Run PI tasks or Pipeline nodes.

Execute a task declared in ProjectInterface, or run a Pipeline node directly.
Give exactly one of --task/-t, --node/-n, or a subcommand.

Shortcuts:
  maactl run -t <task-name>   run a PI task
  maactl run -n <node-name>   run a Pipeline node

Usage:
  maactl run [flags]
  maactl run [command]

Commands:
  task <task-name>   Run a task declared in ProjectInterface
  node <node-name>   Run a Pipeline node from a PI resource
  preset <name>      Run the enabled tasks in a PI preset (planned)

Flags:
  -t, --task string   shortcut for "maactl run task <task-name>"
  -n, --node string   shortcut for "maactl run node <node-name>"

Execution Flags (shared by "run task" and "run node"):
  -r, --resource string        PI resource name (default: first compatible resource)
  ...

Planned Flags:
      --dry-run                resolve and display execution without connecting a controller (planned)
      ...

Global Flags:
  ...（同 3.1）

Examples:
  maactl run -t "自动挂机卖蛋" -f D:\MaaMio --stop-after 10s
  maactl run -n "签到-开始签到" --adb-address 127.0.0.1:16384 --events all

Use "maactl run [command] --help" for more information about a command.
```

### 3.3 `maactl run task --help`（叶子，执行旗标完整列出但标注来源）

```text
Run a task declared in ProjectInterface.

Usage:
  maactl run task <task-name> [flags]

Execution Flags (inherited from "maactl run"):
  ...（全部共享旗标，语义只定义一次）

Global Flags:
  ...（同 3.1）

Examples:
  maactl run task "自动挂机卖蛋" -f D:\MaaMio --stop-after 10s
```

### 3.4 `maactl interface --help`（动作分组，planned 置后）

```text
Inspect and validate a ProjectInterface.

Exactly one action is required. "interface show" style subcommands are not supported.

Usage:
  maactl interface [flags]

Actions:
  -s, --show          show ProjectInterface summary
  -c, --controllers   list controllers (name, label, type)
  -r, --resources     list resources (name, label, path)
  -t, --tasks         list tasks (name, label, entry)
  -v, --validate      validate ProjectInterface loading

Planned Actions:
      --options       list PI option definitions (planned)
      --presets       list PI presets (planned)

Global Flags:
  ...

Examples:
  maactl interface --tasks -f D:\MaaMio
  maactl interface --validate -f D:\MaaMio --json
```

### 3.5 `maactl resource --help` / `maactl adb devices --help`

- `resource`：`Flags: -i/--inspect、-n/--nodes、-r/--resource`（shortcut 与子命令各有一行描述），子命令列表末尾 `hash ... (planned)`；`-r` 归属本命令，不再标 Global。
- `adb devices`：只列 `-j` 继承自全局（删除本地重复定义），Global Flags 与根帮助一致。

## 4. 实施要点

### 4.1 渲染器改造（`help.go` 重写）

保留"自定义 `HelpFunc`"路线（Cobra 模板对分区/来源分类表达力不足，收益有限），重写为通用 `renderHelp(cmd, out)`：

1. **Usage**：根显示 `maactl [command]`；有子命令显示 `UseLine()` + `[command]`；叶子显示 `UseLine()`。
2. **描述**：打印 `cmd.Long`（为空回退 `cmd.Short`），按终端宽度或 100 列换行。
3. **Commands**：`cmd.Commands()` 中 `IsAvailableCommand()` 的命令输出 `Name + 参数占位 + Short`，`(planned)` 由 `cmd.Annotations["planned"]=="true"` 追加；**不递归**。
4. **Flags 分区**：
   - 本命令：`cmd.NonInheritedFlags()`；
   - 执行旗标：`cmd.InheritedFlags()` 中**非根**来源的旗标，标题写明来源命令（沿 `cmd.Parent()` 链找第一个定义该旗标的祖先）；
   - planned：按 `flag.Annotations["planned"]=="true"` 从上述两区抽出，统一放入 `Planned Flags`（附 `(planned)` 后缀）；
   - global：仅根的持久旗标 + `-h/--help` + `--version`；
   - 移除现有 `sameHelpFlag` 的"属性比对去重"逻辑（脆弱：同名同属性不同旗标会误隐藏）。
5. **Examples**：打印 `cmd.Example`；根与顶层命令必须提供。
6. **Footer**：有子命令时输出 `Use "maactl <cmd> [command] --help" ...`。
7. **`-h/--help`**：不再手工合成假旗标，直接用 Cobra 的 help flag 描述，保证与解析行为一致。

### 4.2 旗标定义去重（二选一）

- **方案 A（推荐）**：共享执行旗标注册一次到 `run.PersistentFlags()`；`-t/-n` 保持 `run` 局部；`run task`/`run node` 通过父命令读取选项。叶子帮助中它们自然落入 "Execution Flags (inherited from \"maactl run\")"，根帮助不再重复。验收解析用例：
  - `run -t x -r base --stop-after 10s`
  - `run task x -r base`
  - `run -r base task x`（父级旗标提前解析）
  - planned 旗标不因移动而改变报错文案。
- **方案 B（低改动）**：保留三处 `addRunFlags`，仅改渲染。实现快，但定义仍三处，后续文案改动易漏。

### 4.3 其他清理

- 删除 `adb devices` 的本地 `-j/--json`（`deviceOptions.json`），统一读 `global.json`；行为不变。
- `run preset`、`resource hash`、`--options/--presets`、planned 旗标全部改挂 `Annotations{"planned":"true"}`；`Short` 去掉 `(planned)` 后缀。
- 根版本旗标：预定义 `--version`（无 shorthand），保留 `root.Version`；把 `-v` 让给 `interface --validate` 与未来的 `--verbose`。若团队坚持 `-v`=version，则需在 `interface` 帮助中显著提示差异。
- completion：根命令列表中保留一行 `completion`，但不展开其子命令旗标（`IsAdditionalHelpTopicCommand` 或按名称过滤）；如希望完全隐藏，`root.CompletionOptions.DisableDefaultCmd = true`（命令仍可用）。
- （可选）`--lib-dir` 对 `interface` 无意义：P2 可按命令白名单隐藏无关全局旗标。

## 5. 文案调整清单（节选）

| 位置 | 现状 | 建议 |
| --- | --- | --- |
| 全局 `--interface` | `ProjectInterface file or directory (default: ./interface.json)` | `ProjectInterface file or project directory (default: ./interface.json in the current directory)` |
| 全局 `--lib-dir` | `MaaFramework DLL directory (default: ./maafw/bin)` | `MaaFramework runtime directory containing MaaFramework.dll and MaaToolkit.dll (default: ./maafw/bin)` |
| 全局 `--json` | `output JSON` | `emit JSON on stdout; run also switches sink events to JSON` |
| `run --events` | `sink output: focus (default), all, or off` | `event output: focus (PI focus text only), all (all sink events), off (default "focus")` |
| `run --stop-after` | `stop a running task after this duration (for bounded runs/tests)` | `stop the task after this duration; use for bounded runs and tests (e.g. 30s)` |
| `run --no-agent` | `do not start the ProjectInterface agent` | `do not launch the agent process declared in interface.json` |
| `run --option/-p` | `planned: option value as name=<JSON>; repeatable` | 移入 Planned Flags：`option value as name=<JSON>; repeatable (planned)` |
| `interface --validate` | `validate ProjectInterface loading` | `validate that the ProjectInterface loads` |
| `resource inspect/nodes` | Short 存在但不可见 | 渲染为子命令描述行 + 子命令 help 的 Long |
| 各 planned 命令 | `Short` 内含 `(planned)` | `Short` 纯描述；marker 由注解渲染 |

## 6. 测试与验收

- 更新 `TestInterfaceHelpUsesFlags`：根帮助断言改为"包含命令短描述、不含叶子旗标"；action 旗标断言迁移到 `maactl interface -h`。
- 新增 golden 测试：`root`、`run`、`run task`、`resource`、`resource nodes`、`adb devices` 六个帮助页；断言：
  - 每个可见命令行都带 Short；
  - 同一帮助页内任一旗标 usage 不重复出现；
  - `-h` 与 `--help`、`maactl help run` 与 `maactl run -h` 输出一致；
  - planned 项位于 Planned 分区；
  - `resource nodes -h` 与 `resource -h` 都把 `-r` 归为 resource 的旗标（来源一致）；
  - `adb devices -h` 的 Global Flags 与根一致。
- 解析回归：§4.2 的四组调用形式 + 现有 integration 测试全部通过。
- 验收标准：根帮助 ≤ 30 行；`run -h` 中共享旗标只出现一次；任何 help 都不触发 DLL 加载/设备枚举；`go test ./...` 通过。

## 7. 文档同步

- `README.md` "帮助与 JSON" 章节按新行为改写，并补充 `maactl help <command>`。
- `docs/maafw-cli-design.md`：
  - `--interface/-i` → `--interface/-f`（与实现、README 一致）；
  - `--events text|jsonl|off` → `focus|all|off`；
  - 未实现项（`--lang`、`--log-dir`、`--verbose`、`run node --resource-path`）标注 planned 或删除，保持"已公布契约"唯一来源。

## 8. 分阶段落地建议

| 阶段 | 内容 | 预估 |
| --- | --- | --- |
| Phase 1 (P0) | 渲染器重写（Short/Long、分区、depth=1、来源标注）、`-j` 去重、planned 注解、更新测试与 README | ~1 天 |
| Phase 2 (P1) | `run` 共享旗标持久化（方案 A）、Example、footer、interface 动作分组、版本旗标调整、completion 处理、设计文档同步 | ~0.5–1 天 |
| Phase 3 (P2) | 窄终端换行、`--help` 错误提示、按命令隐藏无关全局旗标 | 按需 |

## 9. 非目标

- 不实现任何 planned 功能，不改变命令/旗标语义（唯一行为等价改动：删除 `adb devices` 重复的 `-j`）。
- 不引入多语言帮助（表格中的中英文列名维持现状）。
- 不改动事件输出格式与运行逻辑。

## 10. 实施结果（已落地）

已完成：

- `help.go` 按本文 §4.1 重写：描述、单层命令列表、旗标分区、owner 来源标注、planned 分区、Examples、footer；删除 `sameHelpFlag` 属性比对。
- `run` 共享执行旗标只注册一次在 `run.PersistentFlags()`（方案 A），`run task`/`run node` 显示 `Execution Flags (inherited from "maactl run")`；`-t/-n` 仍为 `run` 局部。
- planned 已改为注解驱动：命令在列表中以 `(planned)` 标注，参数归入 `Planned Flags`；`Short` 不再含后缀。
- 删除 `adb devices` 重复的 `-j/--json`，统一使用全局旗标；`--version` 去掉 `-v` 短参。
- `interface` 动作用例归入 `Actions:`；`resource` 的 `-r` 在子命令中显示为 `Inherited Flags (from "maactl resource")`。
- 测试：更新 `interface_test.go`；新增 `help_test.go`（根概览、共享旗标去重、planned 分区、继承来源、`help` 与 `-h` 一致性、`--version`、旗标在子命令前后解析）。
- 帮助双语：新增 `lang.go` / `lang_windows.go` / `lang_other.go`；Windows 取用户默认 UI 语言，POSIX 读取 `LC_ALL`/`LC_MESSAGES`/`LANG`，中文显示中文、其余显示英文，可用 `MAACTL_LANG=zh_CN|en` 覆盖；全部命令描述、旗标文案、帮助分区标题与 pflag 的 `(default ...)` 注解均已本地化。

与设计稿的偏差：

- interface 的 planned 动作集中在 `Planned Flags`，未单独命名为 `Planned Actions`。
- P2 未做：窄终端换行、未知旗标后的 `--help` 提示、按命令隐藏无关全局旗标。
