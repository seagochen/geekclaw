// GeekClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 GeekClaw contributors

// Package fileutil 提供文件操作工具函数。
package fileutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// WriteFileAtomic 使用临时文件+重命名模式原子地将数据写入文件。
//
// 保证目标文件要么：
// - 完全写入新数据
// - 保持不变（如果重命名前任何步骤失败）
//
// 该函数执行以下步骤：
// 1. 在同一目录创建临时文件（原文件不受影响）
// 2. 将数据写入临时文件
// 3. 同步数据到磁盘（对 SD 卡/闪存存储至关重要）
// 4. 设置文件权限
// 5. 同步目录元数据（确保重命名的持久性）
// 6. 原子地将临时文件重命名为目标路径
//
// 安全保证：
// - 在成功重命名之前，原文件不会被修改
// - 出错时临时文件始终会被清理
// - 重命名前数据已刷新到物理存储
// - 目录条目已同步，防止孤立的 inode
//
// 参数：
//   - path：目标文件路径
//   - data：要写入的数据
//   - perm：文件权限模式（例如 0o600 为安全模式，0o644 为可读模式）
//
// 返回：
//   - 任何步骤失败返回错误，成功返回 nil
//
// 示例：
//
//	// 安全的配置文件（仅所有者可读写）
//	err := utils.WriteFileAtomic("config.json", data, 0o600)
//
//	// 公开可读文件
//	err := utils.WriteFileAtomic("public.txt", data, 0o644)
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 在同一目录创建临时文件（确保原子重命名可用）
	// 使用隐藏前缀（.tmp-）避免某些工具的干扰
	tmpFile, err := os.OpenFile(
		filepath.Join(dir, fmt.Sprintf(".tmp-%d-%d", os.Getpid(), time.Now().UnixNano())),
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		perm,
	)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	tmpPath := tmpFile.Name()
	cleanup := true

	defer func() {
		if cleanup {
			tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	// 将数据写入临时文件
	// 注意：此时原文件未被修改
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// 关键步骤：在其他操作之前强制同步到存储介质。
	// 确保数据已物理写入磁盘，而非仅在缓存中。
	// 对边缘设备上的 SD 卡、eMMC 及其他闪存存储至关重要。
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	// 关闭前设置文件权限
	if err := tmpFile.Chmod(perm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// 重命名前关闭文件（Windows 上必需）
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// 原子重命名：临时文件变为目标文件
	// POSIX 上：rename() 是原子操作
	// Windows 上：Rename() 对文件也是原子操作
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// 同步目录以确保重命名的持久性
	// 防止崩溃后重命名的文件消失
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		dirFile.Close()
	}

	// 成功：跳过清理（文件已重命名，无需删除临时文件）
	cleanup = false
	return nil
}

// CopyFile 将文件从 src 复制到 dst，保留文件权限。
// 如果目标文件已存在，将被覆盖。
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	// 如需创建目标目录
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}

// CopyDirectory 递归地将目录从 src 复制到 dst。
// 保留文件权限和目录结构。
// 如果目标目录已存在，文件将被合并/覆盖。
func CopyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %s: %w", path, err)
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return CopyFile(path, dstPath)
	})
}

// CopyFileOrDirectory 根据源类型复制文件或目录。
// 这是一个便捷函数，根据情况调用 CopyFile 或 CopyDirectory。
func CopyFileOrDirectory(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source: %w", err)
	}

	if info.IsDir() {
		return CopyDirectory(src, dst)
	}
	return CopyFile(src, dst)
}
