# GeekClaw: 非核心模块记录

本文档记录在 Go → Rust 迁移中被移除的非核心模块。
这些模块可在最小核心稳定后按需重新实现。

## 移除模块列表

### channels/ — 渠道系统（5199 行）
- **功能**：14+ 渠道（Telegram、Discord、Slack 等）的抽象层
- **包含**：ChannelManager、速率限制器、消息分片、Webhook、外部渠道插件桥接
- **依赖**：bus、config、plugin
- **重新实现建议**：作为独立 crate `geekclaw-channels`，每个渠道一个 feature flag

### commands/ — 斜杠命令（2467 行）
- **功能**：`/help`、`/status`、`/cron` 等命令的注册与分发
- **包含**：CommandRegistry、外部命令插件桥接
- **重新实现建议**：可合并到 agent 模块作为消息预处理器

### internal/ — CLI 子命令（2541 行）
- **功能**：`geekclaw agent`、`geekclaw gateway`、`geekclaw cron`、`geekclaw skills` 等 CLI 入口
- **包含**：交互式 shell、状态查询、技能管理
- **重新实现建议**：最小核心只需 `agent` 和 `version` 子命令，其余按需添加

### skills/ — 技能系统（2022 行）
- **功能**：Markdown 技能文件加载、ClawHub 技能仓库注册
- **包含**：SkillsLoader、BM25 搜索、技能安装/搜索/移除
- **重新实现建议**：可作为 agent context 的可选扩展

### routing/ — 多 Agent 路由（1116 行）
- **功能**：多 Agent 绑定、按 channel+chatID 路由到不同 agent 配置
- **重新实现建议**：最小核心只有单 agent，多 agent 是高级功能

### media/ — 媒体存储（802 行）
- **功能**：MediaStore 接口、文件生命周期管理、引用计数清理
- **重新实现建议**：需要多媒体支持时再加

### mcp/ — Model Context Protocol（841 行）
- **功能**：MCP 客户端、MCP 工具桥接
- **依赖**：`modelcontextprotocol/go-sdk`
- **重新实现建议**：等 Rust MCP SDK 成熟后接入

### interactive/ — 交互式确认（641 行）
- **功能**：工具执行前的用户确认流程
- **重新实现建议**：作为 agent loop 的可选中间件

### tasks/ — 任务队列（600 行）
- **功能**：AI 任务队列，支持取消操作
- **重新实现建议**：最小核心用 tokio task + CancellationToken 即可

### voice/ — 语音转录（309 行）
- **功能**：语音消息转文字抽象层
- **重新实现建议**：作为插件通过 JSON-RPC 接入

### health/ — 健康检查（188 行）
- **功能**：HTTP `/health` 端点
- **重新实现建议**：几行 axum 代码，需要时再加

### tools/ 具体实现 — 内置工具（~7000 行）
以下内置工具在最小核心中不包含，仅保留 Tool trait 和 ToolRegistry：

| 工具 | 文件 | 功能 |
|------|------|------|
| filesystem | `filesystem.go` (717 行) | 文件读写、目录操作 |
| shell | `shell.go` (433 行) | Shell 命令执行 |
| edit | `edit.go` | 文件编辑（diff-based） |
| web | `web.go` | Web 搜索/抓取 |
| cron | `cron.go` (351 行) | 定时任务管理工具 |
| search | `search_tool.go` | 语义搜索 |
| send_file | `send_file.go` | 文件发送 |
| spawn | `spawn.go` | 子进程启动 |
| subagent | `subagent.go` (366 行) | 子 Agent 调用 |
| mcp_tool | `mcp_tool.go` | MCP 工具桥接 |
| toolloop | `toolloop.go` | 工具循环执行 |

### utils/ — 工具函数（1210 行）
- **包含**：BM25 搜索、HTTP 重试、字符串工具、技能辅助函数
- **重新实现建议**：按需迁移到各 crate 内部

### session/ — 会话存储后端（300 行）
- **功能**：会话存储后端适配层
- **重新实现建议**：最小核心的 memory 模块已包含 JSONL 后端

### fileutil/ — 文件工具（305 行）
- **功能**：原子写入、安全路径处理
- **重新实现建议**：Rust 标准库 + `tempfile` crate 覆盖

## 总计移除

- **模块数**：15 个
- **代码量**：~35,000 行 Go 代码
- **保留核心**：~10,000 行 Go → 预估 ~6,000-8,000 行 Rust
