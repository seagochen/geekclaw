package fileutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建源文件
	srcFile := filepath.Join(tmpDir, "source.txt")
	content := []byte("test content")
	err := os.WriteFile(srcFile, content, 0o644)
	require.NoError(t, err)

	// 复制到目标路径
	dstFile := filepath.Join(tmpDir, "dest.txt")
	err = CopyFile(srcFile, dstFile)
	require.NoError(t, err)

	// 验证内容
	result, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, content, result)

	// 验证文件权限
	info, err := os.Stat(dstFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestCopyFileSourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "nonexistent.txt")
	dstFile := filepath.Join(tmpDir, "dest.txt")

	err := CopyFile(srcFile, dstFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open source file")
}

func TestCopyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "source")
	dstDir := filepath.Join(tmpDir, "dest")

	// 创建源目录结构
	err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0o644)
	require.NoError(t, err)

	// 复制目录
	err = CopyDirectory(srcDir, dstDir)
	require.NoError(t, err)

	// 验证已复制的文件
	content1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content1", string(content1))

	content2, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content2", string(content2))
}

func TestCopyFileOrDirectory_File(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建源文件
	srcFile := filepath.Join(tmpDir, "source.txt")
	err := os.WriteFile(srcFile, []byte("test"), 0o644)
	require.NoError(t, err)

	// 以文件方式复制
	dstFile := filepath.Join(tmpDir, "dest.txt")
	err = CopyFileOrDirectory(srcFile, dstFile)
	require.NoError(t, err)

	// 验证
	content, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, "test", string(content))
}

func TestCopyFileOrDirectory_Directory(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "source")
	dstDir := filepath.Join(tmpDir, "dest")

	// 创建源目录
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("test"), 0o644)
	require.NoError(t, err)

	// 以目录方式复制
	err = CopyFileOrDirectory(srcDir, dstDir)
	require.NoError(t, err)

	// 验证
	content, err := os.ReadFile(filepath.Join(dstDir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "test", string(content))
}
