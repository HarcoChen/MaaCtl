package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"maactl/internal/clientconfig"
	"maactl/internal/i18n"
	"maactl/internal/output"

	"github.com/spf13/cobra"
)

// newConfigCommand exposes the client configuration maactl reads for defaults.
// maactl never writes the file: it is owned by the user's GUI client.
func newConfigCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: i18n.Text("Inspect the client configuration", "查看客户端配置"),
		Long: i18n.Text(`Reads the client configuration file (maa_pi_config.json) that remembers the
user's controller, resource, device, and option choices. maactl only reads it.

The file is searched at --config, then <PI directory>/config/maa_pi_config.json,
then ./config/maa_pi_config.json.`, `读取客户端配置文件（maa_pi_config.json）——它记录了用户的控制器、资源、设备
和配置项取值。maactl 只读取，不写回。

查找顺序：--config，其次 <PI 目录>/config/maa_pi_config.json，再次
./config/maa_pi_config.json。`),
		Example: `  maactl config path -f D:\01_Projects\github\MaaMio
  maactl config show -f D:\01_Projects\github\MaaMio`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(&cobra.Command{
		Use: "path", Short: i18n.Text("Print the configuration file path", "显示配置文件路径"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, path, err := global.LoadConfig(nil)
			if err != nil {
				return err
			}
			if global.JSON {
				return output.JSON(cmd.OutOrStdout(), map[string]any{"path": path, "exists": fileExists(path)})
			}
			if path == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "No client configuration path; pass --config <path>")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "show", Short: i18n.Text("Show the effective client configuration", "显示生效的客户端配置"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			project, err := global.LoadProject()
			if err != nil {
				// A missing ProjectInterface is fine for inspecting the config
				// itself, but an unreadable one is not.
				if !strings.Contains(err.Error(), "no such file") {
					return err
				}
			}
			config, path, err := global.LoadConfig(project)
			if err != nil {
				return err
			}
			return showConfig(cmd.OutOrStdout(), global, config, path)
		},
	})
	return cmd
}

// configOutput is the JSON shape of `config show`.
type configOutput struct {
	Path       string             `json:"path"`
	Exists     bool               `json:"exists"`
	Controller string             `json:"controller,omitempty"`
	Resource   string             `json:"resource,omitempty"`
	Adb        clientconfig.Adb   `json:"adb,omitempty"`
	Win32      clientconfig.Win32 `json:"win32,omitempty"`
	Option     map[string]any     `json:"option,omitempty"`
	Tasks      []configTaskView   `json:"tasks,omitempty"`
}

// configTaskView is one task entry with password-like values masked.
type configTaskView struct {
	Name    string         `json:"name"`
	Enabled *bool          `json:"enabled,omitempty"`
	Option  map[string]any `json:"option,omitempty"`
}

func showConfig(out io.Writer, global *GlobalOptions, config *clientconfig.Config, path string) error {
	view := configOutput{
		Path:       path,
		Exists:     path != "" && fileExists(path),
		Controller: config.DefaultController(),
		Resource:   config.DefaultResource(),
		Adb:        config.Adb,
		Win32:      config.Win32,
		Option:     maskOptionValues(config.Option),
	}
	for _, task := range config.Task {
		view.Tasks = append(view.Tasks, configTaskView{Name: task.Name, Enabled: task.Enabled, Option: maskOptionValues(task.Option)})
	}
	if global.JSON {
		return output.JSON(out, view)
	}
	if path == "" {
		fmt.Fprintln(out, "path: (none)")
	} else {
		fmt.Fprintf(out, "path: %s (exists: %t)\n", path, view.Exists)
	}
	if view.Controller != "" {
		fmt.Fprintf(out, "controller: %s\n", view.Controller)
	}
	if view.Resource != "" {
		fmt.Fprintf(out, "resource: %s\n", view.Resource)
	}
	if view.Adb.Address != "" || view.Adb.AdbPath != "" {
		fmt.Fprintf(out, "adb: address=%s adb_path=%s screencap=%s input=%s\n",
			output.Value(view.Adb.Address), output.Value(view.Adb.AdbPath), output.Value(view.Adb.Screencap), output.Value(view.Adb.Input))
	}
	for _, key := range sortedKeys(view.Option) {
		fmt.Fprintf(out, "option.%s = %v\n", key, view.Option[key])
	}
	for _, task := range view.Tasks {
		enabled := "-"
		if task.Enabled != nil {
			enabled = fmt.Sprint(*task.Enabled)
		}
		fmt.Fprintf(out, "task %s: enabled=%s options=%d\n", task.Name, enabled, len(task.Option))
	}
	return nil
}

// maskOptionValues hides password-looking values. The client config stores
// secrets as env references or encrypted blobs; anything that is not an object
// with an `env` key could be plaintext, so it is not echoed verbatim.
func maskOptionValues(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = maskValue(value)
	}
	return out
}

func maskValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		if env, ok := v["env"].(string); ok {
			return map[string]any{"env": env}
		}
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = maskValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = maskValue(item)
		}
		return out
	default:
		return value
	}
}

// fileExists reports whether path points at an existing file.
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
