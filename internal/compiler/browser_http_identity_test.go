package compiler

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func browserIdentityUnits(bodyName, source string) []SourceUnit {
	model := SourceUnit{
		Filename: "/project/src/contracts/models.trb", ModulePath: "contracts/models",
		Source: []byte(fmt.Sprintf(`record %s
	title: String
end

record Input
	body: %s
end
`, bodyName, bodyName)),
	}
	consumer := SourceUnit{
		Filename: "/project/src/client.trb", ModulePath: "client",
		Source: []byte(source),
	}
	return []SourceUnit{model, consumer}
}

func TestBrowserTransportBodyKeepsItsDefiningModule(t *testing.T) {
	for _, bodyName := range []string{"Body", "Headers", "HttpMethod", "Response", "RequestError", "RequestBody", "NoBody", "HttpClient"} {
		t.Run(bodyName, func(t *testing.T) {
			units := browserIdentityUnits(bodyName, `import { Input } from contracts/models
import { HttpClient, RequestError, RequestErrorKind } from trb/platform/typescript/browser
import trb/std/result

def request(client: HttpClient, _input: Input): Result<Integer, RequestError>
	raw := try client.request("/items")
	return Result<Integer, RequestError>::Err(RequestError.new(RequestErrorKind::Contract, "unexpected status", raw))
end

def describe_failure(client: HttpClient, input: Input): String
	case request(client, input)
	when Result::Ok(value)
		return value.to_s()
	when Result::Err(error)
		raw := error.response
		if raw != nil
			return error.message + ":" + raw.status.to_s() + ":" + raw.text().body
		end
		return error.message
	end
end
`)
			artifacts, err := CompileProject(units, Options{
				Mode: "typescript", TypeScriptRuntime: "browser", ProjectRoot: "/project", SourceRoot: "/project/src",
			})
			if err != nil {
				t.Fatalf("unrelated application %s changed the transport contract: %v", bodyName, err)
			}
			checkTypeScriptArtifacts(t, artifacts, "browser_identity_"+bodyName)
			checkBrowserIdentityExecution(t, artifacts)
		})
	}
}

func checkBrowserIdentityExecution(t *testing.T, artifacts []*Artifact) {
	t.Helper()
	harness := `import { describe_failure } from "./client.ts";
import { HttpClient } from "./trb/platform/typescript/browser/index.ts";
const client = new HttpClient("https://example.test");
const input = { body: { title: "application value" } };
let calls = 0;
globalThis.fetch = async (url) => {
  if (String(url) !== "https://example.test/items") throw new Error("wrong URL");
  calls++;
  return new Response("conflict", { status: 409 });
};
console.log(await describe_failure(undefined, client, input));
globalThis.fetch = async () => { calls++; throw new Error("offline"); };
console.log(await describe_failure(undefined, client, input));
if (calls !== 2) throw new Error("wrong request count");
`
	checkIdentityTypeScriptExecution(t, artifacts, harness, "unexpected status:409:conflict\noffline\n")
}

func checkIdentityTypeScriptExecution(t *testing.T, artifacts []*Artifact, harness, want string) {
	t.Helper()
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun is not installed")
	}
	root := t.TempDir()
	writeProjectRuntimeArtifacts(t, root, artifacts)
	writeCompilerRuntimeFile(t, filepath.Join(root, "runner.ts"), []byte(harness))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "bun", "run", "runner.ts")
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil || string(output) != want {
		t.Fatalf("TypeScript identity execution: %v\n%s\nwant %q", err, output, want)
	}
}

func TestTransitiveCodecEnumIdentityDoesNotShadowLocalEnum(t *testing.T) {
	units := []SourceUnit{
		{Filename: "/project/src/models.trb", ModulePath: "models", Source: []byte(`enum Status
	Remote = "REMOTE"
end
record User
	status: Status
end
`)},
		{Filename: "/project/src/client.trb", ModulePath: "client", Source: []byte(`import { User } from models
import trb/std/json
import trb/std/result
enum Status
	Local = "LOCAL"
end
def decode(source: String): Result<User, JSON::Error>
	return JSON.decode<User>(source)
end
def local_status(): Status
	return Status::Local
end
`)},
	}
	artifacts, err := CompileProject(units, Options{
		Mode: "typescript", TypeScriptRuntime: "browser", ProjectRoot: "/project", SourceRoot: "/project/src",
	})
	if err != nil {
		t.Fatal(err)
	}
	checkTypeScriptArtifacts(t, artifacts, "transitive_codec_identity")
	checkIdentityTypeScriptExecution(t, artifacts, `import { decode, local_status } from "./client.ts";
const decoded = decode('{"status":"REMOTE"}');
if (decoded.kind !== "Ok") throw new Error("decode failed");
console.log(decoded.value.status);
console.log(local_status());
`, "REMOTE\nLOCAL\n")
}

func TestBrowserTransportBodyPreservesNullableAliases(t *testing.T) {
	units := browserIdentityUnits("Body", `import { Input } from contracts/models
import { Body as TransportBody } from trb/http
import { RequestError, RequestErrorKind, Response } from trb/platform/typescript/browser

def wrap(raw: Response<TransportBody>, _input: Input): RequestError
	optional: Response<TransportBody>? := raw
	return RequestError.new(RequestErrorKind::Contract, "unexpected status", optional)
end
`)
	artifacts, err := CompileProject(units, Options{
		Mode: "typescript", TypeScriptRuntime: "browser", ProjectRoot: "/project", SourceRoot: "/project/src",
	})
	if err != nil {
		t.Fatal(err)
	}
	checkTypeScriptArtifacts(t, artifacts, "browser_nullable_identity")
}

func TestBrowserTransportBodyRejectsUnrelatedApplicationValues(t *testing.T) {
	for _, test := range []struct{ name, source, want string }{
		{"error response", `import { Body } from contracts/models
import { RequestError, RequestErrorKind, Response } from trb/platform/typescript/browser
def rejected(raw: Response<Body>): RequestError
	return RequestError.new(RequestErrorKind::Contract, "unexpected status", raw)
end
`, "argument 3 to RequestError()"},
		{"response decoder", `import { Body } from contracts/models
import { Response } from trb/platform/typescript/browser
def rejected(raw: Response<Body>): Response<String>
	return raw.text()
end
`, "has no member text"},
		{"request headers", `import { Headers } from contracts/models
import { HttpClient, RequestError, Response } from trb/platform/typescript/browser
import { Body } from trb/http
import trb/std/result
def rejected(client: HttpClient, headers: Headers): Result<Response<Body>, RequestError>
	return client.request("/items", headers: headers)
end
`, "argument 2 to request()"},
	} {
		t.Run(test.name, func(t *testing.T) {
			bodyName := "Body"
			if test.name == "request headers" {
				bodyName = "Headers"
			}
			_, err := CompileProject(browserIdentityUnits(bodyName, test.source), Options{
				Mode: "typescript", TypeScriptRuntime: "browser", ProjectRoot: "/project", SourceRoot: "/project/src",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want diagnostic containing %q", err, test.want)
			}
		})
	}
}
