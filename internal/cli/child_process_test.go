package cli

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCommandSignalRelayForwardsInterruptAndWaits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not provide a graceful os.Interrupt Process.Signal implementation")
	}
	root := t.TempDir()
	readyPath := filepath.Join(root, "ready")
	receivedPath := filepath.Join(root, "received")
	command := exec.Command(os.Args[0])
	// /dev/null is a character device, but it has no terminal process group
	// that can deliver Ctrl-C to the child on the relay's behalf.
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	command.Stdin = input
	command.Env = append(os.Environ(),
		"TRB_TEST_SIGNAL_HELPER=1",
		"TRB_TEST_SIGNAL_READY="+readyPath,
		"TRB_TEST_SIGNAL_RECEIVED="+receivedPath,
	)
	relay := &commandSignalRelay{signals: make(chan os.Signal, 2)}
	wait := make(chan error, 1)
	go func() { wait <- relay.Run(command) }()
	t.Cleanup(func() {
		if command.Process != nil && command.ProcessState == nil {
			_ = forceStopChildProcess(command, true)
		}
	})
	waitForSignalHelperFile(t, readyPath)
	relay.signals <- os.Interrupt
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("signal relay returned an error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("signal relay did not wait for the child to exit")
	}
	waitForSignalHelperFile(t, receivedPath)
}

func runTestSignalHelper() {
	readyPath := os.Getenv("TRB_TEST_SIGNAL_READY")
	receivedPath := os.Getenv("TRB_TEST_SIGNAL_RECEIVED")
	if readyPath == "" || receivedPath == "" {
		os.Exit(2)
	}
	received := make(chan os.Signal, 1)
	signal.Notify(received, childProcessSignals()...)
	defer signal.Stop(received)
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o644); err != nil {
		os.Exit(2)
	}
	<-received
	if os.Getenv("TRB_TEST_SIGNAL_HOLD") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	if err := os.WriteFile(receivedPath, []byte("received\n"), 0o644); err != nil {
		os.Exit(2)
	}
}

func TestCommandSignalRelayForcedStopIsFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no graceful os.Interrupt Process.Signal implementation")
	}
	for _, repeated := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "second-signal"}[repeated], func(t *testing.T) {
			root := t.TempDir()
			ready := filepath.Join(root, "ready")
			command := exec.Command(os.Args[0])
			command.Env = append(os.Environ(), "TRB_TEST_SIGNAL_HELPER=1", "TRB_TEST_SIGNAL_HOLD=1",
				"TRB_TEST_SIGNAL_READY="+ready, "TRB_TEST_SIGNAL_RECEIVED="+filepath.Join(root, "received"))
			relay := &commandSignalRelay{signals: make(chan os.Signal, 2), shutdownTimeout: 150 * time.Millisecond}
			wait := make(chan error, 1)
			go func() { wait <- relay.Run(command) }()
			t.Cleanup(func() {
				if command.Process != nil && command.ProcessState == nil {
					_ = forceStopChildProcess(command, true)
				}
			})
			waitForSignalHelperFile(t, ready)
			relay.signals <- os.Interrupt
			if repeated {
				relay.signals <- os.Interrupt
			}
			select {
			case err := <-wait:
				if err == nil || !strings.Contains(err.Error(), "forced to stop") {
					t.Fatalf("forced termination reported success: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("forced termination did not join the child")
			}
		})
	}
}

func TestCommandSignalRelayDoesNotStartAfterStop(t *testing.T) {
	relay := &commandSignalRelay{signals: make(chan os.Signal, 1)}
	relay.signals <- os.Interrupt
	command := exec.Command(os.Args[0])
	if err := relay.Run(command); err == nil || !relay.interrupted || command.Process != nil {
		t.Fatalf("started a command after a pending stop: process=%v error=%v", command.Process, err)
	}
}

func waitForSignalHelperFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
