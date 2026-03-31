package cron

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
)

// NewCronCommand 创建定时任务管理命令，包含列表、添加、删除、启用和禁用子命令。
func NewCronCommand() *cobra.Command {
	var storePath string

	cmd := &cobra.Command{
		Use:     "cron",
		Aliases: []string{"c"},
		Short:   "Manage scheduled tasks",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		// 在执行时解析 storePath，以反映当前配置并在所有子命令间共享。
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := internal.LoadConfig()
			if err != nil {
				return fmt.Errorf("error loading config: %w", err)
			}
			storePath = filepath.Join(cfg.LogsPath(), "jobs.json")
			return nil
		},
	}

	cmd.AddCommand(
		newListCommand(func() string { return storePath }),
		newAddCommand(func() string { return storePath }),
		newRemoveCommand(func() string { return storePath }),
		newEnableCommand(func() string { return storePath }),
		newDisableCommand(func() string { return storePath }),
	)

	return cmd
}
