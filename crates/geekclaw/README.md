# geekclaw — CLI 入口

## 模块概述

可执行二进制的入口点。负责 CLI 参数解析、配置加载、LLM Provider 创建、组件初始化和交互式对话循环。是所有 crate 的最终组装点。

## 源码文件说明

| 文件 | 职责 |
|------|------|
| `src/main.rs` | 程序入口。包含：Banner 打印、日志初始化、clap CLI 定义（`agent` / `version` 子命令）、`create_provider()` Provider 工厂、`run_agent()` 交互式主循环 |

## 启动序列

```
main()
├── 打印 Banner（除非 GEEKCLAW_NO_BANNER=1）
├── geekclaw_logger::init()
├── 解析 CLI 参数
└── match command:
    ├── version → 打印版本号
    └── agent → run_agent(config_path):
        ├── 加载配置（config.yaml 或 load_default()）
        ├── create_provider():
        │   ├── 优先从 config.providers[0] 创建
        │   └── 备选从环境变量创建（OPENAI_API_KEY 等）
        ├── 创建 MessageBus
        ├── 创建 JSONLStore（.geekclaw/data/）
        ├── 创建 ToolRegistry + 注册内置工具（shell, filesystem）
        ├── 创建 AgentInstance + AgentLoop
        ├── 启动出站消息打印任务（后台 tokio task）
        └── 交互式 stdin 循环：
            读取一行 → agent.process_message() → 打印响应
            /quit 或 Ctrl+C 退出
```

## Provider 创建逻辑

```rust
fn create_provider(cfg) -> Result<Arc<dyn LlmProvider>>:
    1. 检查 cfg.providers[0]:
       有 → OpenAICompatProvider::new(base_url, api_key, model)

    2. 检查环境变量（按顺序）：
       OPENAI_API_KEY → ANTHROPIC_API_KEY → DEEPSEEK_API_KEY
       base_url 从 GEEKCLAW_API_BASE 获取（默认 openai）

    3. 都没有 → 返回错误
```

## 设计决策

- 交互式模式直接用 `tokio::io::stdin()` + `BufReader::lines()`，简单够用
- 出站消息打印在独立 task 中，避免阻塞 agent 处理
- Provider 创建支持两种方式（配置文件 / 环境变量），开发时用环境变量更方便
- 会话数据存储在 `.geekclaw/data/`，在 `.gitignore` 中忽略

## 不完善之处

- **无 readline 支持**：没有历史记录、Tab 补全、上下键等终端交互功能。Go 版本使用 `chzyer/readline`
- **无 gateway 模式**：Go 版本有 `gateway` 子命令启动 HTTP 服务 + 渠道（Telegram 等），Rust 版只有交互式 stdin
- **无 cron 子命令**：Go 版本有 `geekclaw cron list/add/remove` CLI 命令，Rust 版只能通过工具调用管理
- **无 skills 子命令**：Go 版本有技能安装/搜索 CLI
- **无 status 子命令**：Go 版本显示运行状态
- **Provider 选择简单**：只用第一个配置的 provider，不支持按模型名路由到不同 provider
- **无 graceful shutdown**：Ctrl+C 直接退出，没有等待正在进行的 LLM 调用完成
- **cron_tool 未注册**：main.rs 只注册了 shell 和 filesystem，cron_tool 需要手动添加
