package agent

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// cmdWorkingDir 记录命令模式下的当前工作目录。
var cmdWorkingDir string

func init() {
	cmdWorkingDir, _ = os.Getwd()
}

// executeShellCommand 在当前工作目录中执行 shell 命令并打印输出。
// 同时处理 cd 命令以切换工作目录。
func executeShellCommand(input string) {
	// 特殊处理 cd 命令以更新工作目录
	if strings.HasPrefix(input, "cd ") || input == "cd" {
		handleCd(input)
		return
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", input)
	} else {
		cmd = exec.Command("sh", "-c", input)
	}
	cmd.Dir = cmdWorkingDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if stdout.Len() > 0 {
		fmt.Print(stdout.String())
		if !strings.HasSuffix(stdout.String(), "\n") {
			fmt.Println()
		}
	}
	if stderr.Len() > 0 {
		fmt.Print(stderr.String())
		if !strings.HasSuffix(stderr.String(), "\n") {
			fmt.Println()
		}
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Printf("Exit code: %d\n", exitErr.ExitCode())
		} else {
			fmt.Printf("Error: %v\n", err)
		}
	}
}

// handleCd 处理 cd 命令，切换命令模式的工作目录。
func handleCd(input string) {
	parts := strings.Fields(input)
	var target string

	if len(parts) < 2 {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		target = home
	} else {
		target = parts[1]
	}

	// 处理 ~ 路径展开
	if strings.HasPrefix(target, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		if target == "~" {
			target = home
		} else if len(target) > 1 && target[1] == '/' {
			target = filepath.Join(home, target[2:])
		}
	}

	// 处理相对路径
	if !filepath.IsAbs(target) {
		target = filepath.Join(cmdWorkingDir, target)
	}

	target = filepath.Clean(target)

	info, err := os.Stat(target)
	if err != nil {
		fmt.Printf("cd: %v\n", err)
		return
	}
	if !info.IsDir() {
		fmt.Printf("cd: %s: Not a directory\n", target)
		return
	}

	cmdWorkingDir = target
}
