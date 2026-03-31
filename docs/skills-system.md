# Skills 系统：加载与交互机制

本文档详细描述 GeekClaw 启动后，`plugins/skills/` 下的技能文件如何被发现、加载、以及在对话中被调用。

## 1. 概述

GeekClaw 的 Skills 系统是**文档驱动**的，而非代码驱动。每个 Skill 就是一个 `SKILL.md` 文件，包含 Markdown 格式的使用说明和示例命令。LLM 在对话中根据用户意图自主决定读取和执行哪个 Skill。

```
用户消息
  │
  ▼
系统提示词（含 Skills 摘要列表）
  │
  ▼
LLM 判断需要哪个 Skill
  │
  ├─ read_file(SKILL.md)   读取完整技能文档
  │
  ├─ shell("curl ...")      按文档中的示例执行命令
  │
  └─ 返回结果给用户
```

## 2. Skill 目录结构

```
plugins/skills/
  ├── weather/
  │   └── SKILL.md
  ├── github/
  │   └── SKILL.md
  ├── summarize/
  │   └── SKILL.md
  └── tmux/
      └── SKILL.md
```

每个 Skill 是一个独立目录，目录内必须包含 `SKILL.md` 文件。

## 3. SKILL.md 文件格式

### 3.1 Frontmatter（可选）

```yaml
---
name: weather
description: Get current weather and forecasts using curl
homepage: https://github.com/example/weather-skill
metadata: {"nanobot":{"emoji":"🌤️","requires":{"bins":["curl"]}}}
---
```

| 字段 | 说明 |
|------|------|
| `name` | 技能名称，字母数字+连字符，最长 64 字符 |
| `description` | 一行描述，最长 1024 字符 |
| `homepage` | 可选，技能主页 URL |
| `metadata` | 可选，扩展元数据（JSON） |

### 3.2 Fallback 解析

如果没有 frontmatter：
- **name**：从第一个 `# 标题` 提取
- **description**：从第一段正文提取

### 3.3 正文内容

Markdown 格式，通常包含：
- 功能描述
- 使用示例（含可执行的 shell 命令）
- 触发短语和使用说明

## 4. 发现与加载流程

**核心代码**：`pkg/skills/loader.go` — `SkillsLoader`

### 4.1 三级优先级扫描

Skills 从三个位置按优先级扫描，**先匹配的优先，不重复加载**：

| 优先级 | 路径 | 来源标签 |
|--------|------|----------|
| 1（最高）| `{pluginsDir}/skills/` | `plugins` |
| 2 | `~/.geekclaw/skills/` | `global` |
| 3（最低）| 内置 skills 目录 | `builtin` |

### 4.2 扫描过程

```
ListSkills()
  │
  ├─ 遍历每个优先级目录
  │   ├─ 扫描子目录
  │   ├─ 检查是否存在 SKILL.md
  │   ├─ 解析 frontmatter 或 fallback 提取元数据
  │   └─ 生成 SkillInfo{Name, Path, Source, Description}
  │
  └─ 返回去重后的 SkillInfo 列表
```

### 4.3 输出结构

```go
type SkillInfo struct {
    Name        string // 技能名称
    Path        string // SKILL.md 完整路径
    Source      string // "plugins" | "global" | "builtin"
    Description string // 一行描述
}
```

## 5. 系统提示词集成

**核心代码**：`pkg/agent/context.go`、`pkg/agent/skills_context.go`

### 5.1 Skills 摘要注入

启动后，所有已发现的 Skills 被编译为 XML 摘要，嵌入系统提示词：

```xml
<skills>
  <skill>
    <name>weather</name>
    <description>Get current weather and forecasts using curl</description>
    <location>/path/to/plugins/skills/weather/SKILL.md</location>
    <source>plugins</source>
  </skill>
  <skill>
    <name>github</name>
    <description>GitHub CLI operations</description>
    <location>/path/to/plugins/skills/github/SKILL.md</location>
    <source>plugins</source>
  </skill>
</skills>
```

系统提示词同时指示 LLM："如需使用技能，请先用 `read_file` 工具读取对应的 SKILL.md 文件。"

### 5.2 缓存机制

- Skills 摘要作为静态系统提示词的一部分被缓存
- 缓存基于文件修改时间（mtime）自动失效
- 递归监控所有 Skill 文件的变更

```
BuildSystemPromptWithCache()
  │
  ├─ 检查缓存是否有效（mtime 比较）
  │   ├─ 有效 → 返回缓存
  │   └─ 失效 → 重新扫描 + 构建摘要 → 更新缓存
  │
  └─ 返回完整系统提示词
```

## 6. 对话中的调用流程

Skills **没有**直接的调用机制。LLM 完全自主决定是否使用以及如何使用 Skill。

### 6.1 完整流程

```
1. 用户发送消息："伦敦天气怎么样？"
   │
2. 构建消息列表（含 Skills 摘要的系统提示词）
   │
3. 调用 LLM → runLLMIteration()
   │
4. LLM 识别出 "weather" Skill 相关
   │
5. LLM 调用工具：read_file("plugins/skills/weather/SKILL.md")
   │  ← 返回完整技能文档
   │
6. LLM 从文档中提取命令模板
   │
7. LLM 调用工具：shell("curl -s 'wttr.in/London?format=3'")
   │  ← 返回 "London: ⛅ +8°C"
   │
8. LLM 生成最终回复："伦敦现在 8°C，多云。"
```

### 6.2 工具迭代循环

```go
// pkg/agent/llm.go
runLLMIteration() {
    for iteration < maxIterations {
        response = provider.Chat(messages, tools)

        if response 无工具调用 → 返回文本回复
        if response 有工具调用 → 并行执行所有工具
            → 将工具结果追加到消息历史
            → 继续下一轮迭代
    }
}
```

LLM 可以在一次对话中链式调用多个工具：先 `read_file` 读取 SKILL.md，再 `shell` 执行命令，最终生成回复。

## 7. Skill 注册与安装

**核心代码**：`pkg/skills/registry.go`、`pkg/skills/installer.go`

### 7.1 在线注册表

支持从 ClawHub 等注册表搜索和安装 Skills：

```
find_skills("天气查询")
  │
  ├─ 并发搜索各注册表
  ├─ 按相关性排序
  └─ 返回匹配结果

install_skill("weather")
  │
  ├─ 从 GitHub/注册表下载 SKILL.md
  ├─ 写入 {pluginsDir}/skills/{name}/SKILL.md
  └─ 原子写入保证可靠性
```

### 7.2 安装位置

安装的 Skill 存放在 `{pluginsDir}/skills/` 目录，属于 `plugins` 优先级层级。

## 8. 设计特点

| 特点 | 说明 |
|------|------|
| **文档驱动** | Skill 是纯 Markdown，无需编译或运行时 |
| **自包含** | 每个 Skill 一个目录，一个文件 |
| **只读** | LLM 只能读取 SKILL.md，不会修改 |
| **工具无关** | Skill 通过标准工具（shell、web 等）执行 |
| **可版本控制** | 纯文本文件，适合 Git 管理 |
| **热更新** | 修改 SKILL.md 后，基于 mtime 的缓存自动失效 |
