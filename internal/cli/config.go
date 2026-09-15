package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"maactl/internal/clientconfig"
	"maactl/internal/i18n"
	"maactl/internal/output"
	"maactl/internal/pi"

	"github.com/spf13/cobra"
)

// newConfigCommand exposes the client configuration maactl reads for defaults.
// maactl never writes the file: it is owned by the user's GUI client.
func newConfigCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Aliases: []string{"cfg"},
		Short:   i18n.Text("Show the client configuration", "查看客户端配置"),
		Long: i18n.Text(`Reads the client configuration file (maa_pi_config.json) that remembers the
user's controller, resource, device, and option choices. maactl only reads it.

Search order: -cfg/--config, then <PI>/config/maa_pi_config.json, then
./config/maa_pi_config.json.`, `读取客户端配置文件（maa_pi_config.json）——它记录用户的控制器、资源、设备和
配置项取值。maactl 只读取，不写回。

查找顺序：-cfg/--config，其次 <PI 目录>/config/maa_pi_config.json，
再次 ./config/maa_pi_config.json。`),
		Example: `  maactl cfg p -if D:\MaaMio
  maactl cfg s -if D:\MaaMio`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(&cobra.Command{
		Use: "path", Aliases: []string{"p"}, Short: i18n.Text("Print the file path", "显示配置文件路径"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, path, err := global.LoadConfig(nil)
			if err != nil {
				return err
			}
			if global.JSON {
				return output.JSON(cmd.OutOrStdout(), map[string]any{"path": path, "exists": fileExists(path)})
			}
			if path == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "No client configuration path; pass -cfg <path>")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "show", Aliases: []string{"s"}, Short: i18n.Text("Show the effective configuration", "显示生效的配置"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			project, err := global.LoadProject()
			if err != nil {
				// A missing ProjectInterface is fine for inspecting the config
				// itself, but an unreadable one is not. The check goes through
				// errors.Is so it holds on every platform, whatever the
				// operating system spells the missing-file error as.
				if !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
			config, path, err := global.LoadConfig(project)
			if err != nil {
				return err
			}
			return showConfig(cmd.OutOrStdout(), global, config, path, project)
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

// configTaskView is one task entry with its password values masked.
type configTaskView struct {
	Name    string         `json:"name"`
	Enabled *bool          `json:"enabled,omitempty"`
	Option  map[string]any `json:"option,omitempty"`
}

// showConfig renders the effective configuration. project may be nil when no
// ProjectInterface is available; it is what tells a declared password field
// from an ordinary option value.
func showConfig(out io.Writer, global *GlobalOptions, config *clientconfig.Config, path string, project *pi.Loaded) error {
	view := configOutput{
		Path:       path,
		Exists:     path != "" && fileExists(path),
		Controller: config.DefaultController(),
		Resource:   config.DefaultResource(),
		Adb:        config.Adb,
		Win32:      config.Win32,
		Option:     maskOptionValues(config.Option, project),
	}
	for _, task := range config.Task {
		view.Tasks = append(view.Tasks, configTaskView{Name: task.Name, Enabled: task.Enabled, Option: maskOptionValues(task.Option, project)})
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

// maskOptionValues hides the option values the ProjectInterface declares as
// password inputs: the client config is a supported place to keep them, so they
// are never echoed back. Every other value stays readable, because `config
// show` exists to explain the choices in effect.
//
// Without a ProjectInterface, or for an option name it does not declare,
// nothing can be shown to be an ordinary value, so everything is hidden.
func maskOptionValues(values map[string]any, project *pi.Loaded) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		secrets, declared := passwordFields(project, key)
		out[key] = maskValue(value, secrets, !declared, true)
	}
	return out
}

// passwordFields names the input fields the ProjectInterface declares as
// passwords for one option. declared is false when the ProjectInterface is
// missing or does not define the option.
func passwordFields(project *pi.Loaded, name string) (secrets map[string]bool, declared bool) {
	if project == nil {
		return nil, false
	}
	option, ok := project.Option.Get(name)
	if !ok {
		return nil, false
	}
	secrets = map[string]bool{}
	for _, field := range option.Inputs {
		if field.Password {
			secrets[field.Name] = true
		}
	}
	return secrets, true
}

// maskValue replaces the password fields of one option value with ******. An
// `env` reference survives as it is: the variable name is not the secret and it
// says where the value comes from. unknown marks a value whose declaration is
// not known, and top marks the option's whole value, which cannot be attributed
// to a single field; both are hidden rather than echoed.
func maskValue(value any, secrets map[string]bool, unknown, top bool) any {
	switch v := value.(type) {
	case map[string]any:
		if env, ok := v["env"].(string); ok {
			return map[string]any{"env": env}
		}
		out := make(map[string]any, len(v))
		for key, item := range v {
			if secrets[key] {
				out[key] = "******"
				continue
			}
			out[key] = maskValue(item, secrets, unknown, false)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = maskValue(item, secrets, unknown, false)
		}
		return out
	default:
		if unknown || (top && len(secrets) > 0) {
			return "******"
		}
		return v
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
