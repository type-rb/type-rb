package compiler

import (
	"strings"
	"testing"
)

func TestTypeScriptBrowserHTTPClient(t *testing.T) {
	source := []byte(`import {
	File,
	FileReadError,
	HttpClient,
	NoBody,
	RequestBody,
	RequestError,
	Response,
	json_body,
} from trb/platform/typescript/browser
import { Body, Header, Headers, HttpMethod } from trb/http
import { Result } from trb/std/result
import { QueryParameter } from trb/std/url

record Todo
	id: Integer
	title: String
end

record CreateTodoInput
	title: String
end

def fetch_todo(client: HttpClient, id: Integer): Result<Response<Todo>, RequestError>
	response := try client.request("/todos", query: [QueryParameter.new(name: "id", value: id.to_s())], headers: Headers.new([Header.new(name: "accept", value: "application/json")]), timeout_milliseconds: 1000)
	return response.json<Todo>()
end

def create_todo(client: HttpClient, input: CreateTodoInput): Result<Response<Todo>, RequestError>
	body := try json_body(input)
	raw := try client.request("/todos", method: HttpMethod.post(), body: body)
	return raw.json<Todo>()
end

def fetch_with_local_request_names(client: HttpClient, id: Integer): Result<Response<Todo>, RequestError>
	path := "/todos"
	query := [QueryParameter.new(name: "id", value: id.to_s())]
	headers := Headers.new([Header.new(name: "accept", value: "application/json")])
	timeout := 1000
	response := try client.request(path, query: query, headers: headers, timeout_milliseconds: timeout)
	return response.json<Todo>()
end

def raw_body(client: HttpClient): Result<Body, RequestError>
	response := try client.request("/health")
	return Result<Body, RequestError>::Ok(response.body)
end

def upload_file(client: HttpClient, file: File): Result<Response<Body>, RequestError>
	return client.request("/uploads", method: HttpMethod.put(), body: RequestBody::File(file))
end

def file_bytes(file: File): Result<Bytes, FileReadError>
	return file.read()
end

def file_text(file: File): Result<String, FileReadError>
	return file.read_text()
end

def text_response(raw: Response<Body>): Response<String>
	return raw.text()
end

def bytes_response(raw: Response<Body>): Response<Bytes>
	return raw.bytes()
end

def empty_response(raw: Response<Body>): Result<Response<NoBody>, RequestError>
	return raw.no_body()
end

def empty_named_response(response: Response<Body>): Result<Response<NoBody>, RequestError>
	return response.no_body()
end
`)

	artifact, err := Compile("browser_http.trb", source, "typescript")
	if err != nil {
		t.Fatalf("typescript rejected browser HTTP client: %v", err)
	}
	output := string(artifact.Output)
	for _, want := range []string{
		`import * as __trb_browser from "./trb/platform/typescript/browser/index.ts";`,
		`async function fetch_todo`,
		`globalThis.fetch`,
		`__trbNativeResponse.arrayBuffer()`,
		`__trb_browser.RequestErrorKind.Contract`,
		`new __trb_browser.RequestError(__trb_browser.RequestErrorKind.Contract, message, response)`,
		`JSON.parse`,
		`JSON.stringify`,
		`__trb_browser.Response<Todo>`,
		`expected an empty response body`,
		`case "File"`,
		`as unknown as globalThis.File`,
		`__trbRequestBody.value.type.length > 0`,
		`await __trbFile.arrayBuffer()`,
		`await __trbFile.text()`,
		`{ message: __trbMessage } satisfies __trb_browser.FileReadError`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "response.ok") {
		t.Fatalf("declared HTTP status responses must not be converted into transport errors:\n%s", output)
	}
	if strings.Contains(output, "const response = response") {
		t.Fatalf("browser response conversion shadows a source binding:\n%s", output)
	}
	for _, shadow := range []string{"const path = path", "const query = query", "const headers = headers", "const timeout: number | null = timeout"} {
		if strings.Contains(output, shadow) {
			t.Fatalf("browser request generation shadows a source binding with %q:\n%s", shadow, output)
		}
	}
	for _, want := range []string{"const __trbPath = path", "const __trbQuery = query", "const __trbRequestHeaders = headers", "const __trbTimeout: number | null = timeout"} {
		if !strings.Contains(output, want) {
			t.Fatalf("browser request generation is missing hygienic binding %q:\n%s", want, output)
		}
	}
}

func TestBrowserHTTPInferredTypesRemainUsableWithoutTypeImports(t *testing.T) {
	source := []byte(`import { HttpClient, RequestError } from trb/platform/typescript/browser
import { Result } from trb/std/result

record Todo
	id: Integer
	title: String
end

def title(client: HttpClient): Result<String, RequestError>
	raw := try client.request("/todo")
	response := try raw.json<Todo>()
	return Result<String, RequestError>::Ok(response.body.title)
end
`)
	artifact, err := Compile("browser_http_inferred.trb", source, "typescript")
	if err != nil {
		t.Fatalf("typescript rejected inferred browser types: %v", err)
	}
	output := string(artifact.Output)
	if !strings.Contains(output, "response.__trb_body.title") {
		t.Fatalf("inferred generic class fields were not specialized and lowered:\n%s", output)
	}
}

func TestBrowserHTTPResultFunctionCanSuspend(t *testing.T) {
	source := []byte(`import { HttpClient, RequestError, Response } from trb/platform/typescript/browser
import { Result } from trb/std/result

record Todo
	id: Integer
	title: String
end

def load(client: HttpClient): Result<Response<Todo>, RequestError>
	response := try client.request("/todos/1")
	return response.json<Todo>()
end
`)
	artifact, err := Compile("browser_http_callback.trb", source, "typescript")
	if err != nil {
		t.Fatalf("typescript rejected a suspending fallible function value: %v", err)
	}
	output := string(artifact.Output)
	for _, want := range []string{
		"async function load",
		"Promise<Result<__trb_browser.Response<Todo>, __trb_browser.RequestError>>",
		"globalThis.fetch",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated suspending callback is missing %q:\n%s", want, output)
		}
	}
}

func TestTypeScriptBrowserHTTPDefaultsAvoidInvalidStrictTemporaries(t *testing.T) {
	source := []byte(`import { HttpClient, RequestError, Response } from trb/platform/typescript/browser
import { Result } from trb/std/result

def load(client: HttpClient): Result<Response<String>, RequestError>
	response := try client.request("/message")
	return Result<Response<String>, RequestError>::Ok(response.text())
end
`)
	artifact, err := Compile("browser_http_defaults.trb", source, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, invalid := range []string{"const __trbQuery = [];", "const __trbRequestHeaders = null;"} {
		if strings.Contains(output, invalid) {
			t.Fatalf("generated browser defaults expose a temporary rejected by strict TypeScript %q:\n%s", invalid, output)
		}
	}
	if !strings.Contains(output, "globalThis.fetch") {
		t.Fatalf("generated browser request is missing fetch:\n%s", output)
	}
}

func TestOfficialReceiverMethodsRequireTheirImport(t *testing.T) {
	source := []byte(`class HttpClient
end

def fetch(client: HttpClient)
	client.request("/todo")
	return
end
`)
	if _, err := Compile("browser_http_missing_import.trb", source, "typescript"); err == nil || !strings.Contains(err.Error(), "class HttpClient has no instance member request") {
		t.Fatalf("browser receiver method was available without an import: %v", err)
	}
}

func TestTypeScriptBrowserPackageBuildsAsAnOfficialProjectUnit(t *testing.T) {
	source := SourceUnit{Filename: "main.trb", ModulePath: "app/main", Source: []byte(`import { HttpClient } from trb/platform/typescript/browser

def client(): HttpClient
	return HttpClient.new("https://api.example.test")
end
`)}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript", TypeScriptRuntime: "browser"})
	if err != nil {
		t.Fatalf("browser package project build failed: %v", err)
	}
	found := false
	for _, artifact := range artifacts {
		if artifact.IR.ModulePath != "trb/platform/typescript/browser/index" {
			continue
		}
		found = true
		output := string(artifact.Output)
		for _, want := range []string{"export interface File", "export class Response<T>", "export class RequestError", "export class HttpClient"} {
			if !strings.Contains(output, want) {
				t.Fatalf("generated browser package is missing %q:\n%s", want, output)
			}
		}
	}
	if !found {
		t.Fatal("official browser package artifact is missing")
	}
	foundHTTP := false
	for _, artifact := range artifacts {
		if artifact.IR.ModulePath == "trb/http/index" {
			foundHTTP = true
			if !strings.Contains(string(artifact.Output), "export class Headers") {
				t.Fatalf("generated HTTP package does not define Headers:\n%s", artifact.Output)
			}
		}
	}
	if !foundHTTP {
		t.Fatal("official trb/http package artifact is missing")
	}
}

func TestBrowserHTTPPackageIsTypeScriptOnly(t *testing.T) {
	source := []byte(`import { HttpClient } from trb/platform/typescript/browser

def client(): HttpClient
	return HttpClient.new()
end
`)
	for _, mode := range []string{"go", "ruby"} {
		if _, err := Compile("browser_http.trb", source, mode); err == nil || !strings.Contains(err.Error(), "does not support mode "+mode) {
			t.Fatalf("%s accepted the TypeScript browser package: %v", mode, err)
		}
	}
}
