package cli

import (
	"fmt"
	"strconv"

	"maactl/internal/i18n"
	"maactl/internal/maafw"
	"maactl/internal/output"

	maa "github.com/MaaXYZ/maa-framework-go/v3"
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

func newADBCommand(global *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use: "adb", Short: i18n.Text("Inspect ADB devices", "查看 ADB 设备"),
		Long: i18n.Text("List ADB devices discovered by MaaToolkit.\n\nUse \"maactl adb devices\" to see addresses and recommended connection methods.", "列出 MaaToolkit 发现的 ADB 设备。\n\n使用 \"maactl adb devices\" 查看设备地址和建议的连接方式。"),
		Example: `  maactl adb devices
  maactl adb devices --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(newDevicesCommand(global, "adb"))
	return cmd
}

func newWin32Command(global *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use: "win32", Short: i18n.Text("Inspect Win32 desktop windows", "查看 Win32 桌面窗口"),
		Long: i18n.Text("List Win32 desktop windows discovered by MaaToolkit.\n\nUse \"maactl win32 devices\" to see window classes and handles.", "列出 MaaToolkit 发现的 Win32 桌面窗口。\n\n使用 \"maactl win32 devices\" 查看窗口类名和句柄。"),
		Example: `  maactl win32 devices
  maactl win32 devices --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(newDevicesCommand(global, "win32"))
	return cmd
}

func newDevicesCommand(global *Options, kind string) *cobra.Command {
	long := i18n.Text("Windows are discovered through MaaToolkit and include the window name, class, and handle.", "窗口由 MaaToolkit 发现，包含窗口名称、类名和句柄。")
	if kind == "adb" {
		long = i18n.Text("Devices are discovered through MaaToolkit and include the address, ADB path,\nand recommended screencap and input methods.", "设备由 MaaToolkit 发现，包含地址、ADB 路径以及建议的截图和输入方式。")
	}
	cmd := &cobra.Command{
		Use: "devices", Short: fmt.Sprintf(i18n.Text("List %s devices", "列出 %s 设备"), kind), Long: long, Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			libDir, err := maafw.ResolveLibDir(global.LibDir)
			if err != nil {
				return err
			}
			if err := maafw.Init(libDir); err != nil {
				return fmt.Errorf("initialize MaaFramework from %s: %w", libDir, err)
			}
			defer func() { _ = maa.Release() }()
			if kind == "adb" {
				return listADB(global.JSON)
			}
			return listWin32(global.JSON)
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
	windows := maa.FindDesktopWindows()
	if jsonOutput {
		out := make([]desktopWindowOutput, 0, len(windows))
		for _, w := range windows {
			out = append(out, desktopWindowOutput{Handle: strconv.FormatUint(uint64(uintptr(w.Handle)), 16), ClassName: w.ClassName, WindowName: w.WindowName})
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
		fmt.Printf("    handle: 0x%s\n", strconv.FormatUint(uint64(uintptr(w.Handle)), 16))
	}
	return nil
}
