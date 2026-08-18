package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
)

func BenchmarkServerCompletionWithLargeProjectIndex(b *testing.B) {
	const exportCount = 512
	mainPath := cleanPath("app/main.trb")
	mainUnit := compiler.SourceUnit{
		Filename: mainPath, ModulePath: "app/main", Package: "main",
		Source: []byte("def use()\n\treturn\nend\n"),
	}
	units := make([]compiler.SourceUnit, 0, exportCount+1)
	units = append(units, mainUnit)
	for index := 0; index < exportCount; index++ {
		name := fmt.Sprintf("Export%03d", index)
		units = append(units, compiler.SourceUnit{
			Filename:   cleanPath(fmt.Sprintf("models/export_%03d.trb", index)),
			ModulePath: fmt.Sprintf("models/export_%03d", index),
			Package:    "main",
			Source:     []byte(fmt.Sprintf("record %s\n\tvalue: Integer\nend\n", name)),
		})
	}
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(nil), Output: io.Discard,
		Units: units, CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp-benchmark"},
	})
	if err := server.publish(); err != nil {
		b.Fatal(err)
	}
	completionSource := []byte("def use()\n\tExport511\n\treturn\nend\n")
	mainUnit.Source = completionSource
	server.documents[mainPath] = document{unit: mainUnit, source: completionSource, version: 2}
	params, err := json.Marshal(documentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uriFromPath(mainPath)},
		Position:     position{Line: 1, Character: len("Export511")},
	})
	if err != nil {
		b.Fatal(err)
	}
	request := message{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "textDocument/completion",
		Params: params,
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.ReportMetric(exportCount, "exports")
	for b.Loop() {
		if err := server.completion(request); err != nil {
			b.Fatal(err)
		}
	}
}
