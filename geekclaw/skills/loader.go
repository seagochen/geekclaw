package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
	"gopkg.in/yaml.v3"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
)

// namePattern 是技能名称的合法格式正则表达式。
var namePattern = regexp.MustCompile(`^[a-zA-Z0-9]+(-[a-zA-Z0-9]+)*$`)

const (
	MaxNameLength        = 64   // 技能名称最大长度
	MaxDescriptionLength = 1024 // 技能描述最大长度
)

// SkillMetadata 包含技能的名称和描述元数据。
type SkillMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SkillInfo 包含技能的完整信息，包括名称、路径、来源和描述。
type SkillInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

// validate 验证技能信息的合法性。
func (info SkillInfo) validate() error {
	var errs error
	if info.Name == "" {
		errs = errors.Join(errs, errors.New("name is required"))
	} else {
		if len(info.Name) > MaxNameLength {
			errs = errors.Join(errs, fmt.Errorf("name exceeds %d characters", MaxNameLength))
		}
		if !namePattern.MatchString(info.Name) {
			errs = errors.Join(errs, errors.New("name must be alphanumeric with hyphens"))
		}
	}

	if info.Description == "" {
		errs = errors.Join(errs, errors.New("description is required"))
	} else if len(info.Description) > MaxDescriptionLength {
		errs = errors.Join(errs, fmt.Errorf("description exceeds %d character", MaxDescriptionLength))
	}
	return errs
}

// SkillsLoader 从多个源目录加载技能，按优先级解析。
type SkillsLoader struct {
	pluginsDir    string // 插件目录
	pluginsSkills string // 插件级技能目录（项目级）
	globalSkills  string // 全局技能目录（~/.geekclaw/skills）
	builtinSkills string // 内置技能目录
}

// SkillRoots 返回此加载器使用的所有唯一技能根目录。
// 顺序遵循解析优先级：plugins > global > builtin。
func (sl *SkillsLoader) SkillRoots() []string {
	roots := []string{sl.pluginsSkills, sl.globalSkills, sl.builtinSkills}
	seen := make(map[string]struct{}, len(roots))
	out := make([]string, 0, len(roots))

	for _, root := range roots {
		trimmed := strings.TrimSpace(root)
		if trimmed == "" {
			continue
		}
		clean := filepath.Clean(trimmed)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}

	return out
}

// NewSkillsLoader 创建一个新的技能加载器。
func NewSkillsLoader(pluginsDir string, globalSkills string, builtinSkills string) *SkillsLoader {
	return &SkillsLoader{
		pluginsDir:    pluginsDir,
		pluginsSkills: filepath.Join(pluginsDir, "skills"),
		globalSkills:  globalSkills,  // ~/.geekclaw/skills
		builtinSkills: builtinSkills,
	}
}

// ListSkills 列出所有可用技能，按优先级去重。
func (sl *SkillsLoader) ListSkills() []SkillInfo {
	skills := make([]SkillInfo, 0)
	seen := make(map[string]bool)

	addSkills := func(dir, source string) {
		if dir == "" {
			return
		}
		dirs, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, d := range dirs {
			if !d.IsDir() {
				continue
			}
			skillFile := filepath.Join(dir, d.Name(), "SKILL.md")
			if _, err := os.Stat(skillFile); err != nil {
				continue
			}
			info := SkillInfo{
				Name:   d.Name(),
				Path:   skillFile,
				Source: source,
			}
			metadata := sl.getSkillMetadata(skillFile)
			if metadata != nil {
				info.Description = metadata.Description
				info.Name = metadata.Name
			}
			if err := info.validate(); err != nil {
				slog.Warn("invalid skill from "+source, "name", info.Name, "error", err)
				continue
			}
			if seen[info.Name] {
				continue
			}
			seen[info.Name] = true
			skills = append(skills, info)
		}
	}

	// 优先级：工作区 > 全局 > 内置
	addSkills(sl.pluginsSkills, "plugins")
	addSkills(sl.globalSkills, "global")
	addSkills(sl.builtinSkills, "builtin")

	return skills
}

// LoadSkill 按优先级从各技能目录加载指定名称的技能内容。
func (sl *SkillsLoader) LoadSkill(name string) (string, bool) {
	// 1. 首先从工作区技能加载（项目级）
	if sl.pluginsSkills != "" {
		skillFile := filepath.Join(sl.pluginsSkills, name, "SKILL.md")
		if content, err := os.ReadFile(skillFile); err == nil {
			return sl.stripFrontmatter(string(content)), true
		}
	}

	// 2. 然后从全局技能加载（~/.geekclaw/skills）
	if sl.globalSkills != "" {
		skillFile := filepath.Join(sl.globalSkills, name, "SKILL.md")
		if content, err := os.ReadFile(skillFile); err == nil {
			return sl.stripFrontmatter(string(content)), true
		}
	}

	// 3. 最后从内置技能加载
	if sl.builtinSkills != "" {
		skillFile := filepath.Join(sl.builtinSkills, name, "SKILL.md")
		if content, err := os.ReadFile(skillFile); err == nil {
			return sl.stripFrontmatter(string(content)), true
		}
	}

	return "", false
}

// LoadSkillsForContext 加载指定技能列表的内容，格式化为上下文字符串。
func (sl *SkillsLoader) LoadSkillsForContext(skillNames []string) string {
	if len(skillNames) == 0 {
		return ""
	}

	var parts []string
	for _, name := range skillNames {
		content, ok := sl.LoadSkill(name)
		if ok {
			parts = append(parts, fmt.Sprintf("### Skill: %s\n\n%s", name, content))
		}
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// BuildSkillsSummary 构建所有可用技能的 XML 格式摘要。
func (sl *SkillsLoader) BuildSkillsSummary() string {
	allSkills := sl.ListSkills()
	if len(allSkills) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "<skills>")
	for _, s := range allSkills {
		escapedName := escapeXML(s.Name)
		escapedDesc := escapeXML(s.Description)
		escapedPath := escapeXML(s.Path)

		lines = append(lines, fmt.Sprintf("  <skill>"))
		lines = append(lines, fmt.Sprintf("    <name>%s</name>", escapedName))
		lines = append(lines, fmt.Sprintf("    <description>%s</description>", escapedDesc))
		lines = append(lines, fmt.Sprintf("    <location>%s</location>", escapedPath))
		lines = append(lines, fmt.Sprintf("    <source>%s</source>", s.Source))
		lines = append(lines, "  </skill>")
	}
	lines = append(lines, "</skills>")

	return strings.Join(lines, "\n")
}

// getSkillMetadata 从技能文件中读取并解析元数据。
func (sl *SkillsLoader) getSkillMetadata(skillPath string) *SkillMetadata {
	content, err := os.ReadFile(skillPath)
	if err != nil {
		logger.WarnCF("skills", "Failed to read skill metadata",
			map[string]any{
				"skill_path": skillPath,
				"error":      err.Error(),
			})
		return nil
	}

	frontmatter, bodyContent := splitFrontmatter(string(content))
	dirName := filepath.Base(filepath.Dir(skillPath))
	title, bodyDescription := extractMarkdownMetadata(bodyContent)

	metadata := &SkillMetadata{
		Name:        dirName,
		Description: bodyDescription,
	}
	if title != "" && namePattern.MatchString(title) && len(title) <= MaxNameLength {
		metadata.Name = title
	}

	if frontmatter == "" {
		return metadata
	}

	// 首先尝试 JSON 解析（为了向后兼容）
	var jsonMeta struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(frontmatter), &jsonMeta); err == nil {
		if jsonMeta.Name != "" {
			metadata.Name = jsonMeta.Name
		}
		if jsonMeta.Description != "" {
			metadata.Description = jsonMeta.Description
		}
		return metadata
	}

	// 回退到简单 YAML 解析
	yamlMeta := sl.parseSimpleYAML(frontmatter)
	if name := yamlMeta["name"]; name != "" {
		metadata.Name = name
	}
	if description := yamlMeta["description"]; description != "" {
		metadata.Description = description
	}
	return metadata
}

// extractMarkdownMetadata 从 Markdown 内容中提取标题和描述。
func extractMarkdownMetadata(content string) (title, description string) {
	p := parser.NewWithExtensions(parser.CommonExtensions)
	doc := markdown.Parse([]byte(content), p)
	if doc == nil {
		return "", ""
	}

	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}

		switch n := node.(type) {
		case *ast.Heading:
			if title == "" && n.Level == 1 {
				title = nodeText(n)
				if title != "" && description != "" {
					return ast.Terminate
				}
			}
		case *ast.Paragraph:
			if description == "" {
				description = nodeText(n)
				if title != "" && description != "" {
					return ast.Terminate
				}
			}
		}
		return ast.GoToNext
	})

	return title, description
}

// nodeText 递归提取 AST 节点的纯文本内容。
func nodeText(n ast.Node) string {
	var b strings.Builder
	ast.WalkFunc(n, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}

		switch t := node.(type) {
		case *ast.Text:
			b.Write(t.Literal)
		case *ast.Code:
			b.Write(t.Literal)
		case *ast.Softbreak, *ast.Hardbreak, *ast.NonBlockingSpace:
			b.WriteByte(' ')
		}
		return ast.GoToNext
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

// parseSimpleYAML 解析 YAML 前置元数据并提取已知的元数据字段。
func (sl *SkillsLoader) parseSimpleYAML(content string) map[string]string {
	result := make(map[string]string)

	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(content), &meta); err != nil {
		return result
	}
	if meta.Name != "" {
		result["name"] = meta.Name
	}
	if meta.Description != "" {
		result["description"] = meta.Description
	}

	return result
}

// extractFrontmatter 从内容中提取前置元数据部分。
func (sl *SkillsLoader) extractFrontmatter(content string) string {
	frontmatter, _ := splitFrontmatter(content)
	return frontmatter
}

// stripFrontmatter 从内容中去除前置元数据部分，返回正文。
func (sl *SkillsLoader) stripFrontmatter(content string) string {
	_, body := splitFrontmatter(content)
	return body
}

// splitFrontmatter 将内容分割为前置元数据和正文两部分。
func splitFrontmatter(content string) (frontmatter, body string) {
	normalized := string(parser.NormalizeNewlines([]byte(content)))
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return "", content
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return "", content
	}

	frontmatter = strings.Join(lines[1:end], "\n")
	body = strings.Join(lines[end+1:], "\n")
	body = strings.TrimLeft(body, "\n")
	return frontmatter, body
}

// escapeXML 转义字符串中的 XML 特殊字符。
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
