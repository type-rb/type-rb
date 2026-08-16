package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	generatedWorkspaceFormatVersion = 1
	workspaceCreationGrace          = time.Minute
)

type generatedWorkspaceMetadata struct {
	FormatVersion int       `json:"formatVersion"`
	Kind          string    `json:"kind"`
	PID           int       `json:"pid"`
	CreatedAt     time.Time `json:"createdAt"`
	Retained      bool      `json:"retained,omitempty"`
}

type generatedWorkspace struct {
	projectRoot string
	kind        string
	path        string
	lease       *os.File
	retain      bool
	closed      bool
}

func createGeneratedWorkspace(projectRoot, kind string) (*generatedWorkspace, error) {
	if kind != "run" && kind != "test" {
		return nil, fmt.Errorf("unsupported generated workspace kind %q", kind)
	}
	base := generatedWorkspaceBase(projectRoot, kind)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	if _, err := reapGeneratedWorkspaces(projectRoot, kind, true); err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp(base, fmt.Sprintf("%d-*", os.Getpid()))
	if err != nil {
		return nil, err
	}
	workspace := &generatedWorkspace{projectRoot: projectRoot, kind: kind, path: root}
	cleanup := func(cause error) (*generatedWorkspace, error) {
		_ = workspace.releaseLease()
		return nil, errors.Join(cause, os.RemoveAll(root))
	}
	lease, err := os.OpenFile(filepath.Join(root, "lease.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return cleanup(err)
	}
	workspace.lease = lease
	acquired, err := acquireWorkspaceLease(lease, false)
	if err != nil {
		return cleanup(err)
	}
	if !acquired {
		return cleanup(errors.New("generated workspace lease was not acquired"))
	}
	metadata := generatedWorkspaceMetadata{
		FormatVersion: generatedWorkspaceFormatVersion,
		Kind:          kind,
		PID:           os.Getpid(),
		CreatedAt:     time.Now().UTC(),
	}
	if err := writeGeneratedWorkspaceMetadata(root, metadata); err != nil {
		return cleanup(err)
	}
	return workspace, nil
}

func (w *generatedWorkspace) Path() string { return w.path }

func (w *generatedWorkspace) Keep() { w.retain = true }

func (w *generatedWorkspace) Close() (string, error) {
	if w == nil || w.closed {
		return "", nil
	}
	w.closed = true
	if !w.retain {
		return "", errors.Join(w.releaseLease(), os.RemoveAll(w.path))
	}
	metadata, err := readGeneratedWorkspaceMetadata(w.path)
	if err == nil {
		metadata.Retained = true
		err = writeGeneratedWorkspaceMetadata(w.path, metadata)
	}
	if err != nil {
		return "", errors.Join(err, w.releaseLease(), os.RemoveAll(w.path))
	}
	if releaseErr := w.releaseLease(); releaseErr != nil {
		return w.path, releaseErr
	}
	retainedRoot := filepath.Join(w.projectRoot, ".trb", "generated")
	if err := os.MkdirAll(retainedRoot, 0o755); err != nil {
		return w.path, err
	}
	destination := filepath.Join(retainedRoot, filepath.Base(w.path))
	if err := os.Rename(w.path, destination); err != nil {
		return w.path, err
	}
	w.path = destination
	return destination, nil
}

func (w *generatedWorkspace) releaseLease() error {
	if w == nil || w.lease == nil {
		return nil
	}
	lease := w.lease
	w.lease = nil
	return errors.Join(releaseWorkspaceLease(lease), lease.Close())
}

func (c *CLI) closeGeneratedWorkspace(workspace *generatedWorkspace, resultErr *error) {
	retainedPath, cleanupErr := workspace.Close()
	if retainedPath != "" {
		fmt.Fprintf(c.Stdout, "generated files kept at %s\n", displayProjectPath(workspace.projectRoot, retainedPath))
	}
	if cleanupErr == nil {
		return
	}
	cleanupErr = fmt.Errorf("clean generated workspace: %w", cleanupErr)
	if *resultErr == nil {
		*resultErr = cleanupErr
		return
	}
	fmt.Fprintln(c.Stderr, "trb:", cleanupErr)
}

func (c *CLI) runClean(args []string) error {
	flags := flag.NewFlagSet("clean", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	cleanBuild := flags.Bool("build", false, "remove the configured build output")
	cleanCache := flags.Bool("cache", false, "remove project package and native type caches")
	cleanGenerated := flags.Bool("generated", false, "remove source retained by --keep-generated")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("clean does not accept paths")
	}
	config, err := loadConfig(*configPath, ".")
	if err != nil {
		return err
	}
	var removed []string
	for _, kind := range []string{"run", "test"} {
		paths, err := reapGeneratedWorkspaces(config.Root, kind, true)
		if err != nil {
			return err
		}
		removed = append(removed, paths...)
	}
	if *cleanGenerated {
		path := filepath.Join(config.Root, ".trb", "generated")
		wasRemoved, err := removeManagedPath(path)
		if err != nil {
			return err
		}
		if wasRemoved {
			removed = append(removed, path)
		}
	}
	if *cleanCache {
		for _, path := range []string{
			filepath.Join(config.Root, ".trb", "packages"),
			filepath.Join(config.Root, ".trb", "native-types.json"),
		} {
			wasRemoved, err := removeManagedPath(path)
			if err != nil {
				return err
			}
			if wasRemoved {
				removed = append(removed, path)
			}
		}
	}
	if *cleanBuild {
		output := config.OutputPath()
		existed, err := managedPathExists(output)
		if err != nil {
			return err
		}
		if err := cleanBuildOutput(config, output); err != nil {
			return err
		}
		stillExists, err := managedPathExists(output)
		if err != nil {
			return err
		}
		if existed && stillExists {
			return fmt.Errorf("refusing to remove unsafe build output %s", output)
		}
		if existed {
			removed = append(removed, output)
		}
	}
	for _, path := range removed {
		fmt.Fprintf(c.Stdout, "removed %s\n", displayProjectPath(config.Root, path))
	}
	return nil
}

func removeManagedPath(path string) (bool, error) {
	exists, err := managedPathExists(path)
	if err != nil || !exists {
		return false, err
	}
	return true, os.RemoveAll(path)
}

func managedPathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func displayProjectPath(projectRoot, path string) string {
	relative, err := filepath.Rel(projectRoot, path)
	if err != nil || relative == ".." || filepath.IsAbs(relative) ||
		len(relative) > 3 && relative[:3] == ".."+string(filepath.Separator) {
		return path
	}
	return filepath.ToSlash(relative)
}

func generatedWorkspaceBase(projectRoot, kind string) string {
	return filepath.Join(projectRoot, ".trb", kind)
}

func generatedWorkspaceMarker(root string) string {
	return filepath.Join(root, "owner.json")
}

func writeGeneratedWorkspaceMetadata(root string, metadata generatedWorkspaceMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(generatedWorkspaceMarker(root), data, 0o600)
}

func readGeneratedWorkspaceMetadata(root string) (generatedWorkspaceMetadata, error) {
	data, err := os.ReadFile(generatedWorkspaceMarker(root))
	if err != nil {
		return generatedWorkspaceMetadata{}, err
	}
	var metadata generatedWorkspaceMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return generatedWorkspaceMetadata{}, err
	}
	return metadata, nil
}

func reapGeneratedWorkspaces(projectRoot, kind string, removeUnknown bool) ([]string, error) {
	base := generatedWorkspaceBase(projectRoot, kind)
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var removed []string
	var cleanupErrors []error
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		root := filepath.Join(base, entry.Name())
		metadata, metadataErr := readGeneratedWorkspaceMetadata(root)
		if metadataErr != nil || metadata.FormatVersion != generatedWorkspaceFormatVersion || metadata.Kind != kind {
			if !removeUnknown || workspaceIsWithinCreationGrace(root) {
				continue
			}
			if err := os.RemoveAll(root); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("remove generated workspace %s: %w", root, err))
			} else {
				removed = append(removed, root)
			}
			continue
		}
		if metadata.Retained {
			continue
		}
		lease, err := os.OpenFile(filepath.Join(root, "lease.lock"), os.O_RDWR, 0)
		if errors.Is(err, os.ErrNotExist) {
			if time.Since(metadata.CreatedAt) < workspaceCreationGrace {
				continue
			}
			if err := os.RemoveAll(root); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("remove generated workspace %s: %w", root, err))
			} else {
				removed = append(removed, root)
			}
			continue
		}
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("open generated workspace lease %s: %w", root, err))
			continue
		}
		acquired, lockErr := acquireWorkspaceLease(lease, true)
		if lockErr != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("lock generated workspace %s: %w", root, lockErr))
			_ = lease.Close()
			continue
		}
		if !acquired {
			_ = lease.Close()
			continue
		}
		closeErr := errors.Join(releaseWorkspaceLease(lease), lease.Close())
		if closeErr != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("close generated workspace lease %s: %w", root, closeErr))
			continue
		}
		if err := os.RemoveAll(root); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove generated workspace %s: %w", root, err))
		} else {
			removed = append(removed, root)
		}
	}
	return removed, errors.Join(cleanupErrors...)
}

func workspaceIsWithinCreationGrace(root string) bool {
	info, err := os.Stat(root)
	return err == nil && time.Since(info.ModTime()) < workspaceCreationGrace
}
