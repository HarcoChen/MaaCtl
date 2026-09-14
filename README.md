# MaaCtl

`maactl` 是 [MaaFramework](https://github.com/MaaXYZ/MaaFramework) 的命令行客户端。它加载
ProjectInterface v2（PI）项目，检查其中的控制器、资源、任务与配置项，并在指定的 ADB 设备、
Win32 窗口或虚拟手柄上运行 PI task、preset 或任意 Pipeline 节点。

- 只提供 **Windows amd64** 的可执行文件；是否自带 MaaFramework 不影响命令行行为。
- 只有命令名和必需的名称使用位置参数，其余都是 `-`/`--` 选项。
- 默认从**当前工作目录**读取 `./interface.json`，可用 `-f/--interface` 指定 PI 项目目录或文件。
- 会读取生态通行的 `config/maa_pi_config.json`，因此控制器、资源、ADB 设备通常不用每次手填。

## 快速开始

### 方式一：npm（推荐）

包内自带 MaaFramework，装完即用，不需要手动配 PATH、也不需要准备 DLL 目录：

```powershell
# 免安装试用
npx maactl -v

# 或装成全局命令（推荐，之后像普通命令一样使用）
npm install -g maactl
maactl -v
```

```powershell
maactl pi t -if D:\01_Projects\github\MaaMio
maactl device adb
maactl run -t 签到 -if D:\01_Projects\github\MaaMio -sa 30s
```

`maactl` 之后的参数原样透传给 `maactl.exe`（中文、含空格的路径、`-`/`--` 选项都支持），
工作目录不变，退出码也一致。包装器的查找顺序与环境变量见
[docs/npm-package.md](docs/npm-package.md)。

### 方式二：手动下载 maactl.exe

不想装 Node 时，从 [Releases](https://github.com/TanyaShue/MaaCtl/releases/latest) 直接下载：

| 文件 | 说明 | 大小 |
| --- | --- | --- |
| `maactl.exe` | 自带 MaaFramework，开箱即用（推荐） | 约 31 MiB |
| `maactl-lite.exe` | 不自带运行库，运行时从 `./maafw/bin` 或 `--lib-dir` 加载 | 约 5 MiB |

```powershell
# 放到任意目录后直接用绝对/相对路径调用
.\maactl.exe -v
.\maactl.exe pi t -if D:\01_Projects\github\MaaMio

# 把所在目录加进 PATH（当前会话生效），之后就能当普通命令用
$env:Path += ";D:\tools\maactl"
maactl -v
```

运行库的查找顺序见 [docs/build.md](docs/build.md#运行库查找顺序)。

### 方式三：从源码构建

见 [docs/build.md](docs/build.md)。

## 三十秒上手

```powershell
maactl -v                                      # 版本（自带版本会同时显示 MaaFramework 版本）
maactl pi info                                 # 读当前目录的 ./interface.json（可写 maactl pi i）
maactl pi t -if D:\path\to\project             # 列出 PI 中的 task
maactl pi o -t 任务名 -if D:\path\to\project     # 查看该任务会激活哪些配置项
maactl resource l -if D:\path\to\project       # 列出 PI 声明的资源（不加载）
maactl device adb                              # 查看 ADB 设备
maactl device win32                            # 查看 Win32 桌面窗口
maactl run -t "任务名" -if D:\path\to\project    # 运行 task
maactl run -t "任务名" -dr -x                   # 只看最终下发的 Pipeline override
maactl help run                                # 查看某条命令的帮助
```

命令分成五组，**每个命令与每个选项都有短形式**（命令别名 1–3 字母，选项用单字母或 `-if`
这类助记别名，帮助里都会列出）：

| 命令 | 别名 | 作用 |
| --- | --- | --- |
| `pi` | `if` | 检查 ProjectInterface：`info`/`validate`/`controllers`/`tasks`/`groups`/`options`/`presets`/`settings` |
| `resource` | `res` | 资源：`list`（声明）/`inspect`/`nodes`/`hash`（已加载） |
| `device` | `dev` | 列出 MaaToolkit 发现的设备：`device adb` / `device win32` |
| `run` | `r` | 执行：`run task <名字>` / `run preset <名字>` / `run node <名字>` |
| `config` | `cfg` | 查看客户端配置：`config path` / `config show` |

只读查询（`pi`、`resource`、`device`）在前，执行（`run`）在后；同一个对象的所有查询都只在
一个分组里（资源声明与已加载资源都在 `resource` 下）。

全局参数（`-h` 可看全部）：

| 短形式 | 长形式 | 作用 |
| --- | --- | --- |
| `-if` | `--interface` | 指定 PI 文件或包含它的目录，默认 `./interface.json` |
| `-lib` | `--lib-dir` | 指定 MaaFramework DLL 目录，覆盖自带运行库 |
| `-j` | `--json` | 以 JSON 输出，便于脚本处理 |
| `-lg` | `--lang` | 解析 PI `$label` 的语言（默认跟随系统） |
| `-cfg` / `-nocfg` | `--config` / `--no-config` | 指定或跳过客户端配置文件 |
| `-log` | `--log-dir` | MaaFramework 日志目录 |
| `-vb` | `--verbose` | 输出选择来源与合并细节 |
| `-h` / `-v` | `--help` / `--version` | 帮助 / 版本 |

执行选项（`run` 下，子命令共用；按用途分组）：

| 短形式 | 长形式 | 作用 |
| --- | --- | --- |
| `-t` / `-n` | `--task` / `--node` | 运行 PI 中的 task，或直接运行 Pipeline 节点（等价子命令） |
| `-r` / `-c` | `--resource` / `--controller` | 指定 PI 资源与控制器（默认取配置文件） |
| `-a` / `-nm` | `--adb-address` / `--name` | 按地址或名称选择 ADB 设备 |
| `-ap` | `--adb-path` | 覆盖 adb 可执行文件 |
| `-wh` / `-wc` / `-ww` | `--win32-handle` / `-class` / `-window` | 选择 Win32 窗口 |
| `-ws` / `-wm` / `-wk` | `--win32-screencap` / `-mouse` / `-keyboard` | 覆盖 Win32 截图与输入方式 |
| `-gt` | `--gamepad-type` | 虚拟手柄类型：`Xbox360`（默认）或 `DualShock4` |
| `-opt` / `-of` | `--option` / `--option-file` | 设置 PI 配置项取值 |
| `-p` | `--preset` | 把某个 preset 的取值应用到本次 task |
| `-o` / `-ovf` | `--override` / `--override-file` | 最终 Pipeline override JSON |
| `-pa` / `-ol` | `--path` / `--overlay` | 替换或追加资源根目录 |
| `-dr` / `-x` | `--dry-run` / `--explain` | 预演并打印选择与各层覆盖，不执行 |
| `-e` / `-fd` | `--events` / `--focus-display` | 事件与 focus 输出控制 |
| `-to` / `-sa` | `--timeout` / `--stop-after` | 限时运行 |
| `-rh` | `--require-resource-hash` | `resource.hash` 不匹配时失败 |

退出码固定：`0` 成功、`2` 参数/PI、`3` 资源、`4` 控制器、`5` pretask、`6` 任务失败、
`7` 超时、`8` 中断。

更多命令、配置项语法、覆盖顺序与迁移对照见 [docs/cli.md](docs/cli.md)。

## 文档

| 文档 | 内容 |
| --- | --- |
| [docs/cli.md](docs/cli.md) | 命令参考：短形式一览、退出码、配置文件、各组命令、配置项语法、迁移对照 |
| [docs/pi-cli-design.md](docs/pi-cli-design.md) | 现行 CLI 设计（第二版）：设计原则、命令面、协议落实清单、实现分期 |
| [docs/build.md](docs/build.md) | 从源码构建两种 exe、打包 MaaFramework、运行库查找顺序、版本注入与本地验证 |
| [docs/architecture.md](docs/architecture.md) | 目录结构与各包职责 |
| [docs/npm-package.md](docs/npm-package.md) | npm 分发包（`npx maactl`）的设计、环境变量与体积取舍 |
| [docs/release.md](docs/release.md) | 发版流程：推送 tag → GitHub Release → 自动发布 npm |
| [docs/maafw-cli-design.md](docs/maafw-cli-design.md) | 第一版 CLI 设计（已废弃，仅作历史记录） |

## 环境要求

- 操作系统：Windows 10/11 x64（当前只提供该平台的 exe）。
- 用 npm 方式安装时额外需要 Node.js ≥ 22。
- 运行 ADB 任务时设备需已连接，且 `adb devices` 能看到设备；运行 Win32 任务时目标窗口需已打开。
