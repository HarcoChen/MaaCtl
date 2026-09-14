# MaaCtl

`maactl` 是 [MaaFramework](https://github.com/MaaXYZ/MaaFramework) 的命令行客户端。它加载
ProjectInterface v2（PI）项目，查看控制器、资源与任务，并在指定的 ADB 设备、Win32 窗口或虚拟手柄等
控制器上运行 PI task 或 Pipeline 节点。

- 只提供 **Windows amd64** 的可执行文件；是否自带 MaaFramework 不影响命令行行为。
- 所有选项都以 `-` 或 `--` 开头，只有命令名和 task/node 名称使用位置参数。
- 默认从**当前工作目录**读取 `./interface.json`，可用 `-f/--interface` 指定 PI 项目目录或文件。

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
maactl interface --tasks -f D:\01_Projects\github\MaaMio
maactl adb devices
maactl run task "自动挂机卖蛋" -f D:\01_Projects\github\MaaMio --stop-after 10s
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
.\maactl.exe interface --tasks -f D:\01_Projects\github\MaaMio

# 把所在目录加进 PATH（当前会话生效），之后就能当普通命令用
$env:Path += ";D:\tools\maactl"
maactl -v
```

运行库的查找顺序见 [docs/build.md](docs/build.md#运行库查找顺序)。

### 方式三：从源码构建

见 [docs/build.md](docs/build.md)。

## 三十秒上手

```powershell
maactl -v                                        # 版本（自带版本会同时显示 MaaFramework 版本）
maactl interface --show                          # 读当前目录的 ./interface.json
maactl interface --tasks -f D:\path\to\project   # 列出 PI 中的 task
maactl adb devices                               # 查看 ADB 设备
maactl win32 devices                             # 查看 Win32 桌面窗口
maactl run task "任务名" -f D:\path\to\project    # 运行 task
maactl help run                                  # 查看某条命令的帮助
```

全局参数：

| 参数 | 作用 |
| --- | --- |
| `-f, --interface` | 指定 PI 文件或包含它的目录，默认 `./interface.json` |
| `-l, --lib-dir` | 指定 MaaFramework DLL 目录，覆盖自带运行库 |
| `-j, --json` | 以 JSON 输出，便于脚本处理 |
| `-h, --help` / `-v, --version` | 帮助 / 版本 |

运行相关的执行选项（`run` 下）：

| 参数 | 作用 |
| --- | --- |
| `-t, --task` / `-n, --node` | 运行 PI 中的 task，或直接运行 Pipeline 节点 |
| `-a, --adb-address` | ADB 设备序列号/地址，用 MaaToolkit 检测到的设备信息匹配 |
| `--name` | ADB 设备名称（`maactl adb devices` 显示的名称），用 MaaToolkit 检测到的设备信息匹配 |
| `--win32-handle` | Win32 窗口句柄（十进制或 `0x` 开头的十六进制） |
| `--win32-class` / `--win32-window` | Win32 窗口类名 / 标题正则（`maactl win32 devices` 可查看） |
| `--win32-screencap` / `--win32-mouse` / `--win32-keyboard` | 覆盖 Win32 截图与输入方式（默认取 PI `win32` 配置） |
| `--gamepad-type` | 虚拟手柄类型：`Xbox360`（默认）或 `DualShock4`（默认取 PI `gamepad.gamepad_type`） |
| `-c, --controller` / `-r, --resource` | 指定 PI controller 与资源 |
| `--events` | 事件输出：`focus`（默认）/ `all` / `off` |
| `--stop-after` | 运行指定时长后停止，适合验证与限时运行 |
| `-o, --override` / `-O, --override-file` | 最终 Pipeline override JSON 或文件 |

注意 `-v` 的含义随位置不同：顶层是 `--version`，`interface` 下是 `--validate`；`-c`/`-r` 同理，
在 `run` 下指 controller/resource，在 `interface` 下指 controllers/resources。

更多命令、事件输出与 Pipeline 覆盖见 [docs/cli.md](docs/cli.md)。

## 文档

| 文档 | 内容 |
| --- | --- |
| [docs/cli.md](docs/cli.md) | 命令参考：查看 PI、设备与资源、运行 task 与节点、事件输出、Pipeline 覆盖、帮助与 JSON、尚未实现的预留契约 |
| [docs/build.md](docs/build.md) | 从源码构建两种 exe、打包 MaaFramework、运行库查找顺序、版本注入与本地验证 |
| [docs/architecture.md](docs/architecture.md) | 目录结构与各包职责 |
| [docs/npm-package.md](docs/npm-package.md) | npm 分发包（`npx maactl`）的设计、环境变量与体积取舍 |
| [docs/release.md](docs/release.md) | 发版流程：推送 tag → GitHub Release → 自动发布 npm |
| [docs/maafw-cli-design.md](docs/maafw-cli-design.md) | 早期的 CLI 设计文档 |
| [docs/help-optimization.md](docs/help-optimization.md) | 帮助文本的现状审计与改进方案 |

## 环境要求

- 操作系统：Windows 10/11 x64（当前只提供该平台的 exe）。
- 用 npm 方式安装时额外需要 Node.js ≥ 22。
- 运行 ADB 任务时设备需已连接，且 `adb devices` 能看到设备；运行 Win32 任务时目标窗口需已打开。
