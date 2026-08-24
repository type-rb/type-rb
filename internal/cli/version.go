package cli

import (
	"runtime/debug"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

const typeRBModulePath = "github.com/type-rb/type-rb"

func init() {
	if build, ok := debug.ReadBuildInfo(); ok {
		Version = resolveBuildVersion(Version, build.Main.Path, build.Main.Version)
	}
}

func resolveBuildVersion(sourceVersion, modulePath, moduleVersion string) string {
	if !strings.HasSuffix(sourceVersion, "-dev") || modulePath != typeRBModulePath {
		return sourceVersion
	}
	if semver.Canonical(moduleVersion) != moduleVersion || module.IsPseudoVersion(moduleVersion) {
		return sourceVersion
	}
	return strings.TrimPrefix(moduleVersion, "v")
}
