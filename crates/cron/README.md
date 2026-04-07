# geekclaw-cron — 定时任务调度

## 模块概述

支持三种调度模式的定时任务系统：cron 表达式、固定间隔、一次性定时。使用 JSON 文件持久化任务，通过 tick 循环检查到期任务并触发回调。

## 源码文件说明

| 文件 | 职责 |
|------|------|
| `src/types.rs` | 核心数据结构：`CronJob`、`CronSchedule`（3 种调度）、`CronPayload`（触发内容）、`CronJobState`（运行状态）、`CronStoreData`（持久化格式） |
| `src/store.rs` | `CronStore`：JSON 文件 CRUD、原子写入、下次执行时间计算、cron 表达式解析。包含 10 个单元测试 |
| `src/service.rs` | `CronService`：tick 循环（30 秒间隔）、到期检测、回调触发、优雅关闭 |
| `src/error.rs` | `CronError`：IO、JSON、任务未找到 |
| `src/lib.rs` | 模块导出 |

## 核心类型

```rust
pub struct CronJob {
    pub id: String,              // 16 字符 UUID 前缀
    pub name: String,
    pub enabled: bool,
    pub schedule: CronSchedule,  // 调度配置
    pub payload: CronPayload,    // 触发时的负载
    pub state: CronJobState,     // 运行时状态
    pub delete_after_run: bool,  // 一次性任务执行后自动删除
}

pub struct CronSchedule {
    pub kind: String,            // "cron" | "every" | "at"
    pub at_ms: Option<i64>,      // 一次性：目标时间戳（毫秒）
    pub every_ms: Option<i64>,   // 间隔：毫秒数
    pub expr: Option<String>,    // cron：cron 表达式
    pub tz: Option<String>,      // 时区（暂未使用）
}
```

## 算法说明

### 三种调度模式的下次执行时间计算

```rust
fn compute_next_run(schedule, now_ms) -> Option<i64>:
    match schedule.kind:
        "at"    → if at_ms > now { Some(at_ms) } else { None }
        "every" → Some(now + every_ms)
        "cron"  → 使用 cron crate 解析表达式，计算 now 之后的下一个匹配时间
```

**cron 表达式**使用 7 字段格式（秒 分 时 日 月 周 年）：
- `0 * * * * * *` — 每分钟第 0 秒
- `0 0 9 * * * *` — 每天 9:00:00
- `0 30 */2 * * * *` — 每 2 小时的第 30 分钟

### Tick 循环

```
CronService::run():
    loop:
        sleep(30 seconds)
        for job in store.jobs():
            if job.enabled && job.next_run_at_ms <= now:
                callback(job)
                update job.state (last_run, next_run, status)
                if job.delete_after_run:
                    remove job
        store.save()
```

### 原子持久化

与 memory 模块相同的模式：写临时文件 → rename。

## 设计决策

- 任务 ID 使用 UUID 前 16 字符，足够唯一且便于人类阅读
- `delete_after_run` 自动清理一次性任务
- `CronStore` 是同步的（非 async），因为 JSON 文件通常很小
- tick 间隔 30 秒是精度和 CPU 使用的平衡点

## 不完善之处

- **时区支持未实现**：`CronSchedule.tz` 字段存在但未使用，所有时间按 UTC 处理
- **无任务执行超时**：Go 版本有 `exec_timeout_minutes` 控制，Rust 版的 CronService 触发回调后不跟踪执行时长
- **无错误重试**：任务执行失败后只记录 `last_error`，不会重试
- **tick 精度**：30 秒间隔意味着任务最多延迟 30 秒执行，对于需要秒级精度的场景不够
- **无 Web UI**：Go 版本通过斜杠命令管理（`/cron list`），Rust 版只能通过 cron_tool 工具或代码调用
