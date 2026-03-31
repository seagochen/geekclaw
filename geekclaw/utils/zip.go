package utils

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
)

// ExtractZipFile 将磁盘上的 ZIP 归档解压到 targetDir。
// 逐个从磁盘读取条目，保持最小内存使用。
//
// 安全性：拒绝路径遍历攻击和符号链接。
func ExtractZipFile(zipPath string, targetDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("invalid ZIP: %w", err)
	}
	defer reader.Close()

	logger.DebugCF("zip", "Extracting ZIP", map[string]any{
		"zip_path":   zipPath,
		"target_dir": targetDir,
		"entries":    len(reader.File),
	})

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create target dir: %w", err)
	}

	for _, f := range reader.File {
		// 路径遍历防护。
		cleanName := filepath.Clean(f.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			return fmt.Errorf("zip entry has unsafe path: %q", f.Name)
		}

		destPath := filepath.Join(targetDir, cleanName)

		// 二次检查解析后的路径是否在目标目录内（纵深防御）。
		targetDirClean := filepath.Clean(targetDir)
		if !strings.HasPrefix(filepath.Clean(destPath), targetDirClean+string(filepath.Separator)) &&
			filepath.Clean(destPath) != targetDirClean {
			return fmt.Errorf("zip entry escapes target dir: %q", f.Name)
		}

		mode := f.FileInfo().Mode()

		// 拒绝任何符号链接。
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("zip contains symlink %q; symlinks are not allowed", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return err
			}
			continue
		}

		// 确保父目录存在。
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return err
		}

		if err := extractSingleFile(f, destPath); err != nil {
			return err
		}
	}

	return nil
}

// extractSingleFile 将单个 zip.File 条目解压到 destPath，并进行大小检查。
func extractSingleFile(f *zip.File, destPath string) error {
	const maxFileSize = 5 * 1024 * 1024 // 5MB，根据需要调整

	// 如果可用，检查头部中的未压缩大小。
	if f.UncompressedSize64 > maxFileSize {
		return fmt.Errorf("zip entry %q is too large (%d bytes)", f.Name, f.UncompressedSize64)
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open zip entry %q: %w", f.Name, err)
	}
	defer rc.Close()

	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file %q: %w", destPath, err)
	}
	// 使用 defer 进行防御性清理：记录关闭错误并移除部分写入的文件。
	defer func() {
		if cerr := outFile.Close(); cerr != nil {
			_ = os.Remove(destPath)
			logger.ErrorCF("zip", "Failed to close file", map[string]any{
				"dest_path": destPath,
				"error":     cerr.Error(),
			})
		}
	}()

	// 流式大小检查：防止超限和恶意/损坏的头部。
	written, err := io.CopyN(outFile, rc, maxFileSize+1)
	if err != nil && err != io.EOF {
		_ = os.Remove(destPath)
		return fmt.Errorf("failed to extract %q: %w", f.Name, err)
	}
	if written > maxFileSize {
		_ = os.Remove(destPath)
		return fmt.Errorf("zip entry %q exceeds max size (%d bytes)", f.Name, written)
	}

	return nil
}
