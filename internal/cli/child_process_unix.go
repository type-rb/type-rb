//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

func childProcessSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func configureChildProcess(command *exec.Cmd, isolateProcessGroup bool) bool {
	if !isolateProcessGroup {
		return false
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
	return true
}

func forwardChildProcessSignal(command *exec.Cmd, received os.Signal, isolatedProcessGroup bool) error {
	signalValue, ok := received.(syscall.Signal)
	if !ok || !isolatedProcessGroup {
		return command.Process.Signal(received)
	}
	return syscall.Kill(-command.Process.Pid, signalValue)
}

func forceStopChildProcess(command *exec.Cmd, isolatedProcessGroup bool) error {
	if !isolatedProcessGroup {
		return command.Process.Kill()
	}
	return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
}
