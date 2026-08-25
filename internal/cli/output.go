package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func projectOutputPath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", errors.New("path must be relative to the project root")
	}
	clean := filepath.Clean(relative)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path cannot escape the project root")
	}
	return filepath.Join(root, clean), nil
}

func atomicOutputWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".trb-output-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
