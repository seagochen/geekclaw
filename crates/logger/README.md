# geekclaw-logger — 结构化日志

## 模块概述

最轻量的 crate（28 行）。封装 `tracing` + `tracing-subscriber` 的日志初始化，提供统一的 `init()` 函数。

## 源码文件说明

| 文件 | 职责 |
|------|------|
| `src/lib.rs` | `init()` 函数：读取环境变量，配置日志过滤器和输出格式，初始化全局订阅者 |

## API

```rust
pub fn init()  // 调用一次，初始化全局日志
```

## 环境变量

| 变量 | 作用 | 默认值 |
|------|------|--------|
| `GEEKCLAW_LOG` | 日志级别过滤（如 `debug`, `info`, `geekclaw_agent=trace`） | `info` |
| `GEEKCLAW_LOG_JSON` | 设为 `1` 或 `true` 启用 JSON 格式输出 | 关闭（人类可读格式） |

## 设计决策

- 使用 `EnvFilter` 支持 per-crate 级别控制（如 `GEEKCLAW_LOG=info,geekclaw_providers=debug`）
- JSON 模式用于生产环境日志收集（可对接 ELK、Loki 等）

## 不完善之处

- **无文件输出**：只输出到 stderr，不支持写日志文件或日志轮转
- **无结构化字段规范**：Go 版本的 `logger.InfoCF("agent", "msg", fields)` 强制分类和结构化字段，Rust 版依赖 tracing 的 `info!(field = %val)` 宏，没有统一的字段命名约定
- **无动态级别调整**：运行中不能更改日志级别，需要重启
