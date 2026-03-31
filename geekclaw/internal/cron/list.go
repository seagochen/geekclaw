package cron

import "github.com/spf13/cobra"

// newListCommand 创建列出所有定时任务的子命令。
func newListCommand(storePath func() string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all scheduled jobs",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cronListCmd(storePath())
			return nil
		},
	}

	return cmd
}
