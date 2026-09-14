package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	maa "github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/spf13/cobra"
)

// version is the local development version; release builds inject the tag
// version through -ldflags "-X main.version=<version>".
var version = "0.1.0"

type cliOptions struct {
	libDir, interfacePath string
	json                  bool
}

type adbDeviceOutput struct {
	Name            string `json:"name"`
	AdbPath         string `json:"adb_path"`
	Address         string `json:"address"`
	ScreencapMethod string `json:"screencap_method"`
	InputMethod     string `json:"input_method"`
	Config          string `json:"config,omitempty"`
}
type desktopWindowOutput struct {
	Handle     string `json:"handle"`
	ClassName  string `json:"class_name"`
	WindowName string `json:"window_name"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	root := newRootCommand()
	root.SetArgs(args)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	return root.Execute()
}

func newRootCommand() *cobra.Command {
	// Keep the intentional command order instead of alphabetical sorting.
	cobra.EnableCommandSorting = false
	var global cliOptions
	root := &cobra.Command{
		Use:     "maactl",
		Version: version,
		Short:   localized("MaaFramework and ProjectInterface command-line client", "MaaFramework 与 ProjectInterface 命令行客户端"),
		Long: localized(`MaaCtl loads ProjectInterface v2 projects, inspects MaaFramework resources,
and runs Pipeline tasks.

Use positional arguments only for commands and required task/node names.
Every option starts with - or --.`, `MaaCtl 加载 ProjectInterface v2 项目，检查 MaaFramework 资源，
并运行 Pipeline 任务。

只有命令名和必需的 task/node 名称使用位置参数。所有选项以 - 或 -- 开头。`),
		Example: `  maactl interface --show -f D:\projects\demo
  maactl run task "自动挂机卖蛋" -f D:\projects\demo --stop-after 10s
  maactl adb devices --json`,
		SilenceErrors: true, SilenceUsage: true, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.PersistentFlags().StringVarP(&global.libDir, "lib-dir", "l", "", localized("MaaFramework DLL directory (default: ./maafw/bin)", "MaaFramework DLL 目录（默认：./maafw/bin）"))
	root.PersistentFlags().StringVarP(&global.interfacePath, "interface", "f", "", localized("ProjectInterface file or project directory (default: ./interface.json)", "ProjectInterface 文件或项目目录（默认：./interface.json）"))
	root.PersistentFlags().BoolVarP(&global.json, "json", "j", false, localized("output JSON; run emits sink events as JSON", "输出 JSON；运行时 sink 事件也输出 JSON"))
	// Define --version without a shorthand so -v stays reserved for
	// flag shorthands such as "interface --validate".
	root.Flags().Bool("version", false, localized("print version information", "显示版本信息"))
	root.AddCommand(newADBCommand(&global), newWin32Command(&global), newInterfaceCommand(&global), newResourceCommand(&global), newRunCommand(&global))
	// Create the default completion command now so its help can be localized.
	root.InitDefaultCompletionCmd()
	localizeCompletion(root)
	setFullHelp(root)
	return root
}

// localizeCompletion translates the cobra-generated completion command. Its
// English descriptions are kept as-is for the English help language.
func localizeCompletion(root *cobra.Command) {
	if activeLanguage() != langZH {
		return
	}
	for _, cmd := range root.Commands() {
		if cmd.Name() != "completion" {
			continue
		}
		cmd.Short = "为指定的 shell 生成自动补全脚本"
		cmd.Long = "为指定的 shell 生成 maactl 的自动补全脚本。\n每个子命令的帮助中包含生成脚本的使用方法。"
		for _, child := range cmd.Commands() {
			child.Short = fmt.Sprintf("为 %s 生成自动补全脚本", child.Name())
			child.Long = fmt.Sprintf("为 %s 生成 maactl 自动补全脚本。\n", child.Name())
			if flag := child.Flags().Lookup("no-descriptions"); flag != nil {
				flag.Usage = "禁用补全描述"
			}
		}
		return
	}
}

func newADBCommand(global *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use: "adb", Short: localized("Inspect ADB devices", "查看 ADB 设备"),
		Long: localized("List ADB devices discovered by MaaToolkit.\n\nUse \"maactl adb devices\" to see addresses and recommended connection methods.", "列出 MaaToolkit 发现的 ADB 设备。\n\n使用 \"maactl adb devices\" 查看设备地址和建议的连接方式。"),
		Example: `  maactl adb devices
  maactl adb devices --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(newDevicesCommand(global, "adb"))
	return cmd
}

func newWin32Command(global *cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use: "win32", Short: localized("Inspect Win32 desktop windows", "查看 Win32 桌面窗口"),
		Long: localized("List Win32 desktop windows discovered by MaaToolkit.\n\nUse \"maactl win32 devices\" to see window classes and handles.", "列出 MaaToolkit 发现的 Win32 桌面窗口。\n\n使用 \"maactl win32 devices\" 查看窗口类名和句柄。"),
		Example: `  maactl win32 devices
  maactl win32 devices --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(newDevicesCommand(global, "win32"))
	return cmd
}

func newDevicesCommand(global *cliOptions, kind string) *cobra.Command {
	long := localized("Windows are discovered through MaaToolkit and include the window name, class, and handle.", "窗口由 MaaToolkit 发现，包含窗口名称、类名和句柄。")
	if kind == "adb" {
		long = localized("Devices are discovered through MaaToolkit and include the address, ADB path,\nand recommended screencap and input methods.", "设备由 MaaToolkit 发现，包含地址、ADB 路径以及建议的截图和输入方式。")
	}
	cmd := &cobra.Command{
		Use: "devices", Short: fmt.Sprintf(localized("List %s devices", "列出 %s 设备"), kind), Long: long, Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			libDir, err := resolveLibDir(global.libDir)
			if err != nil {
				return err
			}
			if err := maa.Init(maa.WithLibDir(libDir), maa.WithStdoutLevel(maa.LoggingLevelOff)); err != nil {
				return fmt.Errorf("initialize MaaFramework from %s: %w", libDir, err)
			}
			defer func() { _ = maa.Release() }()
			if kind == "adb" {
				return listADB(global.json)
			}
			return listWin32(global.json)
		},
	}
	return cmd
}

func listADB(jsonOutput bool) error {
	devices := maa.FindAdbDevices()
	if jsonOutput {
		out := make([]adbDeviceOutput, 0, len(devices))
		for _, d := range devices {
			out = append(out, adbDeviceOutput{Name: d.Name, AdbPath: d.AdbPath, Address: d.Address, ScreencapMethod: d.ScreencapMethod.String(), InputMethod: d.InputMethod.String(), Config: d.Config})
		}
		return printJSON(out)
	}
	if len(devices) == 0 {
		fmt.Println("No ADB devices found.")
		return nil
	}
	fmt.Printf("Found %d ADB device(s):\n", len(devices))
	for i, d := range devices {
		fmt.Printf("[%d] %s\n", i+1, valueOrDash(d.Name))
		fmt.Printf("    address: %s\n", valueOrDash(d.Address))
		fmt.Printf("    adb: %s\n", valueOrDash(d.AdbPath))
		fmt.Printf("    screencap: %s\n", valueOrDash(d.ScreencapMethod.String()))
		fmt.Printf("    input: %s\n", valueOrDash(d.InputMethod.String()))
	}
	return nil
}

func listWin32(jsonOutput bool) error {
	windows := maa.FindDesktopWindows()
	if jsonOutput {
		out := make([]desktopWindowOutput, 0, len(windows))
		for _, w := range windows {
			out = append(out, desktopWindowOutput{Handle: strconv.FormatUint(uint64(uintptr(w.Handle)), 16), ClassName: w.ClassName, WindowName: w.WindowName})
		}
		return printJSON(out)
	}
	if len(windows) == 0 {
		fmt.Println("No Win32 windows found.")
		return nil
	}
	fmt.Printf("Found %d Win32 window(s):\n", len(windows))
	for i, w := range windows {
		fmt.Printf("[%d] %s\n", i+1, valueOrDash(w.WindowName))
		fmt.Printf("    class: %s\n", valueOrDash(w.ClassName))
		fmt.Printf("    handle: 0x%s\n", strconv.FormatUint(uint64(uintptr(w.Handle)), 16))
	}
	return nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
func valueOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func resolveLibDir(explicit string) (string, error) {
	candidates := make([]string, 0, 4)
	if explicit != "" {
		candidates = append(candidates, explicit)
	} else {
		if cwd, err := os.Getwd(); err == nil {
			candidates = append(candidates, filepath.Join(cwd, "maafw", "bin"))
		}
		if exe, err := os.Executable(); err == nil {
			d := filepath.Dir(exe)
			candidates = append(candidates, filepath.Join(d, "maafw", "bin"), filepath.Join(d, "..", "maafw", "bin"))
		}
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		absolute = filepath.Clean(absolute)
		if seen[absolute] {
			continue
		}
		seen[absolute] = true
		if fileExists(filepath.Join(absolute, "MaaFramework.dll")) && fileExists(filepath.Join(absolute, "MaaToolkit.dll")) {
			return absolute, nil
		}
	}
	if explicit != "" {
		return "", fmt.Errorf("MaaFramework DLLs not found in %q (expected MaaFramework.dll and MaaToolkit.dll)", explicit)
	}
	return "", fmt.Errorf("MaaFramework DLLs not found; expected them under %q", filepath.Join(".", "maafw", "bin"))
}
func fileExists(path string) bool { info, err := os.Stat(path); return err == nil && !info.IsDir() }
