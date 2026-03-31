package skills

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/seagosoft/geekclaw/geekclaw/fileutil"
	"github.com/seagosoft/geekclaw/geekclaw/utils"
)

// SkillInstaller 负责从远程源安装和卸载技能。
type SkillInstaller struct {
	pluginsDir string // 插件目录路径
}

// NewSkillInstaller 创建一个新的技能安装器。
func NewSkillInstaller(pluginsDir string) *SkillInstaller {
	return &SkillInstaller{
		pluginsDir: pluginsDir,
	}
}

// InstallFromGitHub 从 GitHub 仓库安装技能。
// 它会从仓库的 main 分支获取 SKILL.md 文件并保存到本地插件目录。
func (si *SkillInstaller) InstallFromGitHub(ctx context.Context, repo string) error {
	skillDir := filepath.Join(si.pluginsDir, "skills", filepath.Base(repo))

	if _, err := os.Stat(skillDir); err == nil {
		return fmt.Errorf("skill '%s' already exists", filepath.Base(repo))
	}

	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/SKILL.md", repo)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := utils.DoRequestWithRetry(client, req)
	if err != nil {
		return fmt.Errorf("failed to fetch skill: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to fetch skill: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	skillPath := filepath.Join(skillDir, "SKILL.md")

	// 使用统一的原子写入工具，并显式同步以确保闪存存储的可靠性。
	if err := fileutil.WriteFileAtomic(skillPath, body, 0o600); err != nil {
		return fmt.Errorf("failed to write skill file: %w", err)
	}

	return nil
}

// Uninstall 卸载指定名称的技能，删除其整个目录。
func (si *SkillInstaller) Uninstall(skillName string) error {
	skillDir := filepath.Join(si.pluginsDir, "skills", skillName)

	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		return fmt.Errorf("skill '%s' not found", skillName)
	}

	if err := os.RemoveAll(skillDir); err != nil {
		return fmt.Errorf("failed to remove skill: %w", err)
	}

	return nil
}
