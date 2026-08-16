//go:build windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

func childProcessSignals() []os.Signal { return []os.Signal{os.Interrupt} }

func configureChildProcess(command *exec.Cmd, _ bool) bool {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
	return true
}

func forwardChildProcessSignal(command *exec.Cmd, received os.Signal, _ bool) error {
	if err := command.Process.Signal(received); err == nil {
		return nil
	}
	return command.Process.Kill()
}

func forceStopChildProcess(command *exec.Cmd, _ bool) error { return command.Process.Kill() }
