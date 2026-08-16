package cli

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
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
	signal.Notify(received, os.Interrupt)
	defer signal.Stop(received)
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o644); err != nil {
		os.Exit(2)
	}
	<-received
	if err := os.WriteFile(receivedPath, []byte("received\n"), 0o644); err != nil {
		os.Exit(2)
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
