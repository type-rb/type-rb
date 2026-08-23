package runtimeadapterhost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadUsesStrictPackageOwnedRuntimeBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	data := `{
  "protocolVersion": 1,
  "bindings": {
    "github.com/acme/aws-s3/native#head_object": {
      "dependency": "github.com/acme/aws-s3-wire",
      "module": "github.com/acme/aws-s3-wire/s3",
      "symbol": "HeadObject",
      "callConvention": "function",
      "maySuspend": true,
      "propagatesExecutionScope": true
    }
  }
}
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog, err := Load([]Source{{
		Package: "github.com/acme/aws-s3", Mode: "go", Path: path,
		Dependencies: map[string]string{"github.com/acme/aws-s3-wire": "v0.1.0"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := catalog.Lookup("github.com/acme/aws-s3", "go", "github.com/acme/aws-s3/native#head_object")
	if !ok || binding.Module != "github.com/acme/aws-s3-wire/s3" || binding.Symbol != "HeadObject" || !binding.MaySuspend || !binding.PropagatesExecutionScope {
		t.Fatalf("unexpected runtime binding: %#v, %t", binding, ok)
	}

	if err := os.WriteFile(path, []byte(data+` {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "trailing JSON content") {
		t.Fatalf("expected trailing content diagnostic, got %v", err)
	}
	unknown := strings.Replace(data, `"callConvention": "function",`, `"callConvention": "function", "unknown": true,`, 1)
	if err := os.WriteFile(path, []byte(unknown), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), `unknown field "unknown"`) {
		t.Fatalf("expected unknown field diagnostic, got %v", err)
	}
}

func TestLoadRejectsNamespaceAndDependencyEscapes(t *testing.T) {
	tests := []struct {
		name     string
		identity string
		depends  map[string]string
		want     string
	}{
		{name: "namespace", identity: "github.com/other/native#invoke", depends: map[string]string{"wire": "1.0.0"}, want: "outside the package namespace"},
		{name: "dependency", identity: "github.com/acme/runtime/native#invoke", depends: map[string]string{}, want: "selects undeclared go dependency wire"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runtime.json")
			data := `{"protocolVersion":1,"bindings":{"` + test.identity + `":{"dependency":"wire","module":"wire","symbol":"Invoke","callConvention":"function"}}}`
			if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load([]Source{{Package: "github.com/acme/runtime", Mode: "go", Path: path, Dependencies: test.depends}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestLoadRejectsUnsupportedModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	data := `{"protocolVersion":1,"bindings":{"github.com/acme/runtime/native#invoke":{"dependency":"wire","module":"wire","symbol":"Invoke","callConvention":"function"}}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load([]Source{{Package: "github.com/acme/runtime", Mode: "python", Path: path, Dependencies: map[string]string{"wire": "1.0.0"}}})
	if err == nil || !strings.Contains(err.Error(), `unsupported mode "python"`) {
		t.Fatalf("expected unsupported mode diagnostic, got %v", err)
	}
}
