package official

import (
	"encoding/json"
	"testing"
	"testing/fstest"
)

func TestManifestDeclaresPlatformTargets(t *testing.T) {
	packages, err := loadFromFS(fstest.MapFS{
		"packages/browser/trbpackage.json": &fstest.MapFile{Data: []byte(`{
  "name": "example/browser",
  "version": "0.1.0",
  "module": "example/browser/index",
  "source": "src/index.trb",
  "kind": "platform",
  "targets": ["typescript"]
}`)},
		"packages/browser/src/index.trb": &fstest.MapFile{Data: []byte("# browser package\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := packages["example/browser"].Definition
	if definition.Kind != "platform" || !definition.Supports("typescript") || definition.Supports("go") || definition.Supports("ruby") {
		t.Fatalf("unexpected platform package definition: %#v", definition)
	}
}

func TestManifestRejectsInvalidPackageBoundary(t *testing.T) {
	tests := []struct {
		name     string
		boundary string
	}{
		{name: "kind", boundary: `"kind": "native"`},
		{name: "target", boundary: `"targets": ["python"]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadFromFS(fstest.MapFS{
				"packages/example/trbpackage.json": &fstest.MapFile{Data: []byte(`{
  "name": "example/package",
  "version": "0.1.0",
  "module": "example/package/index",
  "source": "src/index.trb",
  ` + test.boundary + `
}`)},
				"packages/example/src/index.trb": &fstest.MapFile{Data: []byte("# package\n")},
			})
			if err == nil {
				t.Fatal("expected invalid package boundary to be rejected")
			}
		})
	}
}

func TestBundledReactPackage(t *testing.T) {
	packageDefinition, ok := Lookup("trb/platform/typescript/react")
	if !ok {
		t.Fatal("React package is not registered")
	}
	if packageDefinition.Definition.Kind != "platform" || !packageDefinition.Definition.Supports("typescript") || packageDefinition.Definition.Supports("go") {
		t.Fatalf("unexpected React package targets: %#v", packageDefinition.Definition)
	}
	dependencies, err := packageDefinition.NativeDependenciesFor("typescript", json.RawMessage(nil))
	if err != nil {
		t.Fatal(err)
	}
	if dependencies["react"] != "latest" || dependencies["react-dom"] != "latest" {
		t.Fatalf("unexpected React dependencies: %#v", dependencies)
	}
	if packageDefinition.Definition.Symbols["mount"].Intrinsic != "trb.platform.typescript.react.mount" {
		t.Fatalf("React semantic provider was not loaded: %#v", packageDefinition.Definition.Symbols)
	}
	if packageDefinition.Definition.TypeProvider != "trb.typescript.react" || packageDefinition.Definition.JSX == nil {
		t.Fatalf("React type and JSX providers were not loaded: %#v", packageDefinition.Definition)
	}
	onClick := packageDefinition.Definition.JSX.IntrinsicAttributes["onClick"]
	if onClick.String() != "(MouseEvent) -> Void" {
		t.Fatalf("unexpected onClick contract: %s", onClick)
	}
}

func TestBundledWebPackage(t *testing.T) {
	packageDefinition, ok := Lookup("trb/web")
	if !ok {
		t.Fatal("trb/web is not registered")
	}
	if packageDefinition.Version != "0.1.0" {
		t.Fatalf("version = %q", packageDefinition.Version)
	}
	if packageDefinition.Definition.ModulePath != "trb/web/index" {
		t.Fatalf("module = %q", packageDefinition.Definition.ModulePath)
	}
	if packageDefinition.Definition.Kind != "portable" || !packageDefinition.Definition.Supports("go") || !packageDefinition.Definition.Supports("ruby") || !packageDefinition.Definition.Supports("typescript") {
		t.Fatalf("unexpected default package boundary: %#v", packageDefinition.Definition)
	}
	if packageDefinition.ProjectProvider != "trb.web.routes" {
		t.Fatalf("project provider = %q", packageDefinition.ProjectProvider)
	}
	if packageDefinition.Definition.Source == "" {
		t.Fatal("package source is empty")
	}
	requestJSON := packageDefinition.Definition.Symbols["request_json"]
	if requestJSON.Intrinsic != "trb.web.request_json" || requestJSON.Return.String() != "Result<T, RequestError>" {
		t.Fatalf("unexpected request_json contract: %#v", requestJSON)
	}
	json := packageDefinition.Definition.Symbols["json"]
	if json.Intrinsic != "trb.web.json" || len(json.Parameters) != 2 || !json.Parameters[1].Optional || json.Return.String() != "Response" {
		t.Fatalf("unexpected json contract: %#v", json)
	}
	configureServer := packageDefinition.Definition.Symbols["configure_server"]
	if configureServer.Intrinsic != "trb.web.configure_server" || configureServer.Return.String() != "ServerConfig" || len(configureServer.Parameters) != 4 {
		t.Fatalf("unexpected configure_server contract: %#v", configureServer)
	}
	for _, parameter := range configureServer.Parameters {
		if !parameter.Optional || !parameter.Keyword {
			t.Fatalf("configure_server parameter is not optional keyword-only: %#v", parameter)
		}
	}
	serve := packageDefinition.Definition.Symbols["serve"]
	if serve.Intrinsic != "trb.web.serve" || serve.Return.String() != "Void" || len(serve.Parameters) != 1 || serve.Parameters[0].Type.String() != "ServerConfig" || !serve.Parameters[0].Optional {
		t.Fatalf("unexpected serve contract: %#v", serve)
	}
}

func TestBundledWebTestingPackage(t *testing.T) {
	packageDefinition, ok := Lookup("trb/web/testing")
	if !ok {
		t.Fatal("trb/web/testing is not registered")
	}
	if packageDefinition.Definition.ModulePath != "trb/web/testing/index" {
		t.Fatalf("module = %q", packageDefinition.Definition.ModulePath)
	}
	dispatch := packageDefinition.Definition.Symbols["dispatch"]
	if dispatch.Intrinsic != "trb.web.testing.dispatch" || dispatch.Return.String() != "Response" {
		t.Fatalf("unexpected dispatch contract: %#v", dispatch)
	}
}

func TestBundledWebLoggerMiddlewarePackage(t *testing.T) {
	packageDefinition, ok := Lookup("trb/web/middleware/logger")
	if !ok {
		t.Fatal("trb/web/middleware/logger is not registered")
	}
	if packageDefinition.Definition.ModulePath != "trb/web/middleware/logger/index" {
		t.Fatalf("module = %q", packageDefinition.Definition.ModulePath)
	}
	if packageDefinition.Definition.Source == "" {
		t.Fatal("logger package source is empty")
	}
}

func TestBundledWebMiddlewarePackage(t *testing.T) {
	packageDefinition, ok := Lookup("trb/web/middleware")
	if !ok {
		t.Fatal("trb/web/middleware is not registered")
	}
	if packageDefinition.Definition.ModulePath != "trb/web/middleware/index" {
		t.Fatalf("module = %q", packageDefinition.Definition.ModulePath)
	}
	if packageDefinition.Definition.Source == "" {
		t.Fatal("middleware package source is empty")
	}
}

func TestBundledWebSecureHeadersMiddlewarePackage(t *testing.T) {
	packageDefinition, ok := Lookup("trb/web/middleware/secure_headers")
	if !ok {
		t.Fatal("trb/web/middleware/secure_headers is not registered")
	}
	if packageDefinition.Definition.ModulePath != "trb/web/middleware/secure_headers/index" {
		t.Fatalf("module = %q", packageDefinition.Definition.ModulePath)
	}
	if packageDefinition.Definition.Source == "" {
		t.Fatal("secure headers package source is empty")
	}
}

func TestBundledWebCORSMiddlewarePackage(t *testing.T) {
	packageDefinition, ok := Lookup("trb/web/middleware/cors")
	if !ok {
		t.Fatal("trb/web/middleware/cors is not registered")
	}
	if packageDefinition.Definition.ModulePath != "trb/web/middleware/cors/index" {
		t.Fatalf("module = %q", packageDefinition.Definition.ModulePath)
	}
	if packageDefinition.Definition.Source == "" {
		t.Fatal("CORS package source is empty")
	}
}

func TestBundledWebRequestIDMiddlewarePackage(t *testing.T) {
	packageDefinition, ok := Lookup("trb/web/middleware/request_id")
	if !ok {
		t.Fatal("trb/web/middleware/request_id is not registered")
	}
	if packageDefinition.Definition.ModulePath != "trb/web/middleware/request_id/index" {
		t.Fatalf("module = %q", packageDefinition.Definition.ModulePath)
	}
	if packageDefinition.Definition.Source == "" {
		t.Fatal("request ID package source is empty")
	}
}

func TestBundledTypeScriptBrowserPackage(t *testing.T) {
	packageDefinition, ok := Lookup("trb/platform/typescript/browser")
	if !ok {
		t.Fatal("trb/platform/typescript/browser is not registered")
	}
	definition := packageDefinition.Definition
	if definition.Kind != "platform" || !definition.Supports("typescript") || definition.Supports("go") || definition.Supports("ruby") {
		t.Fatalf("unexpected browser package boundary: %#v", definition)
	}
	request := definition.Symbols["request"]
	if request.Intrinsic != "trb.platform.typescript.browser.request" || request.Receiver.String() != "HttpClient" || request.Return.String() != "Response<Body>" || request.Fails.String() != "RequestError" {
		t.Fatalf("unexpected request contract: %#v", request)
	}
	json := definition.Symbols["json"]
	if json.Receiver.String() != "Response<Body>" || json.Return.String() != "Response<T>" || len(json.TypeParameters) != 1 || json.Fails.String() != "RequestError" {
		t.Fatalf("unexpected response JSON contract: %#v", json)
	}
	jsonBody := definition.Symbols["json_body"]
	if jsonBody.Return.String() != "RequestBody" || jsonBody.Fails.String() != "RequestError" || len(jsonBody.TypeParameters) != 1 {
		t.Fatalf("unexpected JSON body contract: %#v", jsonBody)
	}
}
