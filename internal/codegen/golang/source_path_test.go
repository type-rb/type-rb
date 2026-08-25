package golang

import "testing"

func TestGeneratedSourceDirectoryEncodesDynamicRouteSegments(t *testing.T) {
	tests := map[string]string{
		"":                                       "",
		".":                                      ".",
		"routes/accounts":                        "routes/accounts",
		"routes/accounts/[id]/preferences":       "routes/accounts/route_param_id/preferences",
		"routes/assets/[...path]":                "routes/assets/route_catch_all_path",
		"routes/accounts/[invalid-name]/details": "routes/accounts/[invalid-name]/details",
	}
	for source, expected := range tests {
		if actual := GeneratedSourceDirectory(source); actual != expected {
			t.Errorf("GeneratedSourceDirectory(%q) = %q, want %q", source, actual, expected)
		}
	}
}
