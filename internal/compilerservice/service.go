// Package compilerservice owns reusable, editor-independent project snapshots.
// CLI, LSP, and future agent adapters can share overlays, diagnostics, checked
// artifacts, and language-service contexts without rebuilding compiler phases.
package compilerservice

import (
	"bytes"
	"path/filepath"
	"sync"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/languageservice"
)

type Snapshot struct {
	Version     uint64
	Diagnostics []diagnostic.Diagnostic
	Artifacts   []*compiler.Artifact
	Stale       bool
	contexts    map[string]languageservice.Context
}

func (s Snapshot) HasErrors() bool {
	for _, item := range s.Diagnostics {
		if item.Severity == diagnostic.Error {
			return true
		}
	}
	return false
}

func (s Snapshot) Context(modulePath string) (languageservice.Context, bool) {
	context, ok := s.contexts[modulePath]
	return context, ok
}

// Service maintains immutable snapshots over project inputs and unsaved
// document overlays. Any input change invalidates the current analysis.
type Service struct {
	mu         sync.Mutex
	base       []compiler.SourceUnit
	options    compiler.Options
	overlays   map[string]compiler.SourceUnit
	generation uint64
	snapshot   *Snapshot
	lastGood   []*compiler.Artifact
	contexts   map[string]languageservice.Context
}

func New(units []compiler.SourceUnit, options compiler.Options) *Service {
	return &Service{
		base: cloneUnits(units), options: options, overlays: map[string]compiler.SourceUnit{},
		generation: 1, contexts: map[string]languageservice.Context{},
	}
}

// SetDocument installs an unsaved source unit. Existing project files and new
// files use the same operation; adapters are responsible for deriving the
// module and package names of a newly created file.
func (s *Service) SetDocument(unit compiler.SourceUnit) {
	unit = cloneUnit(unit)
	key := cleanPath(unit.Filename)
	unit.Filename = key

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, base := range s.base {
		base.Filename = cleanPath(base.Filename)
		if base.Filename != key || !equalUnit(base, unit) {
			continue
		}
		if _, exists := s.overlays[key]; exists {
			delete(s.overlays, key)
			s.generation++
			s.snapshot = nil
		}
		return
	}
	if previous, exists := s.overlays[key]; exists && equalUnit(previous, unit) {
		return
	}
	s.overlays[key] = unit
	s.generation++
	s.snapshot = nil
}

// SetWorkspaceDocument updates the on-disk project input beneath any open
// editor overlay. Closing an overlay therefore restores the latest saved file.
func (s *Service) SetWorkspaceDocument(unit compiler.SourceUnit) {
	unit = cloneUnit(unit)
	key := cleanPath(unit.Filename)
	unit.Filename = key

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.base {
		if cleanPath(s.base[index].Filename) != key {
			continue
		}
		previous := cloneUnit(s.base[index])
		previous.Filename = key
		if equalUnit(previous, unit) {
			return
		}
		s.base[index] = unit
		s.generation++
		s.snapshot = nil
		return
	}
	s.base = append(s.base, unit)
	s.generation++
	s.snapshot = nil
}

// RemoveWorkspaceDocument removes an on-disk project input while preserving an
// open editor overlay until the editor closes it.
func (s *Service) RemoveWorkspaceDocument(filename string) {
	key := cleanPath(filename)
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.base {
		if cleanPath(s.base[index].Filename) != key {
			continue
		}
		s.base = append(s.base[:index], s.base[index+1:]...)
		s.generation++
		s.snapshot = nil
		return
	}
}

// CloseDocument removes an unsaved overlay and restores the on-disk project
// unit, if one exists.
func (s *Service) CloseDocument(filename string) {
	key := cleanPath(filename)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.overlays[key]; !exists {
		return
	}
	delete(s.overlays, key)
	s.generation++
	s.snapshot = nil
}

// Analyze returns the current immutable project snapshot. Compilation happens
// outside the service lock. If inputs change concurrently, the obsolete result
// is discarded and analysis restarts from the newer generation.
func (s *Service) Analyze() Snapshot {
	for {
		s.mu.Lock()
		if s.snapshot != nil {
			result := *s.snapshot
			s.mu.Unlock()
			return result
		}
		generation := s.generation
		units := s.currentUnitsLocked()
		options := s.options
		lastGood := append([]*compiler.Artifact(nil), s.lastGood...)
		lastContexts := s.contexts
		s.mu.Unlock()

		artifacts, err := compiler.CompileProject(units, options)
		diagnostics := diagnosticsFor(err)
		contexts := lastContexts
		stale := err != nil && len(lastGood) > 0
		visibleArtifacts := lastGood
		if err == nil {
			visibleArtifacts = artifacts
			contexts = buildContexts(artifacts)
			stale = false
		}
		result := Snapshot{
			Version: generation, Diagnostics: diagnostics,
			Artifacts: append([]*compiler.Artifact(nil), visibleArtifacts...),
			Stale:     stale, contexts: contexts,
		}

		s.mu.Lock()
		if s.generation != generation {
			s.mu.Unlock()
			continue
		}
		if err == nil {
			s.lastGood = append([]*compiler.Artifact(nil), artifacts...)
			s.contexts = contexts
		}
		s.snapshot = &result
		s.mu.Unlock()
		return result
	}
}

func (s *Service) currentUnitsLocked() []compiler.SourceUnit {
	units := cloneUnits(s.base)
	indexes := make(map[string]int, len(units))
	for index := range units {
		units[index].Filename = cleanPath(units[index].Filename)
		indexes[units[index].Filename] = index
	}
	for filename, overlay := range s.overlays {
		if index, exists := indexes[filename]; exists {
			units[index] = cloneUnit(overlay)
		} else {
			units = append(units, cloneUnit(overlay))
		}
	}
	return units
}

func diagnosticsFor(err error) []diagnostic.Diagnostic {
	if err == nil {
		return nil
	}
	if compilation, ok := err.(*compiler.CompileError); ok {
		return append([]diagnostic.Diagnostic(nil), compilation.Diagnostics...)
	}
	return []diagnostic.Diagnostic{{Code: diagnostic.ProjectError, Severity: diagnostic.Error, Message: err.Error()}}
}

func buildContexts(artifacts []*compiler.Artifact) map[string]languageservice.Context {
	programs := make([]*ir.Program, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact != nil && artifact.IR != nil {
			programs = append(programs, artifact.IR)
		}
	}
	return languageservice.BuildContexts(programs)
}

func cloneUnits(units []compiler.SourceUnit) []compiler.SourceUnit {
	result := make([]compiler.SourceUnit, len(units))
	for index, unit := range units {
		result[index] = cloneUnit(unit)
	}
	return result
}

func cloneUnit(unit compiler.SourceUnit) compiler.SourceUnit {
	unit.Source = append([]byte(nil), unit.Source...)
	if unit.PackageAliases != nil {
		unit.PackageAliases = cloneStrings(unit.PackageAliases)
	}
	return unit
}

func cloneStrings(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func equalUnit(left, right compiler.SourceUnit) bool {
	return left.Filename == right.Filename && left.ModulePath == right.ModulePath && left.Package == right.Package &&
		left.CompilerOwned == right.CompilerOwned && left.Official == right.Official && left.ExternalPackage == right.ExternalPackage &&
		bytes.Equal(left.Source, right.Source)
}

func cleanPath(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		return absolute
	}
	return filepath.Clean(path)
}
