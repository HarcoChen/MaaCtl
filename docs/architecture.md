# 目录结构与包职责

```text
cmd/maactl/            程序入口（main 包：解析参数、把 ExitError 映射为退出码）
internal/
  cli/                 命令树：root、pi、pi_options、resource、device、run、execute、
                       config、controller、window、agent、selfcheck、emit、exit、aliases
  pi/                  ProjectInterface v2 模型、加载与 import 合并、i18n、校验、
                       option 求值、Pipeline 合并、按键码表
  platform/            MaaFramework 发布平台（win/linux/macos × x86_64/aarch64）：
                       平台 id、运行库文件名、可执行文件后缀、可执行文件头架构探测
  clientconfig/        客户端配置（maa_pi_config.json）读取与发现
  event/               sink 事件输出、focus 模板与 display 渠道过滤
  help/                帮助渲染器（按用途分组的选项、短别名、声明顺序）
  i18n/                帮助语言检测与本地化文本
  maafw/               MaaFramework 运行库定位与初始化
    pack/              从本地 MaaFramework 运行库目录生成内嵌 payload
    bundled/           内嵌 payload 的编译期承载与运行期解包
  output/              文本与 JSON 输出辅助
  table/               终端宽度对齐的表格渲染
tools/packmaafw/       打包工具：只读 maafw/bin，生成 payload（不联网）
maafw.version          本项目使用的 MaaFramework 版本
npm/                   npm 分发包：npx maactl / npm i -g maactl 的参数转发器
.github/scripts/       CI 脚本：fetch_maafw.py（下载并解包 release）、
                       build_release.py（构建 + 验签 + 打包一个平台）、release.py（tag 与更新日志）
tests/                 跨包测试（CLI 端到端、PI 加载/合并/求值/校验、打包、平台探测）
docs/                  设计文档
maafw/                 本地 MaaFramework 运行库（不入库，CI 在每个平台上解包 release 到这里）
```

`internal/` 内的包只在本模块可用；`tests/` 通过 `internal/cli` 等导出接口运行 CLI，
因此不需要把内部实现暴露给外部。

## internal/pi 的拆分

| 文件 | 内容 |
| --- | --- |
| `model.go` | PI v2.10.1 全字段模型、保序 `OptionMap`、`DefaultCase`、`agent`/`pretask` 的单对象或数组 |
| `load.go` | 文件/目录解析、JSONC、按协议的 import 收集与合并顺序 |
| `i18n.go` | `languages` 协商与 `$label` 解析（含对 JSON 值整体解析，供 `PI_*` 使用） |
| `lookup.go` | 控制器/资源/任务/preset/group 解析、适用性判断、平台可运行类型 |
| `validate.go` | 结构化校验报告（error/warning）与 `--strict` 语义 |
| `option.go` | option 求值：取值优先级、嵌套选项、checkbox 计数、input/hotkey 转换、密码掩码 |
| `pipeline.go` | Pipeline override 合并与模板替换（整串占位符保留类型） |
| `plan.go` | `pi options` 的只读配置项树 |
| `hotkey.go` | 快捷键字符串解析与 Adb/Win32 虚拟按键码映射 |

## 平台差异

平台相关的东西集中在三处，其余代码与平台无关：

| 位置 | 内容 |
| --- | --- |
| `internal/platform` | 平台 id、运行库文件名（dll/so/dylib）、`.exe` 后缀、从 PE/ELF/Mach-O 头读架构 |
| `internal/pi/lookup.go` | `RunnableControllerTypes()`：各平台能创建的控制器类型（Windows `Win32`/`Gamepad`，macOS `MacOS`/`PlayCover`，Linux `Linux`，全平台 `Adb`） |
| `internal/cli/controller.go` | 各控制器类型的构造与参数解析；不支持的组合在调用 MaaFramework 之前就报错 |

## 命令与短别名

每个命令都有 1–3 字母别名，每个选项都有单字母 shorthand 或 2–3 字母助记别名：

- **命令别名**用 cobra 原生 `Aliases`（`pi` → `if`/`interface`，`pi tasks` → `t` 等）。
- **选项别名**由 `internal/cli/aliases.go` 的 `flagAliases` 表驱动：多字母形式（pflag 只支持
  单字符，无法注册为 shorthand）在解析前由 `NormalizeArgs` 统一改写成 `--long`，同一张表
  同时给帮助渲染器提供要显示的别名。因此“能写的”与“帮助里列的”总是一致。
- 别名在全命令树内唯一，只对 `--` 之前的参数生效；`--` 之后的参数原样传递。

`preserveFlagOrder` 关闭 pflag 的字母序排序（包括 cobra 懒创建的 `lflags`/`iflags`），
使帮助里的选项按代码声明顺序排列，同类选项自然相邻。

## 失败与退出码

`internal/cli` 用 `ExitError{Code, Err}` 给失败标注退出码；`cmd/maactl` 只负责把它翻译成
进程退出码。查询命令在参数或 PI 有问题时返回 2，运行时按阶段返回 3（资源）、4（控制器）、
5（pretask）、6（任务）、7（超时）、8（中断）。细节见 [cli.md](cli.md#退出码)。

## 相关文档

- [cli.md](cli.md)：命令行参考
- [pi-cli-design.md](pi-cli-design.md)：现行 CLI 设计（第二版）
- [build.md](build.md)：构建、打包与运行库查找顺序
- [npm-package.md](npm-package.md)：npm 分发包的设计与实现
- [release.md](release.md)：发版与 npm 发布流程
- [maafw-cli-design.md](maafw-cli-design.md)：第一版设计（已废弃）
