//go:build linux || darwin

package nativefs

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func lockRoot(t *testing.T) (*os.Root, string) {
	t.Helper()
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root, directory
}

func TestTryLockKeepsFileAndRejectsReentry(t *testing.T) {
	root, directory := lockRoot(t)
	name := filepath.Join(directory, "guard")
	if err := os.WriteFile(name, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	first, err := TryLock(root, "guard")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := TryLock(root, "guard")
	if second != nil || !errors.Is(err, ErrBusy) {
		t.Fatalf("reentry: %v, %v", second, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := TryLock(root, "guard")
	if err != nil {
		t.Fatal(err)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || string(data) != "keep" || after.Mode().Perm() != 0o600 {
		t.Fatalf("lock changed file: %q, %v", data, after.Mode())
	}
}

func TestTryLockRejectsSymlinksAndNonregularFiles(t *testing.T) {
	root, directory := lockRoot(t)
	if err := os.WriteFile(filepath.Join(directory, "target"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"link": "target", "dangling": "missing", "absolute": filepath.Join(directory, "target")} {
		if err := os.Symlink(target, filepath.Join(directory, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := root.Mkdir("directory", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(directory, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"link", "dangling", "absolute", "directory", "fifo", "fifo/guard", "target/guard"} {
		t.Run(name, func(t *testing.T) {
			file, err := TryLock(root, name)
			if file != nil {
				_ = file.Close()
				t.Fatal("acquired forbidden file")
			}
			if err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	if _, err := root.Stat("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created dangling target: %v", err)
	}
}

func TestTryLockUsesRenamedAnchorAndContainedParents(t *testing.T) {
	root, directory := lockRoot(t)
	if err := root.Mkdir("old", 0o700); err != nil {
		t.Fatal(err)
	}
	anchor, err := root.OpenRoot("old")
	if err != nil {
		t.Fatal(err)
	}
	defer anchor.Close()
	if err := root.Rename("old", "new"); err != nil {
		t.Fatal(err)
	}
	file, err := TryLock(anchor, "guard")
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if _, err := root.Stat("new/guard"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("new", filepath.Join(directory, "inside")); err != nil {
		t.Fatal(err)
	}
	file, err = TryLock(root, "inside/guard")
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(directory, "outside")); err != nil {
		t.Fatal(err)
	}
	if file, err := TryLock(root, "outside/guard"); file != nil || err == nil {
		if file != nil {
			_ = file.Close()
		}
		t.Fatal("escaped anchor")
	}
	if _, err := os.Stat(filepath.Join(outside, "guard")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created outside lock: %v", err)
	}
}

func TestTryLockProcessRelease(t *testing.T) {
	if directory := os.Getenv("TRB_LOCK_TEST_CHILD"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			os.Exit(2)
		}
		_, err = TryLock(root, "guard")
		if os.Getenv("TRB_LOCK_TEST_BUSY") == "1" {
			if !errors.Is(err, ErrBusy) {
				os.Exit(3)
			}
			os.Exit(0)
		}
		if err != nil {
			os.Exit(4)
		}
		// Exit without resource cleanup: the kernel must release the lock.
		os.Exit(0)
	}
	root, directory := lockRoot(t)
	first, err := TryLock(root, "guard")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	child := func(busy string) {
		t.Helper()
		command := exec.Command(os.Args[0], "-test.run=^TestTryLockProcessRelease$")
		command.Env = append(os.Environ(), "TRB_LOCK_TEST_CHILD="+directory, "TRB_LOCK_TEST_BUSY="+busy)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("child: %v\n%s", err, output)
		}
	}
	child("1")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	child("0")
	last, err := TryLock(root, "guard")
	if err != nil {
		t.Fatal(err)
	}
	_ = last.Close()
}

func TestTryLockKilledProcessReleases(t *testing.T) {
	if directory := os.Getenv("TRB_KILLED_LOCK_CHILD"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			os.Exit(2)
		}
		file, err := TryLock(root, "guard")
		if err != nil {
			os.Exit(3)
		}
		defer file.Close()
		_, _ = os.Stdout.WriteString("held\n")
		var input [1]byte
		_, _ = os.Stdin.Read(input[:])
		os.Exit(4)
	}
	root, directory := lockRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTryLockKilledProcessReleases$")
	child.Env = append(os.Environ(), "TRB_KILLED_LOCK_CHILD="+directory)
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "held\n" {
		t.Fatalf("handshake: %q, %v", line, err)
	}
	if file, err := TryLock(root, "guard"); file != nil || !errors.Is(err, ErrBusy) {
		if file != nil {
			_ = file.Close()
		}
		t.Fatalf("child did not hold lock: %v", err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("child was not killed")
	}
	file, err := TryLock(root, "guard")
	if err != nil {
		t.Fatalf("killed child retained lock: %v", err)
	}
	_ = file.Close()
}

func TestTryLockRejectsConcurrentLeafSymlinks(t *testing.T) {
	root, directory := lockRoot(t)
	if err := root.WriteFile("target", []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	target, err := TryLock(root, "target")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.Symlink("target", filepath.Join(directory, "swap"))
			_ = os.Rename(filepath.Join(directory, "swap"), filepath.Join(directory, "guard"))
			_ = os.Remove(filepath.Join(directory, "guard"))
		}
	}()
	defer func() { close(stop); <-done }()
	for index := 0; index < 1000; index++ {
		file, err := TryLock(root, "guard")
		// No other operation locks guard. Busy would mean a followed link
		// reached the independently locked target.
		if errors.Is(err, ErrBusy) {
			t.Fatal("followed a concurrently replaced symlink")
		}
		if file != nil {
			_ = file.Close()
		}
	}
}
