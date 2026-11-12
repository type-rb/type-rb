package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/mattn/go-isatty"
)

const childProcessShutdownTimeout = 10 * time.Second

type commandSignalRelay struct {
	signals         chan os.Signal
	stop            func()
	shutdownTimeout time.Duration
	interrupted     bool
}

func newCommandSignalRelay() *commandSignalRelay {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, childProcessSignals()...)
	return &commandSignalRelay{
		signals: signals,
		stop:    func() { signal.Stop(signals) },
	}
}

func (r *commandSignalRelay) Close() {
	if r != nil && r.stop != nil {
		r.stop()
		r.stop = nil
	}
}

func (r *commandSignalRelay) Run(command *exec.Cmd) error {
	// A stop received between preparation and execution must not start a worker.
	select {
	case received := <-r.signals:
		r.interrupted = true
		return fmt.Errorf("command interrupted before start: %s", received)
	default:
	}
	// A terminal already sends interrupts to its foreground process group. Moving
	// the child to a new group without transferring terminal ownership would stop
	// interactive reads, so only non-terminal children need explicit isolation.
	stdin, isFile := command.Stdin.(*os.File)
	terminal := isFile && isatty.IsTerminal(stdin.Fd())
	isolatedProcessGroup := configureChildProcess(command, !terminal)
	if err := command.Start(); err != nil {
		return err
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	select {
	case err := <-wait:
		return err
	case received := <-r.signals:
		r.interrupted = true
		if isolatedProcessGroup || received != os.Interrupt {
			_ = forwardChildProcessSignal(command, received, isolatedProcessGroup)
		}
	}
	shutdownTimeout := r.shutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = childProcessShutdownTimeout
	}
	timer := time.NewTimer(shutdownTimeout)
	defer timer.Stop()
	select {
	case err := <-wait:
		return err
	case <-r.signals:
		_ = forceStopChildProcess(command, isolatedProcessGroup)
		return forcedChildProcessError("repeated stop request", <-wait)
	case <-timer.C:
		_ = forceStopChildProcess(command, isolatedProcessGroup)
		return forcedChildProcessError("shutdown deadline exceeded", <-wait)
	}
}

func forcedChildProcessError(reason string, err error) error {
	if err != nil {
		return fmt.Errorf("child process forced to stop (%s): %w", reason, err)
	}
	return fmt.Errorf("child process forced to stop (%s)", reason)
}
