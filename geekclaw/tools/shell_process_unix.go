//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
)

// prepareCommandForTermination 为命令设置进程组，以便可以终止整个进程树。
func prepareCommandForTermination(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateProcessTree 终止由 shell 命令生成的整个进程组。
func terminateProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid
	if pid <= 0 {
		return nil
	}

	// 杀死由 shell 命令生成的整个进程组。
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	// 对 shell 进程本身进行回退杀死。
	_ = cmd.Process.Kill()
	return nil
}
