package cron

import "github.com/spf13/cobra"

// newEnableCommand 创建启用定时任务子命令。
func newEnableCommand(storePath func() string) *cobra.Command {
	return &cobra.Command{
		Use:     "enable",
		Short:   "Enable a job",
		Args:    cobra.ExactArgs(1),
		Example: `geekclaw cron enable 1`,
		RunE: func(_ *cobra.Command, args []string) error {
			cronSetJobEnabled(storePath(), args[0], true)
			return nil
		},
	}
}
