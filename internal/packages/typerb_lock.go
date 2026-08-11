package packages

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/project"
)

const (
	TypeRBLockName       = "trb.lock"
	typeRBLockFormat     = 1
	typeRBChecksumPrefix = "sha256:"
)

type TypeRBLock struct {
	FormatVersion  int                            `json:"formatVersion"`
	ConfigChecksum string                         `json:"configChecksum"`
	Imports        map[string]string              `json:"imports"`
	Packages       map[string]TypeRBLockedPackage `json:"packages"`
}

type TypeRBLockedPackage struct {
	Version          string            `json:"version"`
	Source           string            `json:"source,omitempty"`
	Revision         string            `json:"revision,omitempty"`
	Checksum         string            `json:"checksum,omitempty"`
	Path             string            `json:"path,omitempty"`
	ManifestChecksum string            `json:"manifestChecksum,omitempty"`
	Dependencies     map[string]string `json:"dependencies,omitempty"`
}

func TypeRBLockPath(config *project.Config) string {
	return filepath.Join(config.Root, TypeRBLockName)
}

func ReadTypeRBLock(path string) (*TypeRBLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var lock TypeRBLock
	if err := decoder.Decode(&lock); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode %s: trailing JSON content", path)
	}
	if err := lock.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &lock, nil
}

func (l *TypeRBLock) Validate() error {
	if l.FormatVersion != typeRBLockFormat {
		return fmt.Errorf("unsupported formatVersion %d; expected %d", l.FormatVersion, typeRBLockFormat)
	}
	if !validTypeRBChecksum(l.ConfigChecksum) {
		return errors.New("configChecksum must use sha256")
	}
	if l.Imports == nil || l.Packages == nil {
		return errors.New("imports and packages are required")
	}
	for alias, canonical := range l.Imports {
		if !validExternalPackageName(alias) || !validExternalPackageName(canonical) {
			return fmt.Errorf("invalid import mapping %q -> %q", alias, canonical)
		}
		if _, exists := l.Packages[canonical]; !exists {
			return fmt.Errorf("import %s references missing package %s", alias, canonical)
		}
	}
	for name, locked := range l.Packages {
		if !validExternalPackageName(name) || !validPackageVersion(locked.Version) {
			return fmt.Errorf("invalid locked package %q at version %q", name, locked.Version)
		}
		local := strings.TrimSpace(locked.Path) != ""
		if local {
			if locked.Source != "" || locked.Revision != "" || locked.Checksum != "" {
				return fmt.Errorf("local package %s cannot contain source, revision, or checksum", name)
			}
			if !validTypeRBChecksum(locked.ManifestChecksum) {
				return fmt.Errorf("local package %s requires a manifest checksum", name)
			}
		} else if locked.Source == "" || locked.Revision == "" || !validTypeRBChecksum(locked.Checksum) {
			return fmt.Errorf("remote package %s requires source, revision, and sha256 checksum", name)
		} else if locked.ManifestChecksum != "" {
			return fmt.Errorf("remote package %s cannot contain a separate manifest checksum", name)
		}
		for alias, dependency := range locked.Dependencies {
			if !validExternalPackageName(alias) || !validExternalPackageName(dependency) {
				return fmt.Errorf("package %s has invalid dependency mapping %q -> %q", name, alias, dependency)
			}
			if _, exists := l.Packages[dependency]; !exists {
				return fmt.Errorf("package %s dependency %s is missing from the lock", name, dependency)
			}
		}
	}
	return nil
}

func WriteTypeRBLock(path string, lock *TypeRBLock) error {
	if err := lock.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".trb-lock-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func typeRBConfigChecksum(config *project.Config) (string, error) {
	data, err := json.Marshal(config.Packages)
	if err != nil {
		return "", err
	}
	return typeRBBytesChecksum(data), nil
}

func typeRBManifestChecksum(manifest *TypeRBManifest) (string, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return typeRBBytesChecksum(data), nil
}

func typeRBBytesChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return typeRBChecksumPrefix + hex.EncodeToString(sum[:])
}

func validTypeRBChecksum(checksum string) bool {
	_, err := checksumCacheName(checksum)
	return err == nil
}

func directoryChecksum(root string) (string, error) {
	hash := sha256.New()
	var names []string
	err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		if relative == ".git" || strings.HasPrefix(relative, ".git"+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if relative == "." || entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("package contains unsupported symbolic link %s", filepath.ToSlash(relative))
		}
		names = append(names, relative)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	for _, relative := range names {
		file, err := os.Open(filepath.Join(root, relative))
		if err != nil {
			return "", err
		}
		name := filepath.ToSlash(relative)
		if _, err := fmt.Fprintf(hash, "%d:%s:", len(name), name); err != nil {
			file.Close()
			return "", err
		}
		if _, err := io.Copy(hash, file); err != nil {
			file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
		if _, err := hash.Write([]byte{0}); err != nil {
			return "", err
		}
	}
	return typeRBChecksumPrefix + hex.EncodeToString(hash.Sum(nil)), nil
}

func checksumCacheName(checksum string) (string, error) {
	if !strings.HasPrefix(checksum, typeRBChecksumPrefix) {
		return "", errors.New("unsupported package checksum")
	}
	value := strings.TrimPrefix(checksum, typeRBChecksumPrefix)
	if _, err := hex.DecodeString(value); err != nil || len(value) != sha256.Size*2 {
		return "", errors.New("invalid package checksum")
	}
	return value, nil
}
