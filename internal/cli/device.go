package cli

import (
	"fmt"
	"io"

	"maactl/internal/i18n"
	"maactl/internal/maafw"
	"maactl/internal/output"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/spf13/cobra"
)

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

// newDeviceCommand builds the `device` group: it lists what MaaToolkit can see
// on this machine, which is what the ADB and window selection flags match
// against on every platform.
func newDeviceCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "device",
		Aliases: []string{"dev"},
		Short:   i18n.Text("List devices and windows", "列出设备与窗口"),
		Long: i18n.Text(`Devices and windows are discovered through MaaToolkit. The reported addresses,
names, classes, and handles are exactly what "run" accepts.`, `设备与窗口由 MaaToolkit 发现。这里输出的地址、名称、类名、句柄就是 "run"
接受的取值。`),
		Example: `  maactl device adb
  maactl device window -j`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(newDeviceListCommand(global, "adb"), newDeviceListCommand(global, "window"))
	return cmd
}

// newDeviceListCommand builds `device adb` / `device window`.
func newDeviceListCommand(global *GlobalOptions, kind string) *cobra.Command {
	short := i18n.Text("List ADB devices", "列出 ADB 设备")
	long := i18n.Text("Reports the address, ADB path, and recommended screencap and input methods.", "报告地址、ADB 路径以及建议的截图与输入方式。")
	aliases := []string{"a"}
	example := `  maactl device adb -j`
	if kind == "window" {
		short = i18n.Text("List desktop windows", "列出桌面窗口")
		long = i18n.Text(`Reports the window name, class, and handle, which are exactly the values the
Win32, Gamepad, and macOS controller flags accept. The class is the window class
on Windows and the bundle identifier on macOS; the handle is the window handle,
the macOS window id, or the X11 window id.`, `报告窗口名称、类名与句柄，也就是 Win32、Gamepad 与 macOS 控制器参数接受的取值。
类名在 Windows 上是窗口类名，在 macOS 上是应用标识；句柄则是窗口句柄、macOS 窗口 id
或 X11 窗口 id。`)
		aliases = []string{"w", "win32"}
		example = `  maactl device window -j`
	}
	return &cobra.Command{
		Use: kind, Aliases: aliases, Short: short, Long: long, Example: example, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withMaaFramework(global, func() error {
				if kind == "adb" {
					return listADB(cmd.OutOrStdout(), global.JSON)
				}
				return listWindows(cmd.OutOrStdout(), global.JSON)
			})
		},
	}
}

// newLegacyDeviceCommands returns the pre-redesign `adb devices` and
// `win32 devices` commands. They keep working for one release so existing
// scripts do not break, and point at their replacement.
func newLegacyDeviceCommands(global *GlobalOptions) []*cobra.Command {
	legacy := func(name, replacement string) *cobra.Command {
		parent := &cobra.Command{
			Use: name, Hidden: true,
			Short: i18n.Text("Deprecated; use \"maactl device "+replacement+"\"", "已废弃；请使用 \"maactl device "+replacement+"\""),
			Args:  cobra.NoArgs,
			RunE:  func(c *cobra.Command, _ []string) error { return c.Help() },
		}
		parent.AddCommand(&cobra.Command{
			Use: "devices", Short: i18n.Text("Deprecated alias", "已废弃的别名"), Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: \"maactl %s devices\" is deprecated; use \"maactl device %s\"\n", name, replacement)
				return withMaaFramework(global, func() error {
					if name == "adb" {
						return listADB(cmd.OutOrStdout(), global.JSON)
					}
					return listWindows(cmd.OutOrStdout(), global.JSON)
				})
			},
		})
		return parent
	}
	return []*cobra.Command{legacy("adb", "adb"), legacy("win32", "window")}
}

// withMaaFramework initializes the runtime, runs fn, and releases it. The log
// directory is the global --log-dir, so the option reaches the device and
// resource groups as well.
func withMaaFramework(global *GlobalOptions, fn func() error) error {
	libDir, err := maafw.ResolveLibDir(global.LibDir)
	if err != nil {
		return withExitCode(ExitInternal, err)
	}
	if err := maafw.Init(libDir, global.LogDir); err != nil {
		return withExitCode(ExitInternal, fmt.Errorf("initialize MaaFramework from %s: %w", libDir, err))
	}
	defer func() { _ = maa.Release() }()
	return fn()
}

// listADB writes the ADB devices MaaToolkit found to out, as JSON or as a
// plain list.
func listADB(out io.Writer, jsonOutput bool) error {
	devices, err := maa.FindAdbDevices()
	if err != nil {
		return withExitCode(ExitController, fmt.Errorf("find ADB devices: %w", err))
	}
	if jsonOutput {
		rows := make([]adbDeviceOutput, 0, len(devices))
		for _, d := range devices {
			rows = append(rows, adbDeviceOutput{Name: d.Name, AdbPath: d.AdbPath, Address: d.Address, ScreencapMethod: d.ScreencapMethod.String(), InputMethod: d.InputMethod.String(), Config: d.Config})
		}
		return output.JSON(out, rows)
	}
	if len(devices) == 0 {
		fmt.Fprintln(out, "No ADB devices found.")
		return nil
	}
	fmt.Fprintf(out, "Found %d ADB device(s):\n", len(devices))
	for i, d := range devices {
		fmt.Fprintf(out, "[%d] %s\n", i+1, output.Value(d.Name))
		fmt.Fprintf(out, "    address: %s\n", output.Value(d.Address))
		fmt.Fprintf(out, "    adb: %s\n", output.Value(d.AdbPath))
		fmt.Fprintf(out, "    screencap: %s\n", output.Value(d.ScreencapMethod.String()))
		fmt.Fprintf(out, "    input: %s\n", output.Value(d.InputMethod.String()))
	}
	return nil
}

// listWindows writes the desktop windows MaaToolkit found to out, as JSON or as
// a plain list.
func listWindows(out io.Writer, jsonOutput bool) error {
	windows, err := maa.FindDesktopWindows()
	if err != nil {
		return withExitCode(ExitController, fmt.Errorf("find desktop windows: %w", err))
	}
	if jsonOutput {
		rows := make([]desktopWindowOutput, 0, len(windows))
		for _, w := range windows {
			rows = append(rows, desktopWindowOutput{Handle: windowHandleString(uintptr(w.Handle)), ClassName: w.ClassName, WindowName: w.WindowName})
		}
		return output.JSON(out, rows)
	}
	if len(windows) == 0 {
		fmt.Fprintln(out, "No desktop windows found.")
		return nil
	}
	fmt.Fprintf(out, "Found %d desktop window(s):\n", len(windows))
	for i, w := range windows {
		fmt.Fprintf(out, "[%d] %s\n", i+1, output.Value(w.WindowName))
		fmt.Fprintf(out, "    class: %s\n", output.Value(w.ClassName))
		fmt.Fprintf(out, "    handle: %s\n", windowHandleString(uintptr(w.Handle)))
	}
	return nil
}
