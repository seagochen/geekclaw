# GeekClaw 项目指南

## 项目概述

GeekClaw 是一个最小核心 AI Agent，用 Rust 编写。通过 JSON-RPC over stdio 与 Python 插件通信。每个 crate 有独立的 README，详细说明源码、算法和不完善之处。

## 技术栈

- **语言**: Rust（核心）、Python 3.12（插件）
- **异步运行时**: Tokio
- **通信**: JSON-RPC 2.0 over stdin/stdout
- **存储**: JSONL 文件（会话历史）、JSON 文件（定时任务）

## 构建与测试

```bash
cargo build              # 编译
cargo test               # 运行所有测试（92 tests）
cargo test -p geekclaw-agent --test integration  # 集成测试
cargo bench -p geekclaw-memory                   # 性能基准
cargo run -- agent       # 启动交互式 Agent
cargo run -- version     # 版本信息
./manage.py build        # release 编译 + 准备配置文件
```

## 项目结构

```
crates/
├── geekclaw/     # CLI 入口，组件串联
├── agent/        # 核心循环：消息 → LLM → 工具 → 响应
├── bus/          # 消息总线 (tokio mpsc)
├── config/       # YAML 配置 + GEEKCLAW_ 环境变量覆盖
├── cron/         # 定时任务 (cron/every/at 三种模式)
├── logger/       # tracing 日志初始化
├── memory/       # JSONL 会话持久化 (FNV 分片锁)
├── plugin/       # JSON-RPC 2.0 插件进程管理
├── providers/    # LLM trait + 故障转移 + 错误分类
└── tools/        # Tool trait + shell/filesystem/cron 内置工具
```

各 crate 详细文档见各自的 `README.md`。

## 编码规范

- 中文注释和日志
- 核心抽象用 trait（`LlmProvider`, `Tool`, `SessionStore`）
- 错误处理：`thiserror` 定义类型，`anyhow` 用于应用层
- 零 `unsafe` 代码
- 新增工具实现 `Tool` trait 并在 `main.rs` 中注册
- 新增 Provider 实现 `LlmProvider` trait

## 配置

主配置文件 `config.yaml`（从 `config.example.yaml` 复制）。支持环境变量：

| 变量 | 说明 |
|------|------|
| `GEEKCLAW_MODEL` | 默认模型 |
| `GEEKCLAW_MAX_TOOL_ITERATIONS` | 最大工具迭代 |
| `GEEKCLAW_SESSION_TIMEOUT` | 会话超时（秒） |
| `GEEKCLAW_MAX_CONTEXT_TOKENS` | 最大上下文 token |
| `GEEKCLAW_LOG` | 日志级别 |
| `GEEKCLAW_API_BASE` | LLM API 基础 URL |
| `OPENAI_API_KEY` | OpenAI API 密钥（也支持 ANTHROPIC_API_KEY、DEEPSEEK_API_KEY） |
