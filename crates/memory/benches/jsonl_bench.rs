//! 性能基准测试：JSONL 会话存储。

use std::hint::black_box;
use std::time::Instant;

use geekclaw_memory::{JSONLStore, Message, SessionStore};

fn msg(role: &str, content: &str) -> Message {
    Message {
        role: role.to_string(),
        content: content.to_string(),
        ..Default::default()
    }
}

#[tokio::main]
async fn main() {
    let tmp = tempfile::tempdir().unwrap();
    let store = JSONLStore::new(tmp.path().join("data"), tmp.path().join("meta"))
        .await
        .unwrap();

    // ── Bench: append ───────────────────────────────────────────────────
    let n = 1000;
    let start = Instant::now();
    for i in 0..n {
        store
            .append("bench:session", &msg("user", &format!("message {i}")))
            .await
            .unwrap();
    }
    let elapsed = start.elapsed();
    println!(
        "append x{n}: {:.2}ms total, {:.2}µs/op",
        elapsed.as_secs_f64() * 1000.0,
        elapsed.as_micros() as f64 / n as f64,
    );

    // ── Bench: get_history (full) ───────────────────────────────────────
    let start = Instant::now();
    let iters = 100;
    for _ in 0..iters {
        let history = store.get_history("bench:session", 0).await.unwrap();
        black_box(&history);
    }
    let elapsed = start.elapsed();
    println!(
        "get_history(full, {n} msgs) x{iters}: {:.2}ms total, {:.2}ms/op",
        elapsed.as_secs_f64() * 1000.0,
        elapsed.as_secs_f64() * 1000.0 / iters as f64,
    );

    // ── Bench: get_history (limited) ────────────────────────────────────
    let start = Instant::now();
    for _ in 0..iters {
        let history = store.get_history("bench:session", 10).await.unwrap();
        black_box(&history);
    }
    let elapsed = start.elapsed();
    println!(
        "get_history(limit=10, {n} msgs) x{iters}: {:.2}ms total, {:.2}ms/op",
        elapsed.as_secs_f64() * 1000.0,
        elapsed.as_secs_f64() * 1000.0 / iters as f64,
    );

    // ── Bench: concurrent sessions ──────────────────────────────────────
    let store = std::sync::Arc::new(store);
    let start = Instant::now();
    let mut handles = Vec::new();
    let concurrent = 50;
    for i in 0..concurrent {
        let s = store.clone();
        handles.push(tokio::spawn(async move {
            let key = format!("bench:concurrent:{i}");
            for j in 0..20 {
                s.append(&key, &msg("user", &format!("msg {j}")))
                    .await
                    .unwrap();
            }
            let h = s.get_history(&key, 0).await.unwrap();
            assert_eq!(h.len(), 20);
        }));
    }
    for h in handles {
        h.await.unwrap();
    }
    let elapsed = start.elapsed();
    println!(
        "concurrent ({concurrent} sessions x 20 appends + read): {:.2}ms total",
        elapsed.as_secs_f64() * 1000.0,
    );
}
