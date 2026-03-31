package skills

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/seagosoft/geekclaw/geekclaw/skills"
)

// newRemoveCommand 创建删除已安装技能的子命令。
func newRemoveCommand(installerFn func() (*skills.SkillInstaller, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove",
		Short:   "Remove installed skill",
		Aliases: []string{"rm", "uninstall"},
		Example: `geekclaw skills remove weather`,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("exactly 1 argument is required: <skill-name>")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			installer, err := installerFn()
			if err != nil {
				return err
			}
			skillsRemoveCmd(installer, args[0])
			return nil
		},
	}

	return cmd
}

// skillsRemoveCmd 卸载指定名称的技能。
func skillsRemoveCmd(installer *skills.SkillInstaller, skillName string) {
	fmt.Printf("Removing skill '%s'...\n", skillName)

	if err := installer.Uninstall(skillName); err != nil {
		fmt.Printf("✗ Failed to remove skill: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Skill '%s' removed successfully!\n", skillName)
}
