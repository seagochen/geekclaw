package skills

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/seagosoft/geekclaw/geekclaw/skills"
)

// newShowCommand 创建查看技能详情的子命令。
func newShowCommand(loaderFn func() (*skills.SkillsLoader, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show",
		Short:   "Show skill details",
		Example: `geekclaw skills show weather`,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("exactly 1 argument is required: <skill-name>")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			loader, err := loaderFn()
			if err != nil {
				return err
			}
			skillsShowCmd(loader, args[0])
			return nil
		},
	}

	return cmd
}

// skillsShowCmd 加载并显示指定技能的详细内容。
func skillsShowCmd(loader *skills.SkillsLoader, skillName string) {
	content, ok := loader.LoadSkill(skillName)
	if !ok {
		fmt.Printf("✗ Skill '%s' not found\n", skillName)
		return
	}

	fmt.Printf("\n📦 Skill: %s\n", skillName)
	fmt.Println("----------------------")
	fmt.Println(content)
}
