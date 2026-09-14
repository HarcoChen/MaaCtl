package cli

import (
	"fmt"
	"path/filepath"

	"maactl/internal/help"
	"maactl/internal/i18n"
	"maactl/internal/maafw"
	"maactl/internal/output"
	"maactl/internal/pi"

	maa "github.com/MaaXYZ/maa-framework-go/v3"
	"github.com/spf13/cobra"
)

func newResourceCommand(global *Options) *cobra.Command {
	var resourceName string
	var inspect, nodes bool
	cmd := &cobra.Command{
		Use: "resource", Short: i18n.Text("Load and inspect PI resources", "加载并检查 PI 资源"),
		Long: i18n.Text(`resource inspect shows loaded metadata; resource nodes lists Pipeline nodes
available in the loaded resource. The shortcut flags -i and -n are equivalent
to the two subcommands.`, `resource inspect 显示已加载资源的元数据；resource nodes 列出资源中
可用的 Pipeline 节点。快捷选项 -i 和 -n 分别等价于这两个子命令。`),
		Example: `  maactl resource inspect -f D:\MaaMio
  maactl resource nodes -r base -f D:\MaaMio --json`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !inspect && !nodes {
				return c.Help()
			}
			if inspect && nodes {
				return fmt.Errorf("choose only one resource shortcut")
			}
			project, res, err := loadSelectedResource(global, resourceName)
			if err != nil {
				return err
			}
			return inspectResource(global, project, res, nodes)
		},
	}
	add := func(use string, short, long string, nodes bool) {
		cmd.AddCommand(&cobra.Command{Use: use, Short: short, Long: long, Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
			project, res, err := loadSelectedResource(global, resourceName)
			if err != nil {
				return err
			}
			return inspectResource(global, project, res, nodes)
		}})
	}
	add("inspect", i18n.Text("Show loaded resource metadata", "显示已加载资源的元数据"), i18n.Text("Load the selected resource and show its paths, hash, and node count.", "加载所选资源并显示其路径、hash 和节点数量。"), false)
	add("nodes", i18n.Text("List loaded Pipeline nodes", "列出已加载的 Pipeline 节点"), i18n.Text("Load the selected resource and list all Pipeline nodes it provides.", "加载所选资源并列出其中所有 Pipeline 节点。"), true)
	cmd.AddCommand(plannedResourceCommand("hash", i18n.Text("Print or verify the loaded resource hash", "打印或校验已加载资源的 hash")))
	cmd.PersistentFlags().StringVarP(&resourceName, "resource", "r", "", i18n.Text("PI resource name (default: first resource)", "PI 资源名称（默认：第一个资源）"))
	cmd.Flags().BoolVarP(&inspect, "inspect", "i", false, i18n.Text(`shortcut for "maactl resource inspect"`, `等价于 "maactl resource inspect"`))
	cmd.Flags().BoolVarP(&nodes, "nodes", "n", false, i18n.Text(`shortcut for "maactl resource nodes"`, `等价于 "maactl resource nodes"`))
	return cmd
}

// loadSelectedResource loads the ProjectInterface and resolves the requested
// resource against its controller (the only controller when unambiguous).
func loadSelectedResource(global *Options, name string) (*pi.Loaded, *pi.Resource, error) {
	project, err := pi.Load(global.InterfacePath)
	if err != nil {
		return nil, nil, err
	}
	ctrl := &pi.Controller{}
	if len(project.Controller) == 1 {
		ctrl = &project.Controller[0]
	}
	res, err := project.FindResource(name, ctrl)
	if err != nil {
		return nil, nil, err
	}
	return project, res, nil
}

func plannedResourceCommand(use, short string) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short, Long: i18n.Text("This command is part of the published CLI contract but is not implemented yet. It returns an error when invoked.", "该命令属于已公布的 CLI 契约，但尚未实现，调用时会返回错误。"), Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return fmt.Errorf("resource %s is not implemented yet", use) }}
	help.MarkCommandPlanned(cmd)
	return cmd
}

func inspectResource(global *Options, project *pi.Loaded, spec *pi.Resource, listNodes bool) error {
	libDir, err := maafw.ResolveLibDir(global.LibDir)
	if err != nil {
		return err
	}
	if err := maafw.Init(libDir); err != nil {
		return err
	}
	defer func() { _ = maa.Release() }()
	res := maa.NewResource()
	if res == nil {
		return fmt.Errorf("create Maa resource")
	}
	defer res.Destroy()
	paths := make([]string, 0, len(spec.Path))
	for _, path := range spec.Path {
		full := filepath.Join(project.Dir, path)
		if !res.PostBundle(full).Wait().Success() {
			return fmt.Errorf("load resource %s", full)
		}
		paths = append(paths, full)
	}
	if listNodes {
		nodes, ok := res.GetNodeList()
		if !ok {
			return fmt.Errorf("read resource nodes")
		}
		if global.JSON {
			return output.Stdout(nodes)
		}
		for _, node := range nodes {
			fmt.Println(node)
		}
		return nil
	}
	hash, _ := res.GetHash()
	nodes, _ := res.GetNodeList()
	result := map[string]any{"name": spec.Name, "paths": paths, "hash": hash, "expected_hash": spec.Hash, "node_count": len(nodes)}
	if global.JSON {
		return output.Stdout(result)
	}
	fmt.Printf("resource: %s\nhash: %s\nnodes: %d\n", spec.Name, hash, len(nodes))
	return nil
}
