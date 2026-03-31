package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
	"github.com/seagosoft/geekclaw/geekclaw/skills"
)

// newListCommand 创建列出已安装技能的子命令。
func newListCommand(loaderFn func() (*skills.SkillsLoader, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List installed skills",
		Example: `geekclaw skills list`,
		RunE: func(_ *cobra.Command, _ []string) error {
			loader, err := loaderFn()
			if err != nil {
				return err
			}
			skillsListCmd(loader)
			return nil
		},
	}

	return cmd
}

// skillsListCmd 显示所有已安装技能的列表。
func skillsListCmd(loader *skills.SkillsLoader) {
	allSkills := loader.ListSkills()

	if len(allSkills) == 0 {
		fmt.Println("No skills installed.")
		return
	}

	fmt.Println("\nInstalled Skills:")
	fmt.Println("------------------")
	for _, skill := range allSkills {
		fmt.Printf("  ✓ %s (%s)\n", skill.Name, skill.Source)
		if skill.Description != "" {
			fmt.Printf("    %s\n", skill.Description)
		}
	}
}

// newListBuiltinCommand 创建列出可用内置技能的子命令。
func newListBuiltinCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list-builtin",
		Short:   "List available builtin skills",
		Example: `geekclaw skills list-builtin`,
		Run: func(_ *cobra.Command, _ []string) {
			skillsListBuiltinCmd()
		},
	}

	return cmd
}

// skillsListBuiltinCmd 显示内置技能目录中的可用技能。
func skillsListBuiltinCmd() {
	cfg, err := internal.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}
	builtinSkillsDir := filepath.Join(cfg.PluginsPath(), "skills")

	fmt.Println("\nAvailable Builtin Skills:")
	fmt.Println("-----------------------")

	entries, err := os.ReadDir(builtinSkillsDir)
	if err != nil {
		fmt.Printf("Error reading builtin skills: %v\n", err)
		return
	}

	if len(entries) == 0 {
		fmt.Println("No builtin skills available.")
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			skillName := entry.Name()
			skillFile := filepath.Join(builtinSkillsDir, skillName, "SKILL.md")

			description := "No description"
			if _, err := os.Stat(skillFile); err == nil {
				data, err := os.ReadFile(skillFile)
				if err == nil {
					content := string(data)
					if idx := strings.Index(content, "\n"); idx > 0 {
						firstLine := content[:idx]
						if strings.Contains(firstLine, "description:") {
							descLine := strings.Index(content[idx:], "\n")
							if descLine > 0 {
								description = strings.TrimSpace(content[idx+descLine : idx+descLine])
							}
						}
					}
				}
			}
			status := "✓"
			fmt.Printf("  %s  %s\n", status, entry.Name())
			if description != "" {
				fmt.Printf("     %s\n", description)
			}
		}
	}
}
