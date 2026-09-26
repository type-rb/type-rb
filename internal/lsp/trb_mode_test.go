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
	edited := "def greet(name: String): String\n\treturn \"Hi, \" + name\nend\n"
	invalid := "def greet(name: String): String\n\treturn 1\nend\n"
	uri := uriFromPath(filename)
	change := func(version int, text string) message {
		return message{JSONRPC: "2.0", Method: "textDocument/didChange", Params: rawParams(t, didChangeParams{
			TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: version}, ContentChanges: []contentChange{{Text: text}},
		})}
	}
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "typerb", Version: 1, Text: valid}})},
		change(2, edited),
		change(3, invalid),
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
	var invalidReported bool
	for _, frame := range decodeFrames(t, output.Bytes()) {
		if string(frame["method"]) != `"textDocument/publishDiagnostics"` {
			continue
		}
		var published publishDiagnosticsParams
		decodeParamsFrame(t, frame, &published)
		if published.Version == nil {
			t.Fatalf("diagnostics without a document version: %#v", published)
		}
		if *published.Version < 3 && len(published.Diagnostics) != 0 {
			t.Fatalf("valid trb document version %d diagnostics=%#v", *published.Version, published.Diagnostics)
		}
		if *published.Version == 3 {
			invalidReported = len(published.Diagnostics) > 0 && published.Diagnostics[0].Code == "TRB3000"
		}
	}
	if !invalidReported {
		t.Fatalf("invalid trb document was not diagnosed: %s", output.String())
	}
}

func TestServerOffersNoRunCodeLensForTRBMode(t *testing.T) {
	filename := cleanPath("main.trb")
	source := "def main()\n\treturn\nend\n"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/codeLens", Params: rawParams(t, documentParams{TextDocument: textDocumentIdentifier{URI: uri}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "trb", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "trb"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	for _, frame := range decodeFrames(t, output.Bytes()) {
		if string(frame["id"]) != "2" {
			continue
		}
		var lenses []codeLens
		decodeResult(t, frame, &lenses)
		if len(lenses) != 0 {
			t.Fatalf("mode trb offered execution code lenses: %#v", lenses)
		}
		return
	}
	t.Fatalf("no code lens response: %s", output.String())
}
