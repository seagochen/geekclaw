package skills

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
	"github.com/seagosoft/geekclaw/geekclaw/skills"
)

const skillsSearchMaxResults = 20 // 搜索结果最大数量

// newSearchCommand 创建搜索可用技能的子命令。
func newSearchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "search [query]",
		Short:   "Search available skills",
		Example: `geekclaw skills search weather`,
		RunE: func(_ *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			skillsSearchCmd(query)
			return nil
		},
	}

	return cmd
}

// skillsSearchCmd 从所有注册中心搜索可用技能并显示结果。
func skillsSearchCmd(query string) {
	fmt.Println("Searching for available skills...")

	cfg, err := internal.LoadConfig()
	if err != nil {
		fmt.Printf("✗ Failed to load config: %v\n", err)
		return
	}

	registryMgr := skills.NewRegistryManagerFromConfig(skills.RegistryConfig{
		MaxConcurrentSearches: cfg.Tools.Skills.MaxConcurrentSearches,
		ClawHub:               skills.ClawHubConfig(cfg.Tools.Skills.Registries.ClawHub),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := registryMgr.SearchAll(ctx, query, skillsSearchMaxResults)
	if err != nil {
		fmt.Printf("✗ Failed to fetch skills list: %v\n", err)
		return
	}

	if len(results) == 0 {
		fmt.Println("No skills available.")
		return
	}

	fmt.Printf("\nAvailable Skills (%d):\n", len(results))
	fmt.Println("--------------------")
	for _, result := range results {
		fmt.Printf("  📦 %s\n", result.DisplayName)
		fmt.Printf("     %s\n", result.Summary)
		fmt.Printf("     Slug: %s\n", result.Slug)
		fmt.Printf("     Registry: %s\n", result.RegistryName)
		if result.Version != "" {
			fmt.Printf("     Version: %s\n", result.Version)
		}
		fmt.Println()
	}
}
