# geekclaw-tools — 工具系统

## 模块概述

定义工具接口（`Tool` trait）和注册表（`ToolRegistry`），以及三个内置工具：shell 命令执行、文件系统操作、定时任务管理。工具注册表支持并发执行多个工具调用，通过 Semaphore 控制并发上限。

## 源码文件说明

| 文件 | 职责 |
|------|------|
| `src/lib.rs` | `Tool` trait 定义、`ToolContext`（执行上下文）、`ToolResult`（执行结果）、`ToolCallRequest`（调用请求）、`parse_tool_calls()` 解析 LLM 响应中的工具调用 |
| `src/registry.rs` | `ToolRegistry`：工具注册/查找、生成 LLM API 的 ToolDefinition 列表、`execute_batch()` 并发执行。含 4 个测试 |
| `src/builtin/mod.rs` | 内置工具模块导出 |
| `src/builtin/shell.rs` | **Shell 工具**：命令执行 + 12 种危险命令拦截。含 7 个测试 |
| `src/builtin/filesystem.rs` | **文件系统工具**：read_file / write_file / list_dir。含 4 个测试 |
| `src/builtin/cron_tool.rs` | **定时任务工具**：通过 CronStore 进行 list/add/remove/enable/disable。含 4 个测试 |

## 核心 Trait

```rust
#[async_trait]
pub trait Tool: Send + Sync {
    fn name(&self) -> &str;
    fn description(&self) -> &str;
    fn parameters(&self) -> serde_json::Value;  // JSON Schema
    async fn execute(&self, args: Value, ctx: &ToolContext) -> ToolResult;
}
```

## 算法说明

### 并发执行（execute_batch）

```
execute_batch(calls, ctx, max_concurrent=10):
    semaphore = Semaphore(10)
    for call in calls:
        spawn task:
            acquire semaphore permit
            result = match registry.get(call.name):
                Some(tool) → spawn(tool.execute(args, ctx))  // 二层 spawn 做 panic recovery
                None → ToolResult::err("未找到工具")
    collect all results
```

二层 `tokio::spawn` 的目的：即使某个工具 panic，也不会影响其他工具的执行或导致整个 agent 崩溃。

### Shell 危险命令拦截

使用 12 个预编译的正则表达式匹配危险模式：

| 模式 | 拦截目标 |
|------|----------|
| `\brm\s+-[rf]{1,2}\b` | `rm -rf` / `rm -f` |
| `\b(shutdown\|reboot\|poweroff)\b` | 关机/重启 |
| `\bsudo\b` / `\bsu\b` | 权限提升 |
| `\bdd\s+if=` | 磁盘写入 |
| `\b(mkfs\|format)\b\s` | 格式化 |
| `:\(\)\s*\{.*\};\s*:` | Fork bomb |
| `\|\s*(sh\|bash)\b` | 管道到 shell |
| `\bcurl\b.*\|\s*(sh\|bash)` | 远程代码执行 |
| `\bchown\b` | 修改文件所有者 |
| `\bkill\s+-9\b` | 强制终止进程 |
| `\beval\b` | 动态代码执行 |

所有模式在 `LazyLock` 中预编译，只编译一次。

### 文件系统安全限制

- 读取大小上限 64KB，防止大文件撑爆 LLM 上下文
- 写入时自动创建父目录
- 路径解析：相对路径基于 `ToolContext.working_dir`

## 设计决策

- `Tool` trait 不依赖具体实现，外部插件也可以通过实现 trait 注册工具
- `ToolResult` 区分 `ok` 和 `err`，让 agent 知道工具执行是否成功
- `to_definition()` 有默认实现，工具只需定义 name/description/parameters
- CronTool 通过 `ToolContext.metadata["cron_store_path"]` 获取存储路径，保持 Tool trait 的无状态性

## 不完善之处

- **Shell 工具**：
  - 只支持 `bash`，Windows 上无法运行
  - 危险命令拦截是黑名单模式，无法防御所有变体（如 `r\m -rf`）
  - Go 版本支持 `allow_patterns`（白名单）、`restrict_to_workspace`（工作区限制）、`exec_admins`（管理员豁免），Rust 版均未实现
  - 不支持命令中的绝对路径检测和工作区沙盒
- **Filesystem 工具**：
  - 无工作区限制（Go 版本有 `ValidatePath` + 符号链接检测）
  - 无 append_file 操作
  - 无 edit_file 操作（Go 版本支持基于行号的 diff 编辑）
  - 二进制文件会读取失败但没有明确提示
- **CronTool**：
  - 每次调用都重新打开 CronStore，频繁调用时性能不佳
  - remove 只按完整 ID 匹配，不像 Go 版本支持前缀匹配
- **工具注册表**：
  - 无工具发现机制（Go 版本有 BM25 搜索，根据用户消息动态选择相关工具）
  - 无工具权限控制
  - 无工具 TTL（Go 版本支持临时工具自动过期）
