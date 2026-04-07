# geekclaw-config — 配置加载

## 模块概述

从 YAML 文件加载配置，支持 `GEEKCLAW_` 前缀环境变量覆盖和 `$ENV_VAR` 语法引用。提供类型安全的配置结构体和合理的默认值。

## 源码文件说明

| 文件 | 职责 |
|------|------|
| `src/lib.rs` | 配置结构体定义（`Config`、`AgentsConfig`、`AgentDefaults`、`ToolsConfig`、`ProviderConfig`）；`load()` 从文件加载；`load_default()` 纯默认值；`apply_env_overrides()` 环境变量覆盖；`resolve_env_refs()` 解析 `$VAR` 引用。包含 2 个单元测试 |
| `src/error.rs` | `ConfigError` 错误类型：IO 错误和 YAML 解析错误 |

## 核心类型

```rust
pub struct Config {
    pub agents: AgentsConfig,      // agent 默认参数
    pub tools: ToolsConfig,        // 工具配置（cron 超时等）
    pub providers: Vec<ProviderConfig>,  // LLM provider 列表
}

pub struct ProviderConfig {
    pub name: String,
    pub base_url: Option<String>,
    pub api_key: Option<String>,   // 支持 $ENV_VAR 语法
    pub model: Option<String>,
    pub fallback_models: Vec<String>,
}
```

## 环境变量覆盖

| 变量 | 覆盖字段 | 默认值 |
|------|----------|--------|
| `GEEKCLAW_MODEL` | `agents.defaults.model` | `gpt-4o` |
| `GEEKCLAW_MAX_TOOL_ITERATIONS` | `agents.defaults.max_tool_iterations` | `10` |
| `GEEKCLAW_SESSION_TIMEOUT` | `agents.defaults.session_timeout` | `300` |
| `GEEKCLAW_MAX_CONTEXT_TOKENS` | `agents.defaults.max_context_tokens` | `128000` |

## 设计决策

- 所有结构体实现 `Default`，即使没有配置文件也能运行
- Provider 的 `api_key` 支持 `$OPENAI_API_KEY` 语法，加载时自动替换为环境变量值
- 使用 `serde(default)` 确保缺少字段时不报错

## 不完善之处

- **无热重载**：配置只在启动时加载一次，运行中修改 config.yaml 不会生效
- **无配置校验**：不检查 provider 配置是否完整（如 api_key 为空不报错）
- **环境变量覆盖有限**：只覆盖了 4 个常用字段，Go 版本通过 `caarlos0/env` 支持全量覆盖
- **不支持 model_list**：Go 版本有 `model_list` 格式用于同名模型负载均衡，Rust 版简化为 `providers` 列表
