package status

import (
	"github.com/spf13/cobra"
)

// NewStatusCommand 创建状态查看命令，显示 geekclaw 的整体运行状态。
func NewStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"s"},
		Short:   "Show geekclaw status",
		Run: func(cmd *cobra.Command, args []string) {
			statusCmd()
		},
	}

	return cmd
}
