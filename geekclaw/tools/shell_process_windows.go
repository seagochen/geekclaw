//go:build windows

package tools

import (
	"os/exec"
	"strconv"
)

// prepareCommandForTermination 在 Windows 上为空操作。
func prepareCommandForTermination(cmd *exec.Cmd) {
	// Windows 上无操作
}

// terminateProcessTree 在 Windows 上使用 taskkill 终止进程树。
func terminateProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid
	if pid <= 0 {
		return nil
	}

	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	_ = cmd.Process.Kill()
	return nil
}
