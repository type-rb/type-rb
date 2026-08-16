package cli

import (
	"os"
	"os/exec"
	"os/signal"
	"time"
)

const childProcessShutdownTimeout = 10 * time.Second

type commandSignalRelay struct {
	signals chan os.Signal
	stop    func()
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
	// A terminal already sends interrupts to its foreground process group. Moving
	// the child to a new group without transferring terminal ownership would stop
	// interactive reads, so only non-terminal children need explicit isolation.
	isolatedProcessGroup := configureChildProcess(command, !characterDevice(command.Stdin))
	if err := command.Start(); err != nil {
		return err
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	select {
	case err := <-wait:
		return err
	case received := <-r.signals:
		if isolatedProcessGroup || received != os.Interrupt {
			_ = forwardChildProcessSignal(command, received, isolatedProcessGroup)
		}
	}
	timer := time.NewTimer(childProcessShutdownTimeout)
	defer timer.Stop()
	select {
	case err := <-wait:
		return err
	case <-r.signals:
		_ = forceStopChildProcess(command, isolatedProcessGroup)
		return <-wait
	case <-timer.C:
		_ = forceStopChildProcess(command, isolatedProcessGroup)
		return <-wait
	}
}
