package cli

import "testing"

func TestResolveBuildVersion(t *testing.T) {
	tests := []struct {
		name          string
		sourceVersion string
		modulePath    string
		moduleVersion string
		want          string
	}{
		{
			name:          "tagged module install",
			sourceVersion: "0.3.26-dev",
			modulePath:    typeRBModulePath,
			moduleVersion: "v0.3.25",
			want:          "0.3.25",
		},
		{
			name:          "tagged prerelease install",
			sourceVersion: "0.3.26-dev",
			modulePath:    typeRBModulePath,
			moduleVersion: "v0.3.25-rc.1",
			want:          "0.3.25-rc.1",
		},
		{
			name:          "main branch install",
			sourceVersion: "0.3.26-dev",
			modulePath:    typeRBModulePath,
			moduleVersion: "v0.3.26-0.20260824081740-c1661ba920ba",
			want:          "0.3.26-dev",
		},
		{
			name:          "local source build",
			sourceVersion: "0.3.26-dev",
			modulePath:    typeRBModulePath,
			moduleVersion: "(devel)",
			want:          "0.3.26-dev",
		},
		{
			name:          "release linker override",
			sourceVersion: "0.3.25",
			modulePath:    typeRBModulePath,
			moduleVersion: "(devel)",
			want:          "0.3.25",
		},
		{
			name:          "different module",
			sourceVersion: "0.3.26-dev",
			modulePath:    "example.com/fork/type-rb",
			moduleVersion: "v0.3.25",
			want:          "0.3.26-dev",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveBuildVersion(test.sourceVersion, test.modulePath, test.moduleVersion); got != test.want {
				t.Fatalf("resolveBuildVersion()=%q, want %q", got, test.want)
			}
		})
	}
}
