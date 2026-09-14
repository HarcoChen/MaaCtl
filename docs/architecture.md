# 目录结构与包职责

```text
cmd/maactl/            程序入口（main 包，仅解析参数并调用 cli）
internal/
  cli/                 命令树：root、adb/win32、interface、resource、run、execute、controller、agent
  pi/                  ProjectInterface v2 的数据模型、加载与查找
  event/               sink 事件输出与 focus 文本渲染
  help/                帮助渲染器（分区、继承来源、planned 标记）
  i18n/                帮助语言检测与本地化文本
  maafw/               MaaFramework 运行库定位与初始化
    pack/              从 MaaFramework release 压缩包生成内嵌 payload
    bundled/           内嵌 payload 的编译期承载与运行期解包
  output/              文本与 JSON 输出辅助
  table/               终端宽度对齐的表格渲染
tools/packmaafw/       打包工具：下载/解包 release，只取 bin/ 生成 payload
assets/                下载的 MaaFramework release 压缩包（不入库）
maafw.version          本项目使用的 MaaFramework 版本
npm/                   npm 分发包：npx maactl / npm i -g maactl 的参数转发器
tests/                 跨包测试（CLI 端到端、PI 加载与打包）
docs/                  设计文档
maafw/                 本地 MaaFramework 运行库（不入库）
```

`internal/` 内的包只在本模块可用；`tests/` 通过 `internal/cli` 等导出接口运行 CLI，
因此不需要把内部实现暴露给外部。

## 相关文档

- [cli.md](cli.md)：命令行参考
- [build.md](build.md)：构建、打包与运行库查找顺序
- [npm-package.md](npm-package.md)：npm 分发包的设计与实现
- [release.md](release.md)：发版与 npm 发布流程
- [maafw-cli-design.md](maafw-cli-design.md)、[help-optimization.md](help-optimization.md)：早期设计与帮助文本审计
