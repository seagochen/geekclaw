package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/seagosoft/geekclaw/geekclaw/internal"
	"github.com/seagosoft/geekclaw/geekclaw/config"
	"github.com/seagosoft/geekclaw/geekclaw/fileutil"
	"github.com/seagosoft/geekclaw/geekclaw/skills"
	"github.com/seagosoft/geekclaw/geekclaw/utils"
)

// newInstallCommand 创建从 GitHub 或注册中心安装技能的子命令。
func newInstallCommand(installerFn func() (*skills.SkillInstaller, error)) *cobra.Command {
	var registry string

	cmd := &cobra.Command{
		Use:     "install",
		Short:   "Install skill from GitHub",
		Example: `geekclaw skills install seagosoft/geekclaw-skills/weather`,
		Args: func(_ *cobra.Command, args []string) error {
			if registry != "" {
				if len(args) != 1 {
					return fmt.Errorf("when --registry is set, exactly 1 argument is required: <slug>")
				}
			} else {
				if len(args) != 1 {
					return fmt.Errorf("exactly 1 argument is required: <github>")
				}
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			installer, err := installerFn()
			if err != nil {
				return err
			}

			if registry != "" {
				cfg, err := internal.LoadConfig()
				if err != nil {
					return fmt.Errorf("failed to load config: %w", err)
				}
				return skillsInstallFromRegistry(cfg, registry, args[0])
			}

			return skillsInstallCmd(installer, args[0])
		},
	}

	cmd.Flags().StringVarP(&registry, "registry", "r", "", "Install from a named registry (e.g., clawhub)")

	return cmd
}

// skillsInstallCmd 从 GitHub 仓库安装技能。
func skillsInstallCmd(installer *skills.SkillInstaller, repo string) error {
	fmt.Printf("Installing skill from %s...\n", repo)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := installer.InstallFromGitHub(ctx, repo); err != nil {
		return fmt.Errorf("failed to install skill: %w", err)
	}

	fmt.Printf("\u2713 Skill '%s' installed successfully!\n", filepath.Base(repo))

	return nil
}

// skillsInstallFromRegistry 从指定的注册中心（如 clawhub）安装技能。
func skillsInstallFromRegistry(cfg *config.Config, registryName, slug string) error {
	err := utils.ValidateSkillIdentifier(registryName)
	if err != nil {
		return fmt.Errorf("✗  invalid registry name: %w", err)
	}

	err = utils.ValidateSkillIdentifier(slug)
	if err != nil {
		return fmt.Errorf("✗  invalid slug: %w", err)
	}

	fmt.Printf("Installing skill '%s' from %s registry...\n", slug, registryName)

	registryMgr := skills.NewRegistryManagerFromConfig(skills.RegistryConfig{
		MaxConcurrentSearches: cfg.Tools.Skills.MaxConcurrentSearches,
		ClawHub:               skills.ClawHubConfig(cfg.Tools.Skills.Registries.ClawHub),
	})

	registry := registryMgr.GetRegistry(registryName)
	if registry == nil {
		return fmt.Errorf("✗  registry '%s' not found or not enabled. check your config.yaml.", registryName)
	}

	pluginsDir := cfg.PluginsPath()
	targetDir := filepath.Join(pluginsDir, "skills", slug)

	if _, err = os.Stat(targetDir); err == nil {
		return fmt.Errorf("\u2717 skill '%s' already installed at %s", slug, targetDir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err = os.MkdirAll(filepath.Join(pluginsDir, "skills"), 0o755); err != nil {
		return fmt.Errorf("\u2717 failed to create skills directory: %v", err)
	}

	result, err := registry.DownloadAndInstall(ctx, slug, "", targetDir)
	if err != nil {
		rmErr := os.RemoveAll(targetDir)
		if rmErr != nil {
			fmt.Printf("\u2717 Failed to remove partial install: %v\n", rmErr)
		}
		return fmt.Errorf("✗ failed to install skill: %w", err)
	}

	if result.IsMalwareBlocked {
		rmErr := os.RemoveAll(targetDir)
		if rmErr != nil {
			fmt.Printf("\u2717 Failed to remove partial install: %v\n", rmErr)
		}

		return fmt.Errorf("\u2717 Skill '%s' is flagged as malicious and cannot be installed.\n", slug)
	}

	if result.IsSuspicious {
		fmt.Printf("\u26a0\ufe0f  Warning: skill '%s' is flagged as suspicious.\n", slug)
	}

	fmt.Printf("\u2713 Skill '%s' v%s installed successfully!\n", slug, result.Version)
	if result.Summary != "" {
		fmt.Printf("  %s\n", result.Summary)
	}

	return nil
}

// newInstallBuiltinCommand 创建安装所有内置技能到插件目录的子命令。
func newInstallBuiltinCommand(pluginsDirFn func() (string, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "install-builtin",
		Short:   "Install all builtin skills to plugins directory",
		Example: `geekclaw skills install-builtin`,
		RunE: func(_ *cobra.Command, _ []string) error {
			pluginsDir, err := pluginsDirFn()
			if err != nil {
				return err
			}
			skillsInstallBuiltinCmd(pluginsDir)
			return nil
		},
	}

	return cmd
}

// skillsInstallBuiltinCmd 将内置技能从全局目录复制到当前插件目录。
func skillsInstallBuiltinCmd(pluginsDir string) {
	builtinSkillsDir := filepath.Join(os.Getenv("GEEKCLAW_HOME"), "plugins", "skills")
	if os.Getenv("GEEKCLAW_HOME") == "" {
		home, _ := os.UserHomeDir()
		builtinSkillsDir = filepath.Join(home, ".geekclaw", "plugins", "skills")
	}
	targetSkillsDir := filepath.Join(pluginsDir, "skills")

	fmt.Printf("Copying builtin skills to plugins directory...\n")

	skillsToInstall := []string{
		"weather",
		"news",
		"stock",
		"calculator",
	}

	for _, skillName := range skillsToInstall {
		builtinPath := filepath.Join(builtinSkillsDir, skillName)
		targetPath := filepath.Join(targetSkillsDir, skillName)

		if _, err := os.Stat(builtinPath); err != nil {
			fmt.Printf("⊘ Builtin skill '%s' not found: %v\n", skillName, err)
			continue
		}

		if err := os.MkdirAll(targetPath, 0o755); err != nil {
			fmt.Printf("✗ Failed to create directory for %s: %v\n", skillName, err)
			continue
		}

		if err := fileutil.CopyDirectory(builtinPath, targetPath); err != nil {
			fmt.Printf("✗ Failed to copy %s: %v\n", skillName, err)
		}
	}

	fmt.Println("\n✓ All builtin skills installed!")
	fmt.Println("Now you can use them in your plugins directory.")
}


