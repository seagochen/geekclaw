package config

import "os"

// GeekClaw 支持的环境变量名称。
// 以下是规范名称——请使用下方的访问函数来读取它们。
const (
	// EnvGeekclawHome 覆盖 GeekClaw 主目录（默认：~/.geekclaw）。
	EnvGeekclawHome = "GEEKCLAW_HOME"

	// EnvBuiltinSkills 覆盖内置技能目录
	// （默认：$GEEKCLAW_HOME/plugins/skills）。
	EnvBuiltinSkills = "GEEKCLAW_BUILTIN_SKILLS"
)

// GeekclawHome 返回 GEEKCLAW_HOME 的值，未设置时返回空字符串。
func GeekclawHome() string { return os.Getenv(EnvGeekclawHome) }

// BuiltinSkills 返回 GEEKCLAW_BUILTIN_SKILLS 的值，未设置时返回空字符串。
func BuiltinSkills() string { return os.Getenv(EnvBuiltinSkills) }
