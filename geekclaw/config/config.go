package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/caarlos0/env/v11"
	"gopkg.in/yaml.v3"

	"github.com/seagosoft/geekclaw/geekclaw/fileutil"
)

// FlexibleStringSlice 是一个 []string 类型，同时接受 YAML 数字，
// 因此 allow_from 可以同时包含 "123" 和 123。
type FlexibleStringSlice []string

// UnmarshalYAML 实现 FlexibleStringSlice 的自定义 YAML 反序列化。
func (f *FlexibleStringSlice) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		*f = nil
		return nil
	}
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("expected sequence for FlexibleStringSlice, got kind %d", value.Kind)
	}
	result := make([]string, 0, len(value.Content))
	for _, item := range value.Content {
		// item.Value 是所有标量类型的原始字符串表示
		result = append(result, item.Value)
	}
	*f = result
	return nil
}

// Config 是 GeekClaw 的顶层配置结构。
type Config struct {
	Agents    AgentsConfig    `json:"agents" yaml:"agents"`
	Bindings  []AgentBinding  `json:"bindings,omitempty" yaml:"bindings,omitempty"`
	Session   SessionConfig   `json:"session,omitempty" yaml:"session,omitempty"`
	Channels  ChannelsConfig  `json:"channels" yaml:"channels"`
	Commands  CommandsConfig  `json:"commands,omitempty" yaml:"commands,omitempty"`
	Providers ProvidersConfig `json:"providers,omitempty" yaml:"providers,omitempty"`
	ModelList []ModelConfig   `json:"model_list" yaml:"model_list"` // 新的以模型为中心的提供者配置
	Gateway   GatewayConfig   `json:"gateway" yaml:"gateway"`
	Tools     ToolsConfig     `json:"tools" yaml:"tools"`
	Voice     VoiceConfig     `json:"voice" yaml:"voice"`
	Auth      AuthConfig      `json:"auth,omitempty" yaml:"auth,omitempty"`
	// BuildInfo 包含构建时的版本信息
	BuildInfo BuildInfo `json:"build_info,omitempty" yaml:"build_info,omitempty"`
}

// BuildInfo 包含构建时的版本信息。
type BuildInfo struct {
	Version   string `json:"version" yaml:"version"`
	GitCommit string `json:"git_commit" yaml:"git_commit"`
	BuildTime string `json:"build_time" yaml:"build_time"`
	GoVersion string `json:"go_version" yaml:"go_version"`
}

// MarshalYAML 实现 Config 的自定义 YAML 序列化，
// 在 providers 和 session 为空时省略它们。
func (c Config) MarshalYAML() (interface{}, error) {
	type Alias Config
	return (*Alias)(&c), nil
}

// AgentsConfig 定义代理的配置，包括默认值和代理列表。
type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults" yaml:"defaults"`
	List     []AgentConfig `json:"list,omitempty" yaml:"list,omitempty"`
}

// AgentModelConfig 同时支持字符串和结构化的模型配置。
// 字符串格式："gpt-4"（仅主模型，无备选）
// 对象格式：{"primary": "gpt-4", "fallbacks": ["claude-haiku"]}
type AgentModelConfig struct {
	Primary   string   `json:"primary,omitempty" yaml:"primary,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty" yaml:"fallbacks,omitempty"`
}

// UnmarshalYAML 实现 AgentModelConfig 的自定义 YAML 反序列化。
func (m *AgentModelConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		m.Primary = value.Value
		m.Fallbacks = nil
		return nil
	}
	type raw struct {
		Primary   string   `yaml:"primary"`
		Fallbacks []string `yaml:"fallbacks"`
	}
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	m.Primary = r.Primary
	m.Fallbacks = r.Fallbacks
	return nil
}

// MarshalYAML 实现 AgentModelConfig 的自定义 YAML 序列化。
func (m AgentModelConfig) MarshalYAML() (interface{}, error) {
	if len(m.Fallbacks) == 0 && m.Primary != "" {
		return m.Primary, nil
	}
	type raw struct {
		Primary   string   `yaml:"primary,omitempty"`
		Fallbacks []string `yaml:"fallbacks,omitempty"`
	}
	return raw{Primary: m.Primary, Fallbacks: m.Fallbacks}, nil
}

// AgentConfig 定义单个代理的配置。
type AgentConfig struct {
	ID         string            `json:"id" yaml:"id"`
	Default    bool              `json:"default,omitempty" yaml:"default,omitempty"`
	Name       string            `json:"name,omitempty" yaml:"name,omitempty"`
	Workspace  string            `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	PluginsDir string            `json:"plugins_dir,omitempty" yaml:"plugins_dir,omitempty"`
	Model     *AgentModelConfig `json:"model,omitempty" yaml:"model,omitempty"`
	Skills    []string          `json:"skills,omitempty" yaml:"skills,omitempty"`
	Subagents *SubagentsConfig  `json:"subagents,omitempty" yaml:"subagents,omitempty"`
}

// SubagentsConfig 定义子代理的配置。
type SubagentsConfig struct {
	AllowAgents []string          `json:"allow_agents,omitempty" yaml:"allow_agents,omitempty"`
	Model       *AgentModelConfig `json:"model,omitempty" yaml:"model,omitempty"`
}

// PeerMatch 定义对端匹配规则。
type PeerMatch struct {
	Kind string `json:"kind" yaml:"kind"`
	ID   string `json:"id" yaml:"id"`
}

// BindingMatch 定义代理绑定的匹配条件。
type BindingMatch struct {
	Channel   string     `json:"channel" yaml:"channel"`
	AccountID string     `json:"account_id,omitempty" yaml:"account_id,omitempty"`
	Peer      *PeerMatch `json:"peer,omitempty" yaml:"peer,omitempty"`
	GuildID   string     `json:"guild_id,omitempty" yaml:"guild_id,omitempty"`
	TeamID    string     `json:"team_id,omitempty" yaml:"team_id,omitempty"`
}

// AgentBinding 将代理绑定到特定的渠道和匹配条件。
type AgentBinding struct {
	AgentID string       `json:"agent_id" yaml:"agent_id"`
	Match   BindingMatch `json:"match" yaml:"match"`
}

// SessionConfig 定义会话管理的配置。
type SessionConfig struct {
	DMScope       string              `json:"dm_scope,omitempty" yaml:"dm_scope,omitempty"`
	IdentityLinks map[string][]string `json:"identity_links,omitempty" yaml:"identity_links,omitempty"`
}

// AgentDefaults 定义代理的默认配置参数。
type AgentDefaults struct {
	Workspace                 string         `json:"workspace" yaml:"workspace"                       env:"GEEKCLAW_AGENTS_DEFAULTS_WORKSPACE"`
	RestrictToWorkspace       bool           `json:"restrict_to_workspace" yaml:"restrict_to_workspace"           env:"GEEKCLAW_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE"`
	AllowReadOutsideWorkspace bool           `json:"allow_read_outside_workspace" yaml:"allow_read_outside_workspace"    env:"GEEKCLAW_AGENTS_DEFAULTS_ALLOW_READ_OUTSIDE_WORKSPACE"`
	PluginsDir                string         `json:"plugins_dir" yaml:"plugins_dir"                     env:"GEEKCLAW_AGENTS_DEFAULTS_PLUGINS_DIR"`
	Provider                  string         `json:"provider" yaml:"provider"                        env:"GEEKCLAW_AGENTS_DEFAULTS_PROVIDER"`
	ModelName                 string         `json:"model_name,omitempty" yaml:"model_name,omitempty"            env:"GEEKCLAW_AGENTS_DEFAULTS_MODEL_NAME"`
	Model                     string         `json:"model" yaml:"model"                           env:"GEEKCLAW_AGENTS_DEFAULTS_MODEL"` // 已弃用：请使用 model_name
	ModelFallbacks            []string       `json:"model_fallbacks,omitempty" yaml:"model_fallbacks,omitempty"`
	ImageModel                string         `json:"image_model,omitempty" yaml:"image_model,omitempty"           env:"GEEKCLAW_AGENTS_DEFAULTS_IMAGE_MODEL"`
	ImageModelFallbacks       []string       `json:"image_model_fallbacks,omitempty" yaml:"image_model_fallbacks,omitempty"`
	MaxTokens                 int            `json:"max_tokens" yaml:"max_tokens"                      env:"GEEKCLAW_AGENTS_DEFAULTS_MAX_TOKENS"`
	Temperature               *float64       `json:"temperature,omitempty" yaml:"temperature,omitempty"           env:"GEEKCLAW_AGENTS_DEFAULTS_TEMPERATURE"`
	MaxToolIterations         int            `json:"max_tool_iterations" yaml:"max_tool_iterations"             env:"GEEKCLAW_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS"`
	SummarizeMessageThreshold int            `json:"summarize_message_threshold" yaml:"summarize_message_threshold"     env:"GEEKCLAW_AGENTS_DEFAULTS_SUMMARIZE_MESSAGE_THRESHOLD"`
	SummarizeTokenPercent     int            `json:"summarize_token_percent" yaml:"summarize_token_percent"         env:"GEEKCLAW_AGENTS_DEFAULTS_SUMMARIZE_TOKEN_PERCENT"`
	MaxMediaSize              int            `json:"max_media_size,omitempty" yaml:"max_media_size,omitempty"        env:"GEEKCLAW_AGENTS_DEFAULTS_MAX_MEDIA_SIZE"`
}

// UnmarshalYAML 实现 AgentDefaults 的自定义 YAML 反序列化。
// 兼容旧配置：如果存在旧字段 restrict_to_plugins_dir，映射到 RestrictToWorkspace。
func (d *AgentDefaults) UnmarshalYAML(value *yaml.Node) error {
	// 收集用户实际写入的键。
	keys := make(map[string]bool)
	if value.Kind == yaml.MappingNode {
		for i := 0; i < len(value.Content)-1; i += 2 {
			keys[value.Content[i].Value] = true
		}
	}

	// 使用别名避免无限递归，然后正常解码。
	type Alias AgentDefaults
	var alias Alias
	if err := value.Decode(&alias); err != nil {
		return err
	}
	*d = AgentDefaults(alias)

	// 旧字段 "restrict_to_plugins_dir" 在 restrict_to_workspace 未显式设置时
	// 填充 RestrictToWorkspace。
	if !keys["restrict_to_workspace"] && keys["restrict_to_plugins_dir"] {
		for i := 0; i < len(value.Content)-1; i += 2 {
			if value.Content[i].Value == "restrict_to_plugins_dir" {
				var b bool
				if err := value.Content[i+1].Decode(&b); err == nil {
					d.RestrictToWorkspace = b
				}
				break
			}
		}
	}

	return nil
}

// DefaultMaxMediaSize 是媒体文件的默认最大大小（20 MB）。
const DefaultMaxMediaSize = 20 * 1024 * 1024 // 20 MB

// GetMaxMediaSize 返回有效的最大媒体文件大小。
func (d *AgentDefaults) GetMaxMediaSize() int {
	if d.MaxMediaSize > 0 {
		return d.MaxMediaSize
	}
	return DefaultMaxMediaSize
}

// GetModelName 返回代理默认配置中有效的模型名称。
// 优先使用新的 "model_name" 字段，回退到 "model" 以保持向后兼容。
func (d *AgentDefaults) GetModelName() string {
	if d.ModelName != "" {
		return d.ModelName
	}
	return d.Model
}

// ChannelsConfig 定义渠道配置。
type ChannelsConfig struct {
	// External 定义作为独立进程运行的外部渠道（通过标准输入输出的 JSON-RPC）。
	// 映射键是用于路由的渠道名称。
	External map[string]ExternalChannelConfig `json:"external,omitempty" yaml:"external,omitempty"`
}

// CommandsConfig 保存命令系统的配置，包括外部插件。
type CommandsConfig struct {
	// Plugins 将插件名称映射到其配置。
	// 每个插件作为独立进程运行（通过标准输入输出的 JSON-RPC），
	// 并向命令注册表贡献命令定义。
	Plugins map[string]CommandPluginConfig `json:"plugins,omitempty" yaml:"plugins,omitempty"`
}

// CommandPluginConfig 定义外部命令插件进程。
type CommandPluginConfig struct {
	Enabled bool              `json:"enabled" yaml:"enabled"`
	Command string            `json:"command" yaml:"command"`         // 要运行的可执行文件
	Args    []string          `json:"args,omitempty" yaml:"args,omitempty"`  // 命令参数
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`   // 环境变量
	Config  map[string]any    `json:"config,omitempty" yaml:"config,omitempty"` // 初始化时传递给插件
}

// GroupTriggerConfig 控制机器人在群聊中何时响应。
type GroupTriggerConfig struct {
	MentionOnly bool     `json:"mention_only,omitempty" yaml:"mention_only,omitempty"`
	Prefixes    []string `json:"prefixes,omitempty" yaml:"prefixes,omitempty"`
}

// ExternalChannelConfig 定义作为独立进程运行的渠道，
// 通过标准输入输出的 JSON-RPC 与 geekclaw 通信。
type ExternalChannelConfig struct {
	Enabled            bool              `json:"enabled" yaml:"enabled"`
	Command            string            `json:"command" yaml:"command"`                      // 要运行的可执行文件
	Args               []string          `json:"args,omitempty" yaml:"args,omitempty"`               // 命令参数
	Env                map[string]string `json:"env,omitempty" yaml:"env,omitempty"`                // 环境变量
	AllowFrom          FlexibleStringSlice `json:"allow_from,omitempty" yaml:"allow_from,omitempty"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty" yaml:"group_trigger,omitempty"`
	ReasoningChannelID string            `json:"reasoning_channel_id,omitempty" yaml:"reasoning_channel_id,omitempty"`
}

// VoiceConfig 定义语音功能的配置。
type VoiceConfig struct {
	EchoTranscription bool `json:"echo_transcription" yaml:"echo_transcription" env:"GEEKCLAW_VOICE_ECHO_TRANSCRIPTION"`
	// Plugins 将插件名称映射到其配置。
	// 每个插件作为独立进程运行（通过标准输入输出的 JSON-RPC），
	// 并提供 Transcriber 实现。
	Plugins map[string]VoicePluginConfig `json:"plugins,omitempty" yaml:"plugins,omitempty"`
}

// VoicePluginConfig 定义外部语音转录插件进程。
type VoicePluginConfig struct {
	Enabled bool              `json:"enabled" yaml:"enabled"`
	Command string            `json:"command" yaml:"command"`         // 要运行的可执行文件
	Args    []string          `json:"args,omitempty" yaml:"args,omitempty"`  // 命令参数
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`   // 环境变量
	Config  map[string]any    `json:"config,omitempty" yaml:"config,omitempty"` // 初始化时传递给插件
}

// AuthConfig 保存认证配置。
type AuthConfig struct {
	// Plugins 将提供者名称映射到外部认证插件配置。
	// 每个插件处理特定提供者的认证。
	Plugins map[string]AuthPluginConfig `json:"plugins,omitempty" yaml:"plugins,omitempty"`
	// DefaultMethod 指定未明确指定时使用的默认认证方法。
	DefaultMethod string `json:"default_method,omitempty" yaml:"default_method,omitempty" env:"GEEKCLAW_AUTH_DEFAULT_METHOD"`
}

// ProvidersConfig 定义所有 AI 提供者的配置。
type ProvidersConfig struct {
	Anthropic     ProviderConfig       `json:"anthropic" yaml:"anthropic"`
	OpenAI        OpenAIProviderConfig `json:"openai" yaml:"openai"`
	LiteLLM       ProviderConfig       `json:"litellm" yaml:"litellm"`
	OpenRouter    ProviderConfig       `json:"openrouter" yaml:"openrouter"`
	Groq          ProviderConfig       `json:"groq" yaml:"groq"`
	Zhipu         ProviderConfig       `json:"zhipu" yaml:"zhipu"`
	VLLM          ProviderConfig       `json:"vllm" yaml:"vllm"`
	Gemini        ProviderConfig       `json:"gemini" yaml:"gemini"`
	Nvidia        ProviderConfig       `json:"nvidia" yaml:"nvidia"`
	Ollama        ProviderConfig       `json:"ollama" yaml:"ollama"`
	Moonshot      ProviderConfig       `json:"moonshot" yaml:"moonshot"`
	ShengSuanYun  ProviderConfig       `json:"shengsuanyun" yaml:"shengsuanyun"`
	DeepSeek      ProviderConfig       `json:"deepseek" yaml:"deepseek"`
	Cerebras      ProviderConfig       `json:"cerebras" yaml:"cerebras"`
	Vivgrid       ProviderConfig       `json:"vivgrid" yaml:"vivgrid"`
	VolcEngine    ProviderConfig       `json:"volcengine" yaml:"volcengine"`
	Antigravity   ProviderConfig       `json:"antigravity" yaml:"antigravity"`
	Qwen          ProviderConfig       `json:"qwen" yaml:"qwen"`
	Mistral       ProviderConfig       `json:"mistral" yaml:"mistral"`
	Avian         ProviderConfig       `json:"avian" yaml:"avian"`
	Minimax       ProviderConfig       `json:"minimax" yaml:"minimax"`
}

// IsEmpty 检查所有提供者配置是否为空（未设置 API 密钥或 API 基础 URL）。
// 注意：WebSearch 是优化选项，不计入"非空"判断。
func (p ProvidersConfig) IsEmpty() bool {
	return p.Anthropic.APIKey == "" && p.Anthropic.APIBase == "" &&
		p.OpenAI.APIKey == "" && p.OpenAI.APIBase == "" &&
		p.LiteLLM.APIKey == "" && p.LiteLLM.APIBase == "" &&
		p.OpenRouter.APIKey == "" && p.OpenRouter.APIBase == "" &&
		p.Groq.APIKey == "" && p.Groq.APIBase == "" &&
		p.Zhipu.APIKey == "" && p.Zhipu.APIBase == "" &&
		p.VLLM.APIKey == "" && p.VLLM.APIBase == "" &&
		p.Gemini.APIKey == "" && p.Gemini.APIBase == "" &&
		p.Nvidia.APIKey == "" && p.Nvidia.APIBase == "" &&
		p.Ollama.APIKey == "" && p.Ollama.APIBase == "" &&
		p.Moonshot.APIKey == "" && p.Moonshot.APIBase == "" &&
		p.ShengSuanYun.APIKey == "" && p.ShengSuanYun.APIBase == "" &&
		p.DeepSeek.APIKey == "" && p.DeepSeek.APIBase == "" &&
		p.Cerebras.APIKey == "" && p.Cerebras.APIBase == "" &&
		p.Vivgrid.APIKey == "" && p.Vivgrid.APIBase == "" &&
		p.VolcEngine.APIKey == "" && p.VolcEngine.APIBase == "" &&
		p.Antigravity.APIKey == "" && p.Antigravity.APIBase == "" &&
		p.Qwen.APIKey == "" && p.Qwen.APIBase == "" &&
		p.Mistral.APIKey == "" && p.Mistral.APIBase == "" &&
		p.Avian.APIKey == "" && p.Avian.APIBase == "" &&
		p.Minimax.APIKey == "" && p.Minimax.APIBase == ""
}

// MarshalYAML 实现 ProvidersConfig 的自定义 YAML 序列化，
// 在为空时省略整个部分。
func (p ProvidersConfig) MarshalYAML() (interface{}, error) {
	if p.IsEmpty() {
		return nil, nil
	}
	type Alias ProvidersConfig
	return (*Alias)(&p), nil
}

// ProviderConfig 定义单个 AI 提供者的配置。
type ProviderConfig struct {
	APIKey         string `json:"api_key" yaml:"api_key"                   env:"GEEKCLAW_PROVIDERS_{{.Name}}_API_KEY"`
	APIBase        string `json:"api_base" yaml:"api_base"                  env:"GEEKCLAW_PROVIDERS_{{.Name}}_API_BASE"`
	Proxy          string `json:"proxy,omitempty" yaml:"proxy,omitempty"           env:"GEEKCLAW_PROVIDERS_{{.Name}}_PROXY"`
	RequestTimeout int    `json:"request_timeout,omitempty" yaml:"request_timeout,omitempty" env:"GEEKCLAW_PROVIDERS_{{.Name}}_REQUEST_TIMEOUT"`
	AuthMethod     string `json:"auth_method,omitempty" yaml:"auth_method,omitempty"     env:"GEEKCLAW_PROVIDERS_{{.Name}}_AUTH_METHOD"`
	ConnectMode    string `json:"connect_mode,omitempty" yaml:"connect_mode,omitempty"    env:"GEEKCLAW_PROVIDERS_{{.Name}}_CONNECT_MODE"`
}

// OpenAIProviderConfig 扩展 ProviderConfig，增加 OpenAI 特定的配置选项。
type OpenAIProviderConfig struct {
	ProviderConfig
	WebSearch bool `json:"web_search" yaml:"web_search" env:"GEEKCLAW_PROVIDERS_OPENAI_WEB_SEARCH"`
}

// ModelConfig 表示以模型为中心的提供者配置。
// 它允许仅通过配置添加新的提供者（尤其是 OpenAI 兼容的提供者）。
// model 字段使用协议前缀格式：[protocol/]model-identifier
// 支持的协议：openai、anthropic、antigravity、claude-cli、codex-cli、external
// 如果未指定前缀，默认协议为 "openai"。
type ModelConfig struct {
	// 必填字段
	ModelName string `json:"model_name" yaml:"model_name"` // 面向用户的模型别名
	Model     string `json:"model" yaml:"model"`      // 协议/模型标识符（例如 "openai/gpt-4o"、"anthropic/claude-sonnet-4.6"）

	// 基于 HTTP 的提供者
	APIBase string `json:"api_base,omitempty" yaml:"api_base,omitempty"` // API 端点 URL
	APIKey  string `json:"api_key" yaml:"api_key"`            // API 认证密钥
	Proxy   string `json:"proxy,omitempty" yaml:"proxy,omitempty"`    // HTTP 代理 URL

	// 特殊提供者（基于 CLI、OAuth 等）
	AuthMethod  string `json:"auth_method,omitempty" yaml:"auth_method,omitempty"`  // 认证方法：oauth、token
	ConnectMode string `json:"connect_mode,omitempty" yaml:"connect_mode,omitempty"` // 连接模式：stdio、grpc
	PluginsDir  string `json:"plugins_dir,omitempty" yaml:"plugins_dir,omitempty"`  // 基于 CLI 的提供者的插件目录路径

	// 可选优化
	RPM            int    `json:"rpm,omitempty" yaml:"rpm,omitempty"`              // 每分钟请求限制
	MaxTokensField string `json:"max_tokens_field,omitempty" yaml:"max_tokens_field,omitempty"` // 最大令牌数的字段名（例如 "max_completion_tokens"）
	RequestTimeout int    `json:"request_timeout,omitempty" yaml:"request_timeout,omitempty"`
	ThinkingLevel  string `json:"thinking_level,omitempty" yaml:"thinking_level,omitempty"` // 扩展思考：off|low|medium|high|xhigh|adaptive

	// 外部插件提供者（协议："external"）
	PluginCommand string            `json:"plugin_command,omitempty" yaml:"plugin_command,omitempty"` // 要运行的可执行文件
	PluginArgs    []string          `json:"plugin_args,omitempty" yaml:"plugin_args,omitempty"`   // 命令参数
	PluginEnv     map[string]string `json:"plugin_env,omitempty" yaml:"plugin_env,omitempty"`    // 环境变量
	PluginConfig  map[string]any    `json:"plugin_config,omitempty" yaml:"plugin_config,omitempty"` // 初始化时传递给插件
}

// GatewayConfig 定义网关服务器的配置。
type GatewayConfig struct {
	Host string `json:"host" yaml:"host" env:"GEEKCLAW_GATEWAY_HOST"`
	Port int    `json:"port" yaml:"port" env:"GEEKCLAW_GATEWAY_PORT"`
}

// ToolDiscoveryConfig 定义工具发现功能的配置。
type ToolDiscoveryConfig struct {
	Enabled          bool `json:"enabled" yaml:"enabled"            env:"GEEKCLAW_TOOLS_DISCOVERY_ENABLED"`
	TTL              int  `json:"ttl" yaml:"ttl"                env:"GEEKCLAW_TOOLS_DISCOVERY_TTL"`
	MaxSearchResults int  `json:"max_search_results" yaml:"max_search_results" env:"GEEKCLAW_MAX_SEARCH_RESULTS"`
	UseBM25          bool `json:"use_bm25" yaml:"use_bm25"           env:"GEEKCLAW_TOOLS_DISCOVERY_USE_BM25"`
	UseRegex         bool `json:"use_regex" yaml:"use_regex"          env:"GEEKCLAW_TOOLS_DISCOVERY_USE_REGEX"`
}

// ToolConfig 定义单个工具的基本启用/禁用配置。
type ToolConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled" env:"ENABLED"`
}

// WebToolsConfig 定义网络工具的配置。
type WebToolsConfig struct {
	ToolConfig `envPrefix:"GEEKCLAW_TOOLS_WEB_"`
	// Plugins 将插件名称映射到其配置。
	// 每个插件作为独立进程运行（通过标准输入输出的 JSON-RPC），
	// 并提供 SearchProvider 实现。
	Plugins map[string]WebSearchPluginConfig `json:"plugins,omitempty" yaml:"plugins,omitempty"`
	// Proxy 是网络工具的可选代理 URL（http/https/socks5/socks5h）。
	// 对于需要认证的代理，建议使用 HTTP_PROXY/HTTPS_PROXY 环境变量，而非在配置中嵌入凭据。
	Proxy           string `json:"proxy,omitempty" yaml:"proxy,omitempty"             env:"GEEKCLAW_TOOLS_WEB_PROXY"`
	FetchLimitBytes int64  `json:"fetch_limit_bytes,omitempty" yaml:"fetch_limit_bytes,omitempty" env:"GEEKCLAW_TOOLS_WEB_FETCH_LIMIT_BYTES"`
}

// WebSearchPluginConfig 定义外部搜索插件进程。
type WebSearchPluginConfig struct {
	Enabled    bool              `json:"enabled" yaml:"enabled"`
	Command    string            `json:"command" yaml:"command"`          // 要运行的可执行文件
	Args       []string          `json:"args,omitempty" yaml:"args,omitempty"`   // 命令参数
	Env        map[string]string `json:"env,omitempty" yaml:"env,omitempty"`    // 环境变量
	Config     map[string]any    `json:"config,omitempty" yaml:"config,omitempty"` // 初始化时传递给插件
	MaxResults int               `json:"max_results,omitempty" yaml:"max_results,omitempty"`
}

// ToolPluginConfig 定义外部工具插件进程。
// 每个插件作为独立进程运行（通过标准输入输出的 JSON-RPC），
// 并声明一个或多个工具定义注册到代理的工具注册表中。
type ToolPluginConfig struct {
	Enabled bool              `json:"enabled" yaml:"enabled"`
	Command string            `json:"command" yaml:"command"`          // 要运行的可执行文件
	Args    []string          `json:"args,omitempty" yaml:"args,omitempty"`   // 命令参数
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`    // 环境变量
	Config  map[string]any    `json:"config,omitempty" yaml:"config,omitempty"` // 初始化时传递给插件
}

// AuthPluginConfig 定义外部认证插件进程。
// 每个插件作为独立进程运行（通过标准输入输出的 JSON-RPC），
// 并处理特定提供者的认证流程（OAuth、API 密钥、令牌）。
type AuthPluginConfig struct {
	Enabled bool              `json:"enabled" yaml:"enabled"`
	Command string            `json:"command" yaml:"command"`          // 要运行的可执行文件
	Args    []string          `json:"args,omitempty" yaml:"args,omitempty"`   // 命令参数
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`    // 环境变量
	Config  map[string]any    `json:"config,omitempty" yaml:"config,omitempty"` // 初始化时传递给插件
}

// CronToolsConfig 定义定时任务工具的配置。
type CronToolsConfig struct {
	ToolConfig         `    envPrefix:"GEEKCLAW_TOOLS_CRON_"`
	ExecTimeoutMinutes int `                                 env:"GEEKCLAW_TOOLS_CRON_EXEC_TIMEOUT_MINUTES" json:"exec_timeout_minutes" yaml:"exec_timeout_minutes"` // 0 表示无超时
}

// ExecConfig 定义命令执行工具的配置。
type ExecConfig struct {
	ToolConfig          `         envPrefix:"GEEKCLAW_TOOLS_EXEC_"`
	AllowRemote         bool     `                                 env:"GEEKCLAW_TOOLS_EXEC_ALLOW_REMOTE"          json:"allow_remote" yaml:"allow_remote"`
	CustomDenyPatterns  []string `                                 env:"GEEKCLAW_TOOLS_EXEC_CUSTOM_DENY_PATTERNS"  json:"custom_deny_patterns" yaml:"custom_deny_patterns"`
	CustomAllowPatterns []string `                                 env:"GEEKCLAW_TOOLS_EXEC_CUSTOM_ALLOW_PATTERNS" json:"custom_allow_patterns" yaml:"custom_allow_patterns"`
	TimeoutSeconds      int      `                                 env:"GEEKCLAW_TOOLS_EXEC_TIMEOUT_SECONDS" json:"timeout_seconds" yaml:"timeout_seconds"` // 0 表示使用默认值（60 秒）
	// ExecAdmins 列出被授予不受限制的 shell 执行权限的发送者身份
	// （与渠道 allow_from 格式相同），绕过所有命令防护。
	ExecAdmins []string `env:"GEEKCLAW_TOOLS_EXEC_ADMINS" json:"exec_admins" yaml:"exec_admins"`
}

// SkillsToolsConfig 定义技能工具的配置。
type SkillsToolsConfig struct {
	ToolConfig            `                       envPrefix:"GEEKCLAW_TOOLS_SKILLS_"`
	Registries            SkillsRegistriesConfig `                                   json:"registries" yaml:"registries"`
	MaxConcurrentSearches int                    `                                   json:"max_concurrent_searches" yaml:"max_concurrent_searches" env:"GEEKCLAW_TOOLS_SKILLS_MAX_CONCURRENT_SEARCHES"`
}

// MediaCleanupConfig 定义媒体清理工具的配置。
type MediaCleanupConfig struct {
	ToolConfig `    envPrefix:"GEEKCLAW_MEDIA_CLEANUP_"`
	MaxAge     int `                                    env:"GEEKCLAW_MEDIA_CLEANUP_MAX_AGE"  json:"max_age_minutes" yaml:"max_age_minutes"`
	Interval   int `                                    env:"GEEKCLAW_MEDIA_CLEANUP_INTERVAL" json:"interval_minutes" yaml:"interval_minutes"`
}

// ReadFileToolConfig 定义文件读取工具的配置。
type ReadFileToolConfig struct {
	Enabled         bool `json:"enabled" yaml:"enabled"`
	MaxReadFileSize int  `json:"max_read_file_size" yaml:"max_read_file_size"`
}

// ToolsConfig 定义所有工具的配置。
type ToolsConfig struct {
	AllowReadPaths  []string           `json:"allow_read_paths" yaml:"allow_read_paths"  env:"GEEKCLAW_TOOLS_ALLOW_READ_PATHS"`
	AllowWritePaths []string           `json:"allow_write_paths" yaml:"allow_write_paths" env:"GEEKCLAW_TOOLS_ALLOW_WRITE_PATHS"`
	Web             WebToolsConfig     `json:"web" yaml:"web"`
	Cron            CronToolsConfig    `json:"cron" yaml:"cron"`
	Exec            ExecConfig         `json:"exec" yaml:"exec"`
	Skills          SkillsToolsConfig  `json:"skills" yaml:"skills"`
	MediaCleanup    MediaCleanupConfig `json:"media_cleanup" yaml:"media_cleanup"`
	MCP             MCPConfig          `json:"mcp" yaml:"mcp"`
	// Plugins 将插件名称映射到其配置。
	// 每个插件作为独立进程运行（通过标准输入输出的 JSON-RPC），
	// 并向代理的工具注册表贡献工具定义。
	Plugins      map[string]ToolPluginConfig `json:"plugins,omitempty" yaml:"plugins,omitempty"`
	AppendFile   ToolConfig                  `json:"append_file" yaml:"append_file"   envPrefix:"GEEKCLAW_TOOLS_APPEND_FILE_"`
	EditFile     ToolConfig                  `json:"edit_file" yaml:"edit_file"     envPrefix:"GEEKCLAW_TOOLS_EDIT_FILE_"`
	FindSkills   ToolConfig                  `json:"find_skills" yaml:"find_skills"   envPrefix:"GEEKCLAW_TOOLS_FIND_SKILLS_"`
	InstallSkill ToolConfig                  `json:"install_skill" yaml:"install_skill" envPrefix:"GEEKCLAW_TOOLS_INSTALL_SKILL_"`
	ListDir      ToolConfig                  `json:"list_dir" yaml:"list_dir"      envPrefix:"GEEKCLAW_TOOLS_LIST_DIR_"`
	Message      ToolConfig                  `json:"message" yaml:"message"       envPrefix:"GEEKCLAW_TOOLS_MESSAGE_"`
	ReadFile     ReadFileToolConfig          `json:"read_file" yaml:"read_file"     envPrefix:"GEEKCLAW_TOOLS_READ_FILE_"`
	SendFile     ToolConfig                  `json:"send_file" yaml:"send_file"     envPrefix:"GEEKCLAW_TOOLS_SEND_FILE_"`
	Spawn        ToolConfig                  `json:"spawn" yaml:"spawn"         envPrefix:"GEEKCLAW_TOOLS_SPAWN_"`
	Subagent     ToolConfig                  `json:"subagent" yaml:"subagent"      envPrefix:"GEEKCLAW_TOOLS_SUBAGENT_"`
	WebFetch     ToolConfig                  `json:"web_fetch" yaml:"web_fetch"     envPrefix:"GEEKCLAW_TOOLS_WEB_FETCH_"`
	WriteFile    ToolConfig                  `json:"write_file" yaml:"write_file"    envPrefix:"GEEKCLAW_TOOLS_WRITE_FILE_"`
}


// SkillsRegistriesConfig 定义技能注册表的配置。
type SkillsRegistriesConfig struct {
	ClawHub ClawHubRegistryConfig `json:"clawhub" yaml:"clawhub"`
}

// ClawHubRegistryConfig 定义 ClawHub 技能注册表的配置。
type ClawHubRegistryConfig struct {
	Enabled         bool   `json:"enabled" yaml:"enabled"           env:"GEEKCLAW_SKILLS_REGISTRIES_CLAWHUB_ENABLED"`
	BaseURL         string `json:"base_url" yaml:"base_url"          env:"GEEKCLAW_SKILLS_REGISTRIES_CLAWHUB_BASE_URL"`
	AuthToken       string `json:"auth_token" yaml:"auth_token"        env:"GEEKCLAW_SKILLS_REGISTRIES_CLAWHUB_AUTH_TOKEN"`
	SearchPath      string `json:"search_path" yaml:"search_path"       env:"GEEKCLAW_SKILLS_REGISTRIES_CLAWHUB_SEARCH_PATH"`
	SkillsPath      string `json:"skills_path" yaml:"skills_path"       env:"GEEKCLAW_SKILLS_REGISTRIES_CLAWHUB_SKILLS_PATH"`
	DownloadPath    string `json:"download_path" yaml:"download_path"     env:"GEEKCLAW_SKILLS_REGISTRIES_CLAWHUB_DOWNLOAD_PATH"`
	Timeout         int    `json:"timeout" yaml:"timeout"           env:"GEEKCLAW_SKILLS_REGISTRIES_CLAWHUB_TIMEOUT"`
	MaxZipSize      int    `json:"max_zip_size" yaml:"max_zip_size"      env:"GEEKCLAW_SKILLS_REGISTRIES_CLAWHUB_MAX_ZIP_SIZE"`
	MaxResponseSize int    `json:"max_response_size" yaml:"max_response_size" env:"GEEKCLAW_SKILLS_REGISTRIES_CLAWHUB_MAX_RESPONSE_SIZE"`
}

// MCPServerConfig 定义单个 MCP 服务器的配置。
type MCPServerConfig struct {
	// Enabled 指示该 MCP 服务器是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`
	// Command 是要运行的可执行文件（例如 "npx"、"python"、"/path/to/server"）
	Command string `json:"command" yaml:"command"`
	// Args 是传递给命令的参数
	Args []string `json:"args,omitempty" yaml:"args,omitempty"`
	// Env 是为服务器进程设置的环境变量（仅 stdio 模式）
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	// EnvFile 是包含环境变量的文件路径（仅 stdio 模式）
	EnvFile string `json:"env_file,omitempty" yaml:"env_file,omitempty"`
	// Type 为 "stdio"、"sse" 或 "http"（默认：设置了 command 时为 stdio，设置了 url 时为 sse）
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
	// URL 用于 SSE/HTTP 传输
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
	// Headers 是随请求发送的 HTTP 头（仅 sse/http 模式）
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

// MCPConfig 定义所有 MCP 服务器的配置。
type MCPConfig struct {
	ToolConfig `                    envPrefix:"GEEKCLAW_TOOLS_MCP_"`
	Discovery  ToolDiscoveryConfig `                                json:"discovery" yaml:"discovery"`
	// Servers 是服务器名称到服务器配置的映射
	Servers map[string]MCPServerConfig `json:"servers,omitempty" yaml:"servers,omitempty"`
}

// LoadConfig 从指定路径加载配置文件。
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	// 预扫描检查用户提供了多少个 model_list 条目。
	// YAML 解码器会复用现有切片的后备数组元素而不是零初始化它们，
	// 因此用户 YAML 中缺失的字段（例如 api_base）会静默继承
	// DefaultConfig 模板中相同索引位置的值。
	// 仅当用户实际提供了条目时才重置 cfg.ModelList；
	// 当数量为 0 时保留 DefaultConfig 的内置列表作为回退。
	var tmp struct {
		ModelList []struct{} `yaml:"model_list"`
	}
	if err := yaml.Unmarshal(data, &tmp); err != nil {
		return nil, fmt.Errorf("parsing YAML config: %w", err)
	}
	if len(tmp.ModelList) > 0 {
		cfg.ModelList = nil
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing YAML config: %w", err)
	}

	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	// 自动迁移：如果仅存在旧版 providers 配置，则转换为 model_list
	if len(cfg.ModelList) == 0 && cfg.HasProvidersConfig() {
		cfg.ModelList = ConvertProvidersToModelList(cfg)
	}

	// 验证 model_list 的唯一性和必填字段
	if err := cfg.ValidateModelList(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SaveConfig 将配置保存到指定路径。
func SaveConfig(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	// 使用统一的原子写入工具，并显式同步以确保闪存存储的可靠性。
	return fileutil.WriteFileAtomic(path, data, 0o600)
}

// PluginsPath 返回插件目录的绝对路径。
func (c *Config) PluginsPath() string {
	return expandHome(c.Agents.Defaults.PluginsDir)
}

// WorkspacePath 返回工作目录的绝对路径。
func (c *Config) WorkspacePath() string {
	return expandHome(c.Agents.Defaults.Workspace)
}

// ConfigsPath 返回配置目录的绝对路径。
// 基于 GEEKCLAW_HOME 推导，而非从 PluginsDir 反推。
func (c *Config) ConfigsPath() string {
	return filepath.Join(geekclawHomeDir(), "configs")
}

// LogsPath 返回日志目录的绝对路径。
// 基于 GEEKCLAW_HOME 推导，而非从 PluginsDir 反推。
func (c *Config) LogsPath() string {
	return filepath.Join(geekclawHomeDir(), "logs")
}

// geekclawHomeDir 返回 GEEKCLAW_HOME 的绝对路径。
// 优先级：$GEEKCLAW_HOME 环境变量 > ~/.geekclaw
func geekclawHomeDir() string {
	if home := GeekclawHome(); home != "" {
		return home
	}
	userHome, _ := os.UserHomeDir()
	return filepath.Join(userHome, ".geekclaw")
}

// expandHome 展开路径中的 ~ 为用户主目录。
func expandHome(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		if len(path) > 1 && path[1] == '/' {
			return home + path[1:]
		}
		return home
	}
	return path
}

// IsToolEnabled 检查指定名称的工具是否已启用。
func (t *ToolsConfig) IsToolEnabled(name string) bool {
	switch name {
	case "web":
		return t.Web.Enabled
	case "cron":
		return t.Cron.Enabled
	case "exec":
		return t.Exec.Enabled
	case "skills":
		return t.Skills.Enabled
	case "media_cleanup":
		return t.MediaCleanup.Enabled
	case "append_file":
		return t.AppendFile.Enabled
	case "edit_file":
		return t.EditFile.Enabled
	case "find_skills":
		return t.FindSkills.Enabled
	case "install_skill":
		return t.InstallSkill.Enabled
	case "list_dir":
		return t.ListDir.Enabled
	case "message":
		return t.Message.Enabled
	case "read_file":
		return t.ReadFile.Enabled
	case "spawn":
		return t.Spawn.Enabled
	case "subagent":
		return t.Subagent.Enabled
	case "web_fetch":
		return t.WebFetch.Enabled
	case "send_file":
		return t.SendFile.Enabled
	case "write_file":
		return t.WriteFile.Enabled
	case "mcp":
		return t.MCP.Enabled
	default:
		return true
	}
}
