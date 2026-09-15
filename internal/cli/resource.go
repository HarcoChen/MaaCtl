package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"maactl/internal/i18n"
	"maactl/internal/output"
	"maactl/internal/pi"
	"maactl/internal/table"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/spf13/cobra"
)

// newResourceCommand builds the `resource` group: everything about resources in
// one place — what the ProjectInterface declares, and what MaaFramework loads.
func newResourceCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resource",
		Aliases: []string{"res"},
		Short:   i18n.Text("Browse resources", "浏览资源"),
		Long: i18n.Text(`Reads resource information from two angles: "list" reports what the
ProjectInterface declares, while "inspect", "nodes", and "hash" load MaaFramework
resources (from a PI resource entry or from --path) without creating a controller.`, `从两个角度看资源："list" 报告 ProjectInterface 的声明，"inspect"/"nodes"/"hash"
则加载 MaaFramework 资源（来自 PI 资源条目或 --pa/--path），但不会创建控制器。`),
		Example: `  maactl resource l -if D:\MaaMio
  maactl resource i -r base -if D:\MaaMio
  maactl resource h -pa D:\pkg\resource -vf`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	base := resourceFlags{}
	cmd.PersistentFlags().StringVarP(&base.resource, "resource", "r", "", i18n.Text("PI resource name (default: the first one)", "PI 资源名称（默认：第一个）"))
	cmd.PersistentFlags().StringArrayVar(&base.paths, "path", nil, i18n.Text("resource root, repeatable; alternative to --resource", "资源根目录，可重复；与 --resource 二选一"))
	cmd.PersistentFlags().StringArrayVar(&base.overlays, "overlay", nil, i18n.Text("resource root loaded after the base ones, repeatable", "在基础资源之后加载的资源根目录，可重复"))

	cmd.AddCommand(
		newResourceListCommand(global),
		newResourceActionCommand(global, &base, "inspect", i18n.Text("Load a resource and show its metadata", "加载资源并显示元数据"), false, false),
		newResourceActionCommand(global, &base, "nodes", i18n.Text("List Pipeline nodes", "列出 Pipeline 节点"), true, false),
		newResourceActionCommand(global, &base, "hash", i18n.Text("Print or verify the resource hash", "打印或校验资源 hash"), false, true),
	)
	return cmd
}

// resourceFlags are shared by the loading resource subcommands.
type resourceFlags struct {
	resource string
	paths    []string
	overlays []string
}

func newResourceListCommand(global *GlobalOptions) *cobra.Command {
	var controllerFilter string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   i18n.Text("List the declared resources", "列出声明的资源"),
		Long: i18n.Text(`Reports the resource entries declared by the ProjectInterface: their paths, hash,
controller restrictions, and option count. Nothing is loaded.`, `报告 ProjectInterface 声明的资源条目：路径、hash、控制器限制与选项数量。
不会加载任何资源。`),
		Example: `  maactl resource l -if D:\MaaMio -c Android`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, err := loadPI(global)
			if err != nil {
				return err
			}
			return listResources(cmd.OutOrStdout(), ctx, controllerFilter)
		},
	}
	cmd.Flags().StringVarP(&controllerFilter, "controller", "c", "", i18n.Text("compatibility against this controller", "按该控制器判断兼容性"))
	return cmd
}

// resourceRow is one row of `resource list`.
type resourceRow struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Path        []string `json:"path"`
	Hash        string   `json:"hash,omitempty"`
	Controllers []string `json:"controllers,omitempty"`
	Options     int      `json:"options"`
	Compatible  bool     `json:"compatible"`
}

func listResources(out io.Writer, ctx *piContext, controllerFilter string) error {
	if controllerFilter != "" {
		if _, err := ctx.project.FindController(controllerFilter); err != nil {
			return withExitCode(ExitUsage, err)
		}
	}
	rows := make([]resourceRow, 0, len(ctx.project.Resource))
	for i := range ctx.project.Resource {
		res := &ctx.project.Resource[i]
		rows = append(rows, resourceRow{
			Name:        res.Name,
			Label:       ctx.label(res.Label, res.Name),
			Path:        res.Path,
			Hash:        res.Hash,
			Controllers: res.Controller,
			Options:     len(res.Option),
			Compatible:  controllerFilter == "" || pi.Compatible(res.Controller, controllerFilter),
		})
	}
	if ctx.global.JSON {
		return output.JSON(out, rows)
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		compatible := "-"
		if controllerFilter != "" {
			compatible = yesNo(row.Compatible)
		}
		tableRows = append(tableRows, []string{row.Name, row.Label, pi.Join(row.Path), output.Value(row.Hash), formatList(row.Controllers), compatible})
	}
	return table.Print(out, headers(
		"name", "名称",
		"label", "显示名称",
		"path", "资源路径",
		"hash", "hash",
		"controllers", "限定控制器",
		"compatible", "兼容",
	), tableRows)
}

func newResourceActionCommand(global *GlobalOptions, base *resourceFlags, use, short string, nodes, hash bool) *cobra.Command {
	var verify bool
	aliases := map[string][]string{
		"inspect": {"i"},
		"nodes":   {"n"},
		"hash":    {"h"},
	}[use]
	cmd := &cobra.Command{
		Use: use, Aliases: aliases, Short: short, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withMaaFramework(global, func() error {
				plan, err := loadResourcePlan(global, base)
				if err != nil {
					return err
				}
				res, loadedPaths, err := loadResourceBundles(plan)
				if err != nil {
					return err
				}
				defer res.Destroy()
				switch {
				case nodes:
					return outputResourceNodes(cmd.OutOrStdout(), global, res)
				case hash:
					return outputResourceHash(cmd.OutOrStdout(), global, res, plan, verify)
				default:
					return outputResourceInspect(cmd.OutOrStdout(), global, res, plan, loadedPaths)
				}
			})
		},
	}
	if hash {
		cmd.Flags().BoolVar(&verify, "verify", false, i18n.Text("fail when the hash does not match resource.hash", "hash 与 resource.hash 不一致时失败"))
	}
	return cmd
}

// resourcePlan is what a `resource` subcommand needs before loading.
type resourcePlan struct {
	project  *pi.Loaded
	resource *pi.Resource
	// paths are absolute base resource roots, in load order.
	paths []string
	// overlays are absolute roots loaded after the base paths.
	overlays []string
	// hash is the expected hash from resource.hash, when a PI resource was used.
	hash string
}

// loadResourcePlan resolves --resource/--path/--overlay into absolute paths.
func loadResourcePlan(global *GlobalOptions, base *resourceFlags) (*resourcePlan, error) {
	name := base.resource
	if name != "" && len(base.paths) > 0 {
		return nil, exitErrorf(ExitUsage, "--resource and --path cannot be used together")
	}
	plan := &resourcePlan{}
	if name != "" || len(base.paths) == 0 {
		project, err := global.LoadProject()
		if err != nil {
			return nil, err
		}
		plan.project = project
		resource, err := project.FindResource(name, nil)
		if err != nil {
			return nil, withExitCode(ExitUsage, err)
		}
		plan.resource = resource
		plan.hash = resource.Hash
		for _, path := range resource.Path {
			plan.paths = append(plan.paths, filepath.Join(project.Dir, filepath.FromSlash(path)))
		}
	} else {
		for _, path := range base.paths {
			abs, err := filepath.Abs(path)
			if err != nil {
				return nil, withExitCode(ExitUsage, err)
			}
			plan.paths = append(plan.paths, abs)
		}
	}
	for _, overlay := range base.overlays {
		abs, err := filepath.Abs(overlay)
		if err != nil {
			return nil, withExitCode(ExitUsage, err)
		}
		plan.overlays = append(plan.overlays, abs)
	}
	return plan, nil
}

// loadResourceBundles posts every path to MaaFramework and returns the loaded
// resource plus the absolute paths that were loaded.
func loadResourceBundles(plan *resourcePlan) (*maa.Resource, []string, error) {
	res, err := maa.NewResource()
	if err != nil {
		return nil, nil, withExitCode(ExitResource, fmt.Errorf("create Maa resource: %w", err))
	}
	all := append(append([]string(nil), plan.paths...), plan.overlays...)
	for _, path := range all {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			res.Destroy()
			return nil, nil, exitErrorf(ExitResource, "resource path %s is not a directory", path)
		}
		if job := res.PostBundle(path).Wait(); !job.Success() {
			res.Destroy()
			return nil, nil, exitErrorf(ExitResource, "load resource %s: %s", path, job.Status())
		}
	}
	return res, all, nil
}

// resourceReader is the read-only part of a loaded MaaFramework resource that
// the resource queries consume. *maa.Resource satisfies it; the interface keeps
// their error handling testable without loading MaaFramework.
type resourceReader interface {
	GetHash() (string, error)
	GetNodeList() ([]string, error)
}

func outputResourceNodes(out io.Writer, global *GlobalOptions, res resourceReader) error {
	nodes, err := res.GetNodeList()
	if err != nil {
		return withExitCode(ExitResource, fmt.Errorf("read resource nodes: %w", err))
	}
	if global.JSON {
		return output.JSON(out, nodes)
	}
	for _, node := range nodes {
		fmt.Fprintln(out, node)
	}
	return nil
}

func outputResourceHash(out io.Writer, global *GlobalOptions, res resourceReader, plan *resourcePlan, verify bool) error {
	hash, err := res.GetHash()
	if err != nil {
		return withExitCode(ExitResource, fmt.Errorf("read resource hash: %w", err))
	}
	result := map[string]any{"hash": hash, "expected_hash": plan.hash, "paths": plan.paths}
	if global.JSON {
		if err := output.JSON(out, result); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "hash: %s\n", hash)
		if plan.hash != "" {
			fmt.Fprintf(out, "expected: %s\n", plan.hash)
		}
	}
	if !verify {
		return nil
	}
	switch {
	case plan.hash == "":
		fmt.Fprintln(out, "warning: the selected resource declares no resource.hash; nothing to verify")
		return nil
	case hash != plan.hash:
		return exitErrorf(ExitResource, "resource hash mismatch: got %s, expected %s", hash, plan.hash)
	default:
		fmt.Fprintln(out, "resource hash matches")
		return nil
	}
}

func outputResourceInspect(out io.Writer, global *GlobalOptions, res resourceReader, plan *resourcePlan, loadedPaths []string) error {
	hash, err := res.GetHash()
	if err != nil {
		return withExitCode(ExitResource, fmt.Errorf("read resource hash: %w", err))
	}
	nodes, err := res.GetNodeList()
	if err != nil {
		return withExitCode(ExitResource, fmt.Errorf("read resource nodes: %w", err))
	}
	result := map[string]any{
		"paths":         loadedPaths,
		"hash":          hash,
		"expected_hash": plan.hash,
		"node_count":    len(nodes),
	}
	if plan.resource != nil {
		result["resource"] = plan.resource.Name
	}
	if global.JSON {
		return output.JSON(out, result)
	}
	if plan.resource != nil {
		fmt.Fprintf(out, "resource: %s\n", plan.resource.Name)
	}
	for _, path := range loadedPaths {
		fmt.Fprintf(out, "path: %s\n", path)
	}
	fmt.Fprintf(out, "hash: %s\n", hash)
	if plan.hash != "" {
		status := "match"
		if hash != plan.hash {
			status = "MISMATCH"
		}
		fmt.Fprintf(out, "expected hash: %s (%s)\n", plan.hash, status)
	}
	fmt.Fprintf(out, "nodes: %d\n", len(nodes))
	return nil
}
