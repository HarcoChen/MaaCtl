package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"maactl/internal/i18n"
	"maactl/internal/output"
	"maactl/internal/pi"

	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/spf13/cobra"
)

// newResourceCommand builds the `resource` group: it loads MaaFramework
// resources without creating a controller, so it also works on packaged
// artifacts and on explicit directories.
func newResourceCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource",
		Short: i18n.Text("Load and inspect MaaFramework resources", "加载并检查 MaaFramework 资源"),
		Long: i18n.Text(`Loads a resource (from a PI resource entry, or from explicit directories) and
reports what MaaFramework found. No controller is created and no Pipeline runs.`, `加载资源（来自 PI 资源条目，或显式目录），并报告 MaaFramework 的加载结果。
不会创建控制器，也不会运行 Pipeline。`),
		Example: `  maactl resource inspect -f D:\01_Projects\github\MaaMio
  maactl resource nodes -r base -f D:\01_Projects\github\MaaMio
  maactl resource hash --path D:\projects\pkg\resource --verify`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	base := resourceFlags{}
	cmd.PersistentFlags().StringVarP(&base.resource, "resource", "r", "", i18n.Text("PI resource name (default: the first resource)", "PI 资源名称（默认：第一个资源）"))
	cmd.PersistentFlags().StringArrayVar(&base.paths, "path", nil, i18n.Text("resource root directory, repeatable; alternative to --resource", "资源根目录，可重复；与 --resource 二选一"))
	cmd.PersistentFlags().StringArrayVar(&base.overlays, "overlay", nil, i18n.Text("resource root loaded after the base resource, repeatable", "在基础资源之后加载的资源根目录，可重复"))

	cmd.AddCommand(
		newResourceActionCommand(global, &base, "inspect", i18n.Text("Show loaded resource metadata", "显示已加载资源的元数据"), false, false),
		newResourceActionCommand(global, &base, "nodes", i18n.Text("List Pipeline nodes", "列出 Pipeline 节点"), true, false),
		newResourceActionCommand(global, &base, "hash", i18n.Text("Print or verify the resource hash", "打印或校验资源 hash"), false, true),
	)
	return cmd
}

// resourceFlags are shared by every `resource` subcommand.
type resourceFlags struct {
	resource string
	paths    []string
	overlays []string
}

func newResourceActionCommand(global *GlobalOptions, base *resourceFlags, use, short string, nodes, hash bool) *cobra.Command {
	var verify bool
	cmd := &cobra.Command{
		Use: use, Short: short, Args: cobra.NoArgs,
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

func outputResourceNodes(out io.Writer, global *GlobalOptions, res *maa.Resource) error {
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

func outputResourceHash(out io.Writer, global *GlobalOptions, res *maa.Resource, plan *resourcePlan, verify bool) error {
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

func outputResourceInspect(out io.Writer, global *GlobalOptions, res *maa.Resource, plan *resourcePlan, loadedPaths []string) error {
	hash, _ := res.GetHash()
	nodes, _ := res.GetNodeList()
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
