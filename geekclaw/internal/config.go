package internal

import (
	"os"
	"path/filepath"

	"github.com/seagosoft/geekclaw/geekclaw/config"
)

const Logo = "🦞"

// GetGeekclawHome 返回 GeekClaw 的主目录。
// 优先级: $GEEKCLAW_HOME > ~/.geekclaw
func GetGeekclawHome() string {
	if home := os.Getenv("GEEKCLAW_HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".geekclaw")
}

// GetConfigPath 返回配置文件路径，按优先级依次查找新布局和旧布局。
func GetConfigPath() string {
	if configPath := os.Getenv("GEEKCLAW_CONFIG"); configPath != "" {
		return configPath
	}
	home := GetGeekclawHome()
	// 新布局：配置文件位于 configs/ 子目录下。
	yamlPath := filepath.Join(home, "configs", "config.yaml")
	jsonPath := filepath.Join(home, "configs", "config.json")
	// 旧布局路径（向后兼容）。
	legacyYaml := filepath.Join(home, "config.yaml")
	legacyJSON := filepath.Join(home, "config.json")
	// 优先使用新布局；旧安装回退到旧路径。
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath
	}
	if _, err := os.Stat(jsonPath); err == nil {
		return jsonPath
	}
	if _, err := os.Stat(legacyYaml); err == nil {
		return legacyYaml
	}
	if _, err := os.Stat(legacyJSON); err == nil {
		return legacyJSON
	}
	return yamlPath // 新安装的默认路径
}

// LoadConfig 加载并返回 GeekClaw 配置。
func LoadConfig() (*config.Config, error) {
	return config.LoadConfig(GetConfigPath())
}
