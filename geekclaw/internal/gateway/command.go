package gateway

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/utils"
)

// NewGatewayCommand 创建网关启动命令，支持调试模式和日志截断控制选项。
func NewGatewayCommand() *cobra.Command {
	var debug bool
	var noTruncate bool

	cmd := &cobra.Command{
		Use:     "gateway",
		Aliases: []string{"g"},
		Short:   "Start geekclaw gateway",
		Args:    cobra.NoArgs,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if noTruncate && !debug {
				return fmt.Errorf("the --no-truncate option can only be used in conjunction with --debug (-d)")
			}

			if noTruncate {
				utils.SetDisableTruncation(true)
				logger.Info("String truncation is globally disabled via 'no-truncate' flag")
			}

			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			return gatewayCmd(debug)
		},
	}

	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")
	cmd.Flags().BoolVarP(&noTruncate, "no-truncate", "T", false, "Disable string truncation in debug logs")

	return cmd
}
