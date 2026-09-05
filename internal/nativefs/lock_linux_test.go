//go:build linux

package nativefs

import (
	"errors"
	"os"
	"testing"
)

func TestTmpfsLockConformance(t *testing.T) {
	directory, err := os.MkdirTemp("/dev/shm", "trb-lock-conformance-")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal(err)
		}
		t.Skipf("tmpfs fixture unavailable: %v", err)
	}
	defer os.RemoveAll(directory)
	t.Setenv("TMPDIR", directory)
	t.Run("regular-reentry", TestTryLockKeepsFileAndRejectsReentry)
	t.Run("symlinks-nonregular", TestTryLockRejectsSymlinksAndNonregularFiles)
	t.Run("process-release", TestTryLockProcessRelease)
	t.Run("killed-process", TestTryLockKilledProcessReleases)
	t.Run("leaf-race", TestTryLockRejectsConcurrentLeafSymlinks)
	t.Run("anchor-parents", TestTryLockUsesRenamedAnchorAndContainedParents)
}

func TestProcfsLockProfileUnsupported(t *testing.T) {
	file, err := os.Open("/proc/version")
	if err != nil {
		t.Skipf("procfs fixture unavailable: %v", err)
	}
	defer file.Close()
	if err := localLockProfile(file); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("procfs admitted: %v", err)
	}
}
