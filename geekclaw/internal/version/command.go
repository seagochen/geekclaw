package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
	"github.com/seagosoft/geekclaw/geekclaw/config"
)

// NewVersionCommand 创建版本信息命令，显示版本号和构建信息。
func NewVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		Short:   "Show version information",
		Run: func(_ *cobra.Command, _ []string) {
			printVersion()
		},
	}

	return cmd
}

// printVersion 输出版本号、构建信息和 Go 版本。
func printVersion() {
	fmt.Printf("%s geekclaw %s\n", internal.Logo, config.FormatVersion())
	build, goVer := config.FormatBuildInfo()
	if build != "" {
		fmt.Printf("  Build: %s\n", build)
	}
	if goVer != "" {
		fmt.Printf("  Go: %s\n", goVer)
	}
}
