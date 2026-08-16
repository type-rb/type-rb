//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package cli

import (
	"os"
	"os/exec"
)

func childProcessSignals() []os.Signal { return []os.Signal{os.Interrupt} }

func configureChildProcess(_ *exec.Cmd, _ bool) bool { return false }

func forwardChildProcessSignal(command *exec.Cmd, received os.Signal, _ bool) error {
	return command.Process.Signal(received)
}

func forceStopChildProcess(command *exec.Cmd, _ bool) error { return command.Process.Kill() }
