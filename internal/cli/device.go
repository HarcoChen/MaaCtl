package cli

import (
	"fmt"
	"strconv"
	"unsafe"

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
// on this machine, which is what the ADB and Win32 selection flags match
// against.
func newDeviceCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "device",
		Short: i18n.Text("Inspect devices and windows", "查看设备与窗口"),
		Long: i18n.Text(`Devices and windows are discovered through MaaToolkit. Use this command to get
the addresses, names, classes, and handles that run accepts.`, `设备与窗口由 MaaToolkit 发现。用本命令获取 run 所需的地址、名称、类名和句柄。`),
		Example: `  maactl device adb
  maactl device win32 --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(newDeviceListCommand(global, "adb"), newDeviceListCommand(global, "win32"))
	return cmd
}

// newDeviceListCommand builds `device adb` / `device win32`.
func newDeviceListCommand(global *GlobalOptions, kind string) *cobra.Command {
	short := i18n.Text("List ADB devices", "列出 ADB 设备")
	long := i18n.Text("Devices are discovered through MaaToolkit and include the address, ADB path,\nand recommended screencap and input methods.", "设备由 MaaToolkit 发现，包含地址、ADB 路径以及建议的截图和输入方式。")
	if kind == "win32" {
		short = i18n.Text("List Win32 desktop windows", "列出 Win32 桌面窗口")
		long = i18n.Text("Windows are discovered through MaaToolkit and include the window name, class, and handle.", "窗口由 MaaToolkit 发现，包含窗口名称、类名和句柄。")
	}
	return &cobra.Command{
		Use: kind, Short: short, Long: long, Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return withMaaFramework(global, func() error {
				if kind == "adb" {
					return listADB(global.JSON)
				}
				return listWin32(global.JSON)
			})
		},
	}
}

// newLegacyDeviceCommands returns the pre-redesign `adb devices` and
// `win32 devices` commands. They keep working for one release so existing
// scripts do not break, and point at their replacement.
func newLegacyDeviceCommands(global *GlobalOptions) []*cobra.Command {
	legacy := func(kind string) *cobra.Command {
		parent := &cobra.Command{
			Use: kind, Hidden: true,
			Short: i18n.Text("Deprecated; use \"maactl device "+kind+"\"", "已废弃；请使用 \"maactl device "+kind+"\""),
			Args:  cobra.NoArgs,
			RunE:  func(c *cobra.Command, _ []string) error { return c.Help() },
		}
		parent.AddCommand(&cobra.Command{
			Use: "devices", Short: i18n.Text("Deprecated alias", "已废弃的别名"), Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: \"maactl %s devices\" is deprecated; use \"maactl device %s\"\n", kind, kind)
				return withMaaFramework(global, func() error {
					if kind == "adb" {
						return listADB(global.JSON)
					}
					return listWin32(global.JSON)
				})
			},
		})
		return parent
	}
	return []*cobra.Command{legacy("adb"), legacy("win32")}
}

// withMaaFramework initializes the runtime, runs fn, and releases it.
func withMaaFramework(global *GlobalOptions, fn func() error) error {
	libDir, err := maafw.ResolveLibDir(global.LibDir)
	if err != nil {
		return withExitCode(ExitInternal, err)
	}
	if err := maafw.Init(libDir, ""); err != nil {
		return withExitCode(ExitInternal, fmt.Errorf("initialize MaaFramework from %s: %w", libDir, err))
	}
	defer func() { _ = maa.Release() }()
	return fn()
}

func listADB(jsonOutput bool) error {
	devices, err := maa.FindAdbDevices()
	if err != nil {
		return withExitCode(ExitController, fmt.Errorf("find ADB devices: %w", err))
	}
	if jsonOutput {
		out := make([]adbDeviceOutput, 0, len(devices))
		for _, d := range devices {
			out = append(out, adbDeviceOutput{Name: d.Name, AdbPath: d.AdbPath, Address: d.Address, ScreencapMethod: d.ScreencapMethod.String(), InputMethod: d.InputMethod.String(), Config: d.Config})
		}
		return output.Stdout(out)
	}
	if len(devices) == 0 {
		fmt.Println("No ADB devices found.")
		return nil
	}
	fmt.Printf("Found %d ADB device(s):\n", len(devices))
	for i, d := range devices {
		fmt.Printf("[%d] %s\n", i+1, output.Value(d.Name))
		fmt.Printf("    address: %s\n", output.Value(d.Address))
		fmt.Printf("    adb: %s\n", output.Value(d.AdbPath))
		fmt.Printf("    screencap: %s\n", output.Value(d.ScreencapMethod.String()))
		fmt.Printf("    input: %s\n", output.Value(d.InputMethod.String()))
	}
	return nil
}

func listWin32(jsonOutput bool) error {
	windows, err := maa.FindDesktopWindows()
	if err != nil {
		return withExitCode(ExitController, fmt.Errorf("find desktop windows: %w", err))
	}
	if jsonOutput {
		out := make([]desktopWindowOutput, 0, len(windows))
		for _, w := range windows {
			out = append(out, desktopWindowOutput{Handle: win32HandleString(w.Handle), ClassName: w.ClassName, WindowName: w.WindowName})
		}
		return output.Stdout(out)
	}
	if len(windows) == 0 {
		fmt.Println("No Win32 windows found.")
		return nil
	}
	fmt.Printf("Found %d Win32 window(s):\n", len(windows))
	for i, w := range windows {
		fmt.Printf("[%d] %s\n", i+1, output.Value(w.WindowName))
		fmt.Printf("    class: %s\n", output.Value(w.ClassName))
		fmt.Printf("    handle: %s\n", win32HandleString(w.Handle))
	}
	return nil
}

// win32HandleString renders a window handle the way --win32-handle accepts it,
// so the value shown by "maactl device win32" can be passed back verbatim.
func win32HandleString(handle unsafe.Pointer) string {
	return "0x" + strconv.FormatUint(uint64(uintptr(handle)), 16)
}
