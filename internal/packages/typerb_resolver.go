package packages

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/type-rb/type-rb/internal/project"
	"golang.org/x/mod/semver"
)

type TypeRBResolveOptions struct {
	Frozen  bool
	Offline bool
	Update  bool
}

type TypeRBResolvedPackage struct {
	Name     string
	Root     string
	Manifest *TypeRBManifest
}

type DeclarationAdapter struct {
	Package      string
	Mode         string
	Path         string
	Dependencies map[string]string
}

type DeclarationProvider struct {
	Package string
	Mode    string
	Module  string
	Path    string
}

type RuntimeAdapter struct {
	Package      string
	Mode         string
	Path         string
	Dependencies map[string]string
}

type TypeRBPackages struct {
	Lock                 *TypeRBLock
	Aliases              map[string]string
	Packages             []TypeRBResolvedPackage
	NativeDependencies   map[string]string
	DeclarationProviders []DeclarationProvider
	DeclarationAdapters  []DeclarationAdapter
	RuntimeAdapters      []RuntimeAdapter
}

var errLocalTypeRBManifestChanged = errors.New("local TypeRB package manifest changed")

// ResolveTypeRBPackages creates or reuses the deterministic project lock and
// ensures every remote package is present in the content-addressed cache.
func ResolveTypeRBPackages(config *project.Config, options TypeRBResolveOptions) (*TypeRBPackages, error) {
	if len(config.Packages) == 0 && !options.Update {
		return emptyTypeRBPackages(), nil
	}
	checksum, err := typeRBConfigChecksum(config)
	if err != nil {
		return nil, err
	}
	lockPath := TypeRBLockPath(config)
	locked, lockErr := ReadTypeRBLock(lockPath)
	current := lockErr == nil && locked.ConfigChecksum == checksum
	if options.Frozen && !current {
		if lockErr != nil && !errors.Is(lockErr, os.ErrNotExist) {
			return nil, lockErr
		}
		return nil, errors.New("trb.lock does not match trbconfig.jsonc; run trb install without --frozen")
	}
	if current && !options.Update {
		if err := ensureTypeRBPackageCache(config, locked, options.Offline); err != nil {
			return nil, err
		}
		resolved, loadErr := loadResolvedTypeRBPackages(config, locked)
		if loadErr == nil {
			return resolved, nil
		}
		if options.Frozen || !errors.Is(loadErr, errLocalTypeRBManifestChanged) {
			return nil, loadErr
		}
	}
	if options.Offline && hasRemoteTypeRBRequirement(config.Packages) {
		return nil, errors.New("cannot resolve an unlocked TypeRB package while offline")
	}
	resolver := typeRBResolver{
		config:  config,
		lock:    &TypeRBLock{FormatVersion: typeRBLockFormat, ConfigChecksum: checksum, Imports: map[string]string{}, Packages: map[string]TypeRBLockedPackage{}},
		roots:   map[string]string{},
		loading: map[string]bool{},
	}
	aliases := sortedRequirements(config.Packages)
	for _, alias := range aliases {
		canonical, err := resolver.resolve(alias, config.Packages[alias], config.Root)
		if err != nil {
			return nil, fmt.Errorf("resolve TypeRB package %s: %w", alias, err)
		}
		resolver.lock.Imports[alias] = canonical
	}
	if err := WriteTypeRBLock(lockPath, resolver.lock); err != nil {
		return nil, err
	}
	return loadResolvedTypeRBPackages(config, resolver.lock)
}

func hasRemoteTypeRBRequirement(requirements map[string]project.PackageRequirement) bool {
	for _, requirement := range requirements {
		if strings.TrimSpace(requirement.Path) == "" {
			return true
		}
	}
	return false
}

// LoadTypeRBPackages reads installed packages without changing the lock or
// contacting a package source. Build, check, run, and REPL use this boundary.
func LoadTypeRBPackages(config *project.Config) (*TypeRBPackages, error) {
	if len(config.Packages) == 0 {
		return emptyTypeRBPackages(), nil
	}
	checksum, err := typeRBConfigChecksum(config)
	if err != nil {
		return nil, err
	}
	lock, err := ReadTypeRBLock(TypeRBLockPath(config))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("TypeRB packages are not installed; run trb install")
	}
	if err != nil {
		return nil, err
	}
	if lock.ConfigChecksum != checksum {
		return nil, errors.New("trb.lock does not match trbconfig.jsonc; run trb install")
	}
	return loadResolvedTypeRBPackages(config, lock)
}

func emptyTypeRBPackages() *TypeRBPackages {
	return &TypeRBPackages{Aliases: map[string]string{}, NativeDependencies: map[string]string{}}
}

type typeRBResolver struct {
	config  *project.Config
	lock    *TypeRBLock
	roots   map[string]string
	loading map[string]bool
}

func (r *typeRBResolver) resolve(alias string, requirement project.PackageRequirement, baseRoot string) (string, error) {
	root, locked, manifest, err := r.materialize(alias, requirement, baseRoot)
	if err != nil {
		return "", err
	}
	canonical := manifest.Name
	if existingRoot, exists := r.roots[canonical]; exists {
		if r.loading[canonical] {
			return "", fmt.Errorf("package dependency cycle involving %s", canonical)
		}
		existing := r.lock.Packages[canonical]
		if existing.Source != locked.Source || existing.Path != locked.Path || existing.Revision != locked.Revision || existing.Version != locked.Version {
			return "", fmt.Errorf("package %s resolves to incompatible requirements at %s and %s", canonical, existingRoot, root)
		}
		return canonical, nil
	}
	r.roots[canonical] = root
	r.lock.Packages[canonical] = locked
	if r.loading[canonical] {
		return canonical, nil
	}
	r.loading[canonical] = true
	for _, dependencyAlias := range sortedManifestDependencies(manifest.Packages) {
		dependency, err := r.resolve(dependencyAlias, manifest.Packages[dependencyAlias], root)
		if err != nil {
			return "", fmt.Errorf("%s dependency %s: %w", canonical, dependencyAlias, err)
		}
		current := r.lock.Packages[canonical]
		if current.Dependencies == nil {
			current.Dependencies = map[string]string{}
		}
		current.Dependencies[dependencyAlias] = dependency
		r.lock.Packages[canonical] = current
	}
	delete(r.loading, canonical)
	return canonical, nil
}

func (r *typeRBResolver) materialize(alias string, requirement project.PackageRequirement, baseRoot string) (string, TypeRBLockedPackage, *TypeRBManifest, error) {
	if requirement.Path != "" {
		root := requirement.Path
		if !filepath.IsAbs(root) {
			root = filepath.Join(baseRoot, root)
		}
		root, err := filepath.Abs(root)
		if err != nil {
			return "", TypeRBLockedPackage{}, nil, err
		}
		manifest, err := ReadTypeRBManifest(root)
		if err != nil {
			return "", TypeRBLockedPackage{}, nil, err
		}
		manifestChecksum, err := typeRBManifestChecksum(manifest)
		if err != nil {
			return "", TypeRBLockedPackage{}, nil, err
		}
		lockedPath, err := filepath.Rel(r.config.Root, root)
		if err != nil {
			lockedPath = root
		}
		locked := TypeRBLockedPackage{
			Version: manifest.Version, Path: filepath.ToSlash(lockedPath), ManifestChecksum: manifestChecksum, Dependencies: map[string]string{},
		}
		return root, locked, manifest, nil
	}
	source := strings.TrimSpace(requirement.Source)
	if source == "" {
		source = defaultPackageSource(alias)
	}
	if err := validatePackageSource(source); err != nil {
		return "", TypeRBLockedPackage{}, nil, err
	}
	version := strings.TrimSpace(requirement.Version)
	if version == "" {
		version = "latest"
	}
	ref, err := resolveGitRef(source, version)
	if err != nil {
		return "", TypeRBLockedPackage{}, nil, err
	}
	root, revision, checksum, err := fetchTypeRBPackage(r.config, source, ref)
	if err != nil {
		return "", TypeRBLockedPackage{}, nil, err
	}
	manifest, err := ReadTypeRBManifest(root)
	if err != nil {
		return "", TypeRBLockedPackage{}, nil, err
	}
	if strings.HasPrefix(ref, "refs/tags/") {
		tagVersion := strings.TrimPrefix(ref, "refs/tags/")
		if canonicalVersion(manifest.Version) != tagVersion {
			return "", TypeRBLockedPackage{}, nil, fmt.Errorf("manifest version %s does not match Git tag %s", manifest.Version, tagVersion)
		}
	}
	locked := TypeRBLockedPackage{
		Version: manifest.Version, Source: source, Revision: revision, Checksum: checksum, Dependencies: map[string]string{},
	}
	return root, locked, manifest, nil
}

func loadResolvedTypeRBPackages(config *project.Config, lock *TypeRBLock) (*TypeRBPackages, error) {
	result := &TypeRBPackages{
		Lock: lock, Aliases: map[string]string{}, NativeDependencies: map[string]string{}, Packages: make([]TypeRBResolvedPackage, 0, len(lock.Packages)),
	}
	for alias, canonical := range lock.Imports {
		result.Aliases[alias] = canonical
	}
	names := make([]string, 0, len(lock.Packages))
	for name := range lock.Packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		locked := lock.Packages[name]
		root, err := lockedPackageRoot(config, locked)
		if err != nil {
			return nil, fmt.Errorf("load TypeRB package %s: %w", name, err)
		}
		if locked.Checksum != "" {
			checksum, err := directoryChecksum(root)
			if err != nil {
				return nil, fmt.Errorf("verify TypeRB package %s: %w", name, err)
			}
			if checksum != locked.Checksum {
				return nil, fmt.Errorf("TypeRB package %s checksum mismatch: lock has %s, cache has %s", name, locked.Checksum, checksum)
			}
		}
		manifest, err := ReadTypeRBManifest(root)
		if err != nil {
			return nil, err
		}
		if locked.Path != "" {
			manifestChecksum, err := typeRBManifestChecksum(manifest)
			if err != nil {
				return nil, err
			}
			if manifestChecksum != locked.ManifestChecksum {
				return nil, fmt.Errorf("%w: %s; run trb install", errLocalTypeRBManifestChanged, name)
			}
		}
		if manifest.Name != name || manifest.Version != locked.Version {
			return nil, fmt.Errorf("locked package %s@%s contains %s@%s", name, locked.Version, manifest.Name, manifest.Version)
		}
		if !manifest.Supports(config.Mode) {
			return nil, fmt.Errorf("TypeRB package %s does not support mode %s", name, config.Mode)
		}
		for dependency, version := range manifest.NativeDependenciesFor(config.Mode) {
			if existing, exists := result.NativeDependencies[dependency]; exists && existing != version {
				return nil, fmt.Errorf("TypeRB packages require conflicting versions of %s: %s and %s", dependency, existing, version)
			}
			result.NativeDependencies[dependency] = version
		}
		if adapter := manifest.DeclarationAdapterFor(config.Mode); adapter != "" {
			result.DeclarationAdapters = append(result.DeclarationAdapters, DeclarationAdapter{
				Package: name, Mode: config.Mode, Path: filepath.Join(root, adapter), Dependencies: manifest.NativeDependenciesFor(config.Mode),
			})
		}
		if provider := manifest.DeclarationProviderFor(config.Mode); provider != "" {
			result.DeclarationProviders = append(result.DeclarationProviders, DeclarationProvider{
				Package: name, Mode: config.Mode, Module: name, Path: filepath.Join(root, provider),
			})
		}
		if adapter := manifest.RuntimeAdapterFor(config.Mode); adapter != "" {
			result.RuntimeAdapters = append(result.RuntimeAdapters, RuntimeAdapter{
				Package: name, Mode: config.Mode, Path: filepath.Join(root, adapter), Dependencies: manifest.NativeDependenciesFor(config.Mode),
			})
		}
		result.Packages = append(result.Packages, TypeRBResolvedPackage{Name: name, Root: root, Manifest: manifest})
	}
	return result, nil
}

func ensureTypeRBPackageCache(config *project.Config, lock *TypeRBLock, offline bool) error {
	names := make([]string, 0, len(lock.Packages))
	for name := range lock.Packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		locked := lock.Packages[name]
		if locked.Path != "" {
			continue
		}
		root, err := lockedPackageRoot(config, locked)
		if err == nil {
			checksum, checksumErr := directoryChecksum(root)
			if checksumErr == nil && checksum == locked.Checksum {
				continue
			}
			if checksumErr == nil {
				return fmt.Errorf("TypeRB package %s checksum mismatch in cache", name)
			}
		}
		if offline {
			return fmt.Errorf("TypeRB package %s is not available in the offline cache", name)
		}
		fetchedRoot, revision, checksum, err := fetchTypeRBPackage(config, locked.Source, locked.Revision)
		if err != nil {
			return fmt.Errorf("fetch locked TypeRB package %s: %w", name, err)
		}
		if revision != locked.Revision || checksum != locked.Checksum {
			return fmt.Errorf("fetched TypeRB package %s does not match trb.lock", name)
		}
		manifest, err := ReadTypeRBManifest(fetchedRoot)
		if err != nil {
			return err
		}
		if manifest.Name != name || manifest.Version != locked.Version {
			return fmt.Errorf("fetched package identity %s@%s does not match %s@%s", manifest.Name, manifest.Version, name, locked.Version)
		}
	}
	return nil
}

func lockedPackageRoot(config *project.Config, locked TypeRBLockedPackage) (string, error) {
	if locked.Path != "" {
		root := filepath.FromSlash(locked.Path)
		if !filepath.IsAbs(root) {
			root = filepath.Join(config.Root, root)
		}
		return filepath.Abs(root)
	}
	cacheName, err := checksumCacheName(locked.Checksum)
	if err != nil {
		return "", err
	}
	root := filepath.Join(config.Root, ".trb", "packages", cacheName)
	if _, err := os.Stat(root); err != nil {
		return "", err
	}
	return root, nil
}

func fetchTypeRBPackage(config *project.Config, source, ref string) (string, string, string, error) {
	cacheRoot := filepath.Join(config.Root, ".trb", "packages")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return "", "", "", err
	}
	temporary, err := os.MkdirTemp(cacheRoot, ".fetch-*")
	if err != nil {
		return "", "", "", err
	}
	defer os.RemoveAll(temporary)
	repository := filepath.Join(temporary, "repository")
	if err := runGit("", "init", "--quiet", repository); err != nil {
		return "", "", "", err
	}
	if err := runGit(repository, "remote", "add", "origin", gitSourceURL(source)); err != nil {
		return "", "", "", err
	}
	if err := runGit(repository, "fetch", "--quiet", "--depth", "1", "origin", ref); err != nil {
		return "", "", "", err
	}
	if err := runGit(repository, "checkout", "--quiet", "--detach", "FETCH_HEAD"); err != nil {
		return "", "", "", err
	}
	revisionBytes, err := gitOutput(repository, "rev-parse", "HEAD")
	if err != nil {
		return "", "", "", err
	}
	revision := strings.TrimSpace(string(revisionBytes))
	if err := os.RemoveAll(filepath.Join(repository, ".git")); err != nil {
		return "", "", "", err
	}
	checksum, err := directoryChecksum(repository)
	if err != nil {
		return "", "", "", err
	}
	cacheName, err := checksumCacheName(checksum)
	if err != nil {
		return "", "", "", err
	}
	destination := filepath.Join(cacheRoot, cacheName)
	if _, err := os.Stat(destination); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(repository, destination); err != nil {
			return "", "", "", err
		}
	} else if err != nil {
		return "", "", "", err
	} else {
		existing, err := directoryChecksum(destination)
		if err != nil || existing != checksum {
			return "", "", "", fmt.Errorf("content-addressed package cache %s is invalid", destination)
		}
	}
	return destination, revision, checksum, nil
}

func resolveGitRef(source, version string) (string, error) {
	if version != "latest" {
		if validPackageVersion(version) {
			return "refs/tags/" + canonicalVersion(version), nil
		}
		if isGitRevision(version) {
			return version, nil
		}
		return "", fmt.Errorf("unsupported package version %q", version)
	}
	output, err := gitOutput("", "ls-remote", "--tags", "--refs", gitSourceURL(source))
	if err != nil {
		return "", err
	}
	latest := ""
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !strings.HasPrefix(fields[1], "refs/tags/") {
			continue
		}
		tag := strings.TrimPrefix(fields[1], "refs/tags/")
		if !semver.IsValid(tag) {
			continue
		}
		if latest == "" || semver.Compare(tag, latest) > 0 {
			latest = tag
		}
	}
	if latest == "" {
		return "", fmt.Errorf("package source %s has no semantic version tags", source)
	}
	return "refs/tags/" + latest, nil
}

func defaultPackageSource(alias string) string {
	first := strings.SplitN(alias, "/", 2)[0]
	if strings.Contains(first, ".") {
		return alias
	}
	return "github.com/" + alias
}

func gitSourceURL(source string) string {
	if strings.Contains(source, "://") || strings.HasPrefix(source, "git@") || filepath.IsAbs(source) {
		return source
	}
	return "https://" + strings.TrimSuffix(source, ".git") + ".git"
}

func isGitRevision(value string) bool {
	if len(value) < 7 || len(value) > 40 {
		return false
	}
	for _, character := range value {
		if !unicode.IsDigit(character) && (character < 'a' || character > 'f') && (character < 'A' || character > 'F') {
			return false
		}
	}
	return true
}

func sortedRequirements(values map[string]project.PackageRequirement) []string {
	result := make([]string, 0, len(values))
	for name := range values {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func runGit(directory string, arguments ...string) error {
	command := exec.Command("git", arguments...)
	command.Dir = directory
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("git %s: %s", strings.Join(arguments, " "), message)
	}
	return nil
}

func gitOutput(directory string, arguments ...string) ([]byte, error) {
	command := exec.Command("git", arguments...)
	command.Dir = directory
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(arguments, " "), message)
	}
	return output, nil
}
