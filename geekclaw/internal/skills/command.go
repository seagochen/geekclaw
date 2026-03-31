package skills

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
	"github.com/seagosoft/geekclaw/geekclaw/skills"
)

// deps 保存技能命令的共享依赖项。
type deps struct {
	pluginsDir   string               // 插件目录路径
	installer    *skills.SkillInstaller // 技能安装器
	skillsLoader *skills.SkillsLoader   // 技能加载器
}

// NewSkillsCommand 创建技能管理命令，包含列表、安装、删除、搜索和查看子命令。
func NewSkillsCommand() *cobra.Command {
	var d deps

	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage skills",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := internal.LoadConfig()
			if err != nil {
				return fmt.Errorf("error loading config: %w", err)
			}

			d.pluginsDir = cfg.PluginsPath()
			d.installer = skills.NewSkillInstaller(d.pluginsDir)

			// 获取全局主目录和内置技能目录
			globalDir := internal.GetGeekclawHome()
			globalSkillsDir := filepath.Join(globalDir, "skills")
			builtinSkillsDir := filepath.Join(globalDir, "plugins", "skills")
			d.skillsLoader = skills.NewSkillsLoader(d.pluginsDir, globalSkillsDir, builtinSkillsDir)

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	installerFn := func() (*skills.SkillInstaller, error) {
		if d.installer == nil {
			return nil, fmt.Errorf("skills installer is not initialized")
		}
		return d.installer, nil
	}

	loaderFn := func() (*skills.SkillsLoader, error) {
		if d.skillsLoader == nil {
			return nil, fmt.Errorf("skills loader is not initialized")
		}
		return d.skillsLoader, nil
	}

	pluginsDirFn := func() (string, error) {
		if d.pluginsDir == "" {
			return "", fmt.Errorf("plugins directory is not initialized")
		}
		return d.pluginsDir, nil
	}

	cmd.AddCommand(
		newListCommand(loaderFn),
		newInstallCommand(installerFn),
		newInstallBuiltinCommand(pluginsDirFn),
		newListBuiltinCommand(),
		newRemoveCommand(installerFn),
		newSearchCommand(),
		newShowCommand(loaderFn),
	)

	return cmd
}
