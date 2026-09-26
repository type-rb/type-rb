package lsp

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
)

func TestServerPublishesDiagnosticsForTRBMode(t *testing.T) {
	filename := cleanPath("main.trb")
	valid := "def greet(name: String): String\n\treturn \"Hello, \" + name\nend\n"
	invalid := "def greet(name: String): String\n\treturn 1\nend\n"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "typerb", Version: 1, Text: valid}})},
		message{JSONRPC: "2.0", Method: "textDocument/didChange", Params: rawParams(t, didChangeParams{
			TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: 2}, ContentChanges: []contentChange{{Text: invalid}},
		})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "trb", Version: "test", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Source: []byte(valid)}},
		CompilerOptions: compiler.Options{Mode: "trb"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 4 {
		t.Fatalf("response count=%d, want 4: %s", len(frames), output.String())
	}
	var opened publishDiagnosticsParams
	decodeParamsFrame(t, frames[1], &opened)
	if len(opened.Diagnostics) != 0 {
		t.Fatalf("valid trb document diagnostics=%#v", opened.Diagnostics)
	}
	var changed publishDiagnosticsParams
	decodeParamsFrame(t, frames[2], &changed)
	if len(changed.Diagnostics) == 0 || changed.Diagnostics[0].Code != "TRB3000" {
		t.Fatalf("invalid trb document diagnostics=%#v", changed.Diagnostics)
	}
}
