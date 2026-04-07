//! 文件系统工具：读取、写入、列出文件和目录。

use async_trait::async_trait;
use serde_json::{json, Value};
use std::path::Path;

use crate::{Tool, ToolContext, ToolResult};

/// 最大文件读取大小（64KB）。
const MAX_READ_SIZE: u64 = 64 * 1024;

/// 文件系统操作工具。
pub struct FileSystemTool;

#[async_trait]
impl Tool for FileSystemTool {
    fn name(&self) -> &str {
        "filesystem"
    }

    fn description(&self) -> &str {
        "文件系统操作：读取文件、写入文件、列出目录内容。\
         操作类型通过 action 参数指定：read_file / write_file / list_dir。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "action": {
                    "type": "string",
                    "enum": ["read_file", "write_file", "list_dir"],
                    "description": "操作类型"
                },
                "path": {
                    "type": "string",
                    "description": "文件或目录路径（相对于工作目录）"
                },
                "content": {
                    "type": "string",
                    "description": "写入的内容（仅 write_file 时需要）"
                }
            },
            "required": ["action", "path"]
        })
    }

    async fn execute(&self, args: Value, ctx: &ToolContext) -> ToolResult {
        let action = match args["action"].as_str() {
            Some(a) => a,
            None => return ToolResult::err("缺少 action 参数"),
        };
        let path_str = match args["path"].as_str() {
            Some(p) => p,
            None => return ToolResult::err("缺少 path 参数"),
        };

        // 解析路径（相对于工作目录）。
        let base = if ctx.working_dir.is_empty() {
            ".".to_string()
        } else {
            ctx.working_dir.clone()
        };
        let full_path = resolve_path(&base, path_str);

        match action {
            "read_file" => read_file(&full_path).await,
            "write_file" => {
                let content = args["content"].as_str().unwrap_or("");
                write_file(&full_path, content).await
            }
            "list_dir" => list_dir(&full_path).await,
            _ => ToolResult::err(format!("未知操作: {action}")),
        }
    }
}

/// 解析路径：如果是相对路径，拼接到 base 上。
fn resolve_path(base: &str, path: &str) -> String {
    let p = Path::new(path);
    if p.is_absolute() {
        path.to_string()
    } else {
        Path::new(base).join(path).to_string_lossy().to_string()
    }
}

/// 读取文件内容。
async fn read_file(path: &str) -> ToolResult {
    let path = Path::new(path);

    // 检查文件是否存在。
    if !path.exists() {
        return ToolResult::err(format!("文件不存在: {}", path.display()));
    }

    // 检查文件大小。
    match tokio::fs::metadata(path).await {
        Ok(meta) => {
            if meta.len() > MAX_READ_SIZE {
                return ToolResult::err(format!(
                    "文件过大（{} 字节），最大允许 {} 字节",
                    meta.len(),
                    MAX_READ_SIZE
                ));
            }
        }
        Err(e) => return ToolResult::err(format!("无法读取文件元数据: {e}")),
    }

    match tokio::fs::read_to_string(path).await {
        Ok(content) => ToolResult::ok(content),
        Err(e) => ToolResult::err(format!("读取文件失败: {e}")),
    }
}

/// 写入文件内容。
async fn write_file(path: &str, content: &str) -> ToolResult {
    let path = Path::new(path);

    // 确保父目录存在。
    if let Some(parent) = path.parent() {
        if !parent.exists() {
            if let Err(e) = tokio::fs::create_dir_all(parent).await {
                return ToolResult::err(format!("创建目录失败: {e}"));
            }
        }
    }

    match tokio::fs::write(path, content).await {
        Ok(()) => ToolResult::ok(format!("已写入 {} 字节到 {}", content.len(), path.display())),
        Err(e) => ToolResult::err(format!("写入文件失败: {e}")),
    }
}

/// 列出目录内容。
async fn list_dir(path: &str) -> ToolResult {
    let path = Path::new(path);

    if !path.exists() {
        return ToolResult::err(format!("目录不存在: {}", path.display()));
    }
    if !path.is_dir() {
        return ToolResult::err(format!("不是目录: {}", path.display()));
    }

    let mut entries = Vec::new();
    let mut read_dir = match tokio::fs::read_dir(path).await {
        Ok(rd) => rd,
        Err(e) => return ToolResult::err(format!("读取目录失败: {e}")),
    };

    while let Ok(Some(entry)) = read_dir.next_entry().await {
        let name = entry.file_name().to_string_lossy().to_string();
        let is_dir = entry.file_type().await.map(|ft| ft.is_dir()).unwrap_or(false);
        let suffix = if is_dir { "/" } else { "" };

        let size = entry
            .metadata()
            .await
            .map(|m| m.len())
            .unwrap_or(0);

        if is_dir {
            entries.push(format!("  {name}{suffix}"));
        } else {
            entries.push(format!("  {name}{suffix}  ({size} bytes)"));
        }
    }

    entries.sort();

    if entries.is_empty() {
        ToolResult::ok("(空目录)")
    } else {
        ToolResult::ok(entries.join("\n"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    fn test_ctx(dir: &str) -> ToolContext {
        ToolContext {
            session_key: "test".into(),
            channel: "test".into(),
            chat_id: "1".into(),
            working_dir: dir.into(),
            metadata: HashMap::new(),
        }
    }

    #[test]
    fn test_resolve_path_relative() {
        assert_eq!(resolve_path("/home/user", "test.txt"), "/home/user/test.txt");
    }

    #[test]
    fn test_resolve_path_absolute() {
        assert_eq!(resolve_path("/home/user", "/tmp/test.txt"), "/tmp/test.txt");
    }

    #[tokio::test]
    async fn test_read_nonexistent() {
        let tool = FileSystemTool;
        let result = tool
            .execute(
                json!({"action": "read_file", "path": "/tmp/geekclaw_nonexistent_12345.txt"}),
                &test_ctx("/tmp"),
            )
            .await;
        assert!(!result.success);
        assert!(result.content.contains("不存在"));
    }

    #[tokio::test]
    async fn test_write_and_read() {
        let tmp = tempfile::tempdir().unwrap();
        let dir = tmp.path().to_str().unwrap();
        let tool = FileSystemTool;
        let ctx = test_ctx(dir);

        // 写入。
        let result = tool
            .execute(
                json!({"action": "write_file", "path": "test.txt", "content": "hello rust"}),
                &ctx,
            )
            .await;
        assert!(result.success);

        // 读取。
        let result = tool
            .execute(json!({"action": "read_file", "path": "test.txt"}), &ctx)
            .await;
        assert!(result.success);
        assert_eq!(result.content, "hello rust");
    }

    #[tokio::test]
    async fn test_list_dir() {
        let tmp = tempfile::tempdir().unwrap();
        let dir = tmp.path().to_str().unwrap();

        // 创建一些文件。
        tokio::fs::write(tmp.path().join("a.txt"), "aaa").await.unwrap();
        tokio::fs::write(tmp.path().join("b.txt"), "bbb").await.unwrap();
        tokio::fs::create_dir(tmp.path().join("subdir")).await.unwrap();

        let tool = FileSystemTool;
        let result = tool
            .execute(json!({"action": "list_dir", "path": "."}), &test_ctx(dir))
            .await;
        assert!(result.success);
        assert!(result.content.contains("a.txt"));
        assert!(result.content.contains("b.txt"));
        assert!(result.content.contains("subdir/"));
    }
}
