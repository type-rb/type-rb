package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/compilerservice"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/languageservice"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/token"
)

func TestServerPublishesDiagnosticsAndServesCompletionAndFormatting(t *testing.T) {
	filename := cleanPath("main.trb")
	valid := "def greet(name: String): String\n\treturn \"Hello, \" + name\nend\n"
	invalid := "def greet(name: String): String\n  return \"Hello, \" + name\nend\n\nmissing()\ngre"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "typerb", Version: 1, Text: valid}})},
		message{JSONRPC: "2.0", Method: "textDocument/didChange", Params: rawParams(t, didChangeParams{
			TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: 2}, ContentChanges: []contentChange{{Text: invalid}},
		})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/completion", Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: position{Line: 5, Character: 3}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "textDocument/formatting", Params: rawParams(t, formattingParams{TextDocument: textDocumentIdentifier{URI: uri}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Version: "test", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Package: "main", Source: []byte(valid)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 6 {
		t.Fatalf("response count=%d, want 6: %s", len(frames), output.String())
	}

	var initialized initializeResult
	decodeResult(t, frames[0], &initialized)
	if initialized.ServerInfo.Name != "TypeRB" || initialized.Capabilities.TextDocumentSync != (textDocumentSyncOptions{OpenClose: true, Change: textDocumentSyncIncremental, Save: true}) || !initialized.Capabilities.CodeActionProvider || !initialized.Capabilities.FoldingRangeProvider || !initialized.Capabilities.DocumentHighlightProvider || !initialized.Capabilities.SelectionRangeProvider || initialized.Capabilities.CodeLensProvider == nil || initialized.Capabilities.CodeLensProvider.ResolveProvider {
		t.Fatalf("unexpected initialize result: %#v", initialized)
	}
	var opened publishDiagnosticsParams
	decodeParamsFrame(t, frames[1], &opened)
	if opened.Version == nil || *opened.Version != 1 || len(opened.Diagnostics) != 0 {
		t.Fatalf("valid document diagnostics=%#v", opened.Diagnostics)
	}
	var changed publishDiagnosticsParams
	decodeParamsFrame(t, frames[2], &changed)
	if changed.Version == nil || *changed.Version != 2 || len(changed.Diagnostics) == 0 || changed.Diagnostics[0].Code != "TRB3000" {
		t.Fatalf("invalid document diagnostics=%#v", changed.Diagnostics)
	}

	var completions completionList
	decodeResult(t, frames[3], &completions)
	if completions.IsIncomplete || !containsCompletion(completions.Items, "greet") {
		t.Fatalf("completion response does not contain a complete greet result: %#v", completions)
	}
	var edits []textEdit
	decodeResult(t, frames[4], &edits)
	if len(edits) != 1 || !strings.Contains(edits[0].NewText, "\treturn") {
		t.Fatalf("unexpected formatting edits: %#v", edits)
	}
	if string(frames[5]["result"]) != "null" {
		t.Fatalf("shutdown result=%s, want null", frames[5]["result"])
	}
}

func TestServerFormattingCanonicalizesOnlyEquivalentIndexImports(t *testing.T) {
	root := t.TempDir()
	entryPath := cleanPath(filepath.Join(root, "main.trb"))
	entrySource := "import { DataTable } from shared / ui / DataTable / index\nimport { IndexedUser } from models / user / index\n"
	var output bytes.Buffer
	server := New(Options{
		Mode: "typescript", Input: bytes.NewReader(nil), Output: &output,
		Units: []compiler.SourceUnit{
			{Filename: entryPath, ModulePath: "main", Source: []byte(entrySource)},
			{Filename: cleanPath(filepath.Join(root, "shared", "ui", "DataTable", "index.trb")), ModulePath: "shared/ui/DataTable/index"},
			{Filename: cleanPath(filepath.Join(root, "models", "user.trb")), ModulePath: "models/user"},
			{Filename: cleanPath(filepath.Join(root, "models", "user", "index.trb")), ModulePath: "models/user/index"},
		},
		CompilerOptions: compiler.Options{Mode: "typescript"},
	})
	if err := server.format(message{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "textDocument/formatting",
		Params: rawParams(t, formattingParams{TextDocument: textDocumentIdentifier{URI: uriFromPath(entryPath)}}),
	}); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 1 {
		t.Fatalf("response count=%d, want 1: %s", len(frames), output.String())
	}
	var edits []textEdit
	decodeResult(t, frames[0], &edits)
	want := "import { DataTable } from shared/ui/DataTable\nimport { IndexedUser } from models/user/index\n"
	if len(edits) != 1 || edits[0].NewText != want {
		t.Fatalf("formatting edits=%#v, want %q", edits, want)
	}
}

func TestServerServesCompletionAndFormattingWhileDiagnosticsAreRunning(t *testing.T) {
	filename := cleanPath("main.trb")
	valid := "record Message\n\ttext: String\nend\n\ndef render(message: Message)\n\tputs(message.text)\n\treturn\nend\n"
	changed := "record Message\n\ttext: String\nend\n\ndef render(message: Message)\n  puts(message.te)\n\treturn\nend\n"
	uri := uriFromPath(filename)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(nil), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Package: "main", Source: []byte(valid)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.open(textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: valid}); err != nil {
		t.Fatal(err)
	}
	enableTestBackgroundDiagnostics(server)
	started := make(chan struct{})
	release := make(chan struct{})
	originalAnalyze := server.diagnostics.analyze
	server.diagnostics.analyze = func() (compilerservice.Snapshot, bool) {
		close(started)
		<-release
		return originalAnalyze()
	}
	server.diagnostics.Start()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
		server.diagnostics.Stop(diagnosticShutdownGrace)
	}()

	output.Reset()
	if err := server.change(didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []contentChange{{Text: changed}},
	}); err != nil {
		t.Fatal(err)
	}
	waitForTestSignal(t, started, "diagnostics analysis to start")
	if server.documents[filename].version != 2 {
		t.Fatalf("document version=%d, want 2", server.documents[filename].version)
	}
	cursor := positionAt([]byte(changed), strings.Index(changed, "message.te")+len("message.te"))
	if err := server.completion(message{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "textDocument/completion",
		Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: cursor}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.format(message{
		JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/formatting",
		Params: rawParams(t, formattingParams{TextDocument: textDocumentIdentifier{URI: uri}}),
	}); err != nil {
		t.Fatal(err)
	}

	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 2 {
		t.Fatalf("response count=%d, want 2: %s", len(frames), output.String())
	}
	var completions completionList
	decodeResult(t, frames[0], &completions)
	if !completions.IsIncomplete || !containsCompletion(completions.Items, "text") {
		t.Fatalf("completion response does not contain text: %#v", completions)
	}
	var edits []textEdit
	decodeResult(t, frames[1], &edits)
	if len(edits) != 1 || !strings.Contains(edits[0].NewText, "\tputs(message.te)") {
		t.Fatalf("unexpected formatting edits: %#v", edits)
	}
}

func TestServerServesLocalCompletionAndFormattingDuringInitialAnalysis(t *testing.T) {
	filename := cleanPath("main.trb")
	source := "def greet(name: String): String\n  return name\nend\n\ngre"
	uri := uriFromPath(filename)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(nil), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	enableTestBackgroundDiagnostics(server)
	started := make(chan struct{})
	release := make(chan struct{})
	originalAnalyze := server.diagnostics.analyze
	server.diagnostics.analyze = func() (compilerservice.Snapshot, bool) {
		close(started)
		<-release
		return originalAnalyze()
	}
	if err := server.open(textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}); err != nil {
		t.Fatal(err)
	}
	server.diagnostics.Start()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
		server.diagnostics.Stop(diagnosticShutdownGrace)
	}()
	waitForTestSignal(t, started, "initial diagnostics analysis to start")
	if server.currentSnapshot().Version != 0 {
		t.Fatal("initial analysis unexpectedly completed before the language requests")
	}
	if err := server.completion(message{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "textDocument/completion",
		Params: rawParams(t, documentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			Position:     positionAt([]byte(source), len(source)),
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.format(message{
		JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/formatting",
		Params: rawParams(t, formattingParams{TextDocument: textDocumentIdentifier{URI: uri}}),
	}); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 2 {
		t.Fatalf("response count=%d, want 2: %s", len(frames), output.String())
	}
	var completions completionList
	decodeResult(t, frames[0], &completions)
	if !completions.IsIncomplete || !containsCompletion(completions.Items, "greet") {
		t.Fatalf("cold local completion response does not contain greet: %#v", completions)
	}
	var edits []textEdit
	decodeResult(t, frames[1], &edits)
	if len(edits) != 1 || !strings.Contains(edits[0].NewText, "\treturn name") {
		t.Fatalf("cold formatting edits=%#v", edits)
	}
}

func TestServerCoalescesBackgroundDiagnostics(t *testing.T) {
	filename := cleanPath("main.trb")
	valid := "def run()\n\treturn\nend\n"
	uri := uriFromPath(filename)
	output := &synchronizedBuffer{}
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(nil), Output: output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Package: "main", Source: []byte(valid)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.open(textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: valid}); err != nil {
		t.Fatal(err)
	}
	enableTestBackgroundDiagnostics(server)
	output.Reset()
	sources := []string{
		"def run()\n\tmissing_old()\n\treturn\nend\n",
		valid,
		"def run()\n\tmissing_final()\n\treturn\nend\n",
	}
	for index, source := range sources {
		if err := server.change(didChangeParams{
			TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: index + 2},
			ContentChanges: []contentChange{{Text: source}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if queued := len(server.diagnostics.requests); queued != 1 {
		t.Fatalf("queued analyses=%d, want one coalesced request", queued)
	}
	calls := 0
	originalAnalyze := server.diagnostics.analyze
	server.diagnostics.analyze = func() (compilerservice.Snapshot, bool) {
		calls++
		return originalAnalyze()
	}
	server.diagnostics.Start()
	t.Cleanup(func() { server.diagnostics.Stop(diagnosticShutdownGrace) })
	result := waitForDiagnosticResult(t, server.diagnostics.Results(), "coalesced diagnostics analysis")
	if version := server.currentSnapshot().Version; version != 1 {
		t.Fatalf("diagnostics worker applied snapshot version %d, want main-loop snapshot version 1", version)
	}
	if len(output.Bytes()) != 0 {
		t.Fatalf("diagnostics worker wrote protocol output before the result was applied: %s", output.String())
	}
	if err := server.applyDiagnosticResult(result); err != nil {
		t.Fatal(err)
	}
	server.diagnostics.Stop(diagnosticShutdownGrace)
	if calls != 1 {
		t.Fatalf("analysis calls=%d, want 1", calls)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 1 {
		t.Fatalf("published frames=%d, want 1: %s", len(frames), output.String())
	}
	var published publishDiagnosticsParams
	decodeParamsFrame(t, frames[0], &published)
	if published.Version == nil || *published.Version != 4 || len(published.Diagnostics) != 1 || !strings.Contains(published.Diagnostics[0].Message, "missing_final") || strings.Contains(published.Diagnostics[0].Message, "missing_old") {
		t.Fatalf("coalesced diagnostics=%#v", published.Diagnostics)
	}
}

func TestServerClearsStaleDiagnosticsBeforeDeferredAnalysis(t *testing.T) {
	filename := cleanPath("main.trb")
	invalid := "def run()\n\tmissing()\n\treturn\nend\n"
	valid := "def run()\n\treturn\nend\n"
	uri := uriFromPath(filename)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(nil), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Package: "main", Source: []byte(invalid)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.open(textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: invalid}); err != nil {
		t.Fatal(err)
	}
	if !server.published[filename] {
		t.Fatal("initial error diagnostics were not published")
	}

	enableTestBackgroundDiagnostics(server)
	output.Reset()
	if err := server.change(didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []contentChange{{Text: invalid}},
	}); err != nil {
		t.Fatal(err)
	}
	if len(output.Bytes()) != 0 || !server.published[filename] {
		t.Fatalf("an unchanged source invalidated diagnostics: %s", output.String())
	}

	started := make(chan struct{})
	release := make(chan struct{})
	originalAnalyze := server.diagnostics.analyze
	server.diagnostics.analyze = func() (compilerservice.Snapshot, bool) {
		close(started)
		<-release
		return originalAnalyze()
	}
	if err := server.change(didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: 3},
		ContentChanges: []contentChange{{Text: valid}},
	}); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 1 {
		t.Fatalf("immediate diagnostic frames=%d, want one clear: %s", len(frames), output.String())
	}
	var cleared publishDiagnosticsParams
	decodeParamsFrame(t, frames[0], &cleared)
	if cleared.Version == nil || *cleared.Version != 3 || len(cleared.Diagnostics) != 0 || server.published[filename] {
		t.Fatalf("stale diagnostics were not cleared for version 3: %#v", cleared)
	}

	server.diagnostics.Start()
	defer server.diagnostics.Stop(diagnosticShutdownGrace)
	waitForTestSignal(t, started, "deferred diagnostics analysis to start")
	close(release)
	result := waitForDiagnosticResult(t, server.diagnostics.Results(), "valid diagnostics analysis")
	if err := server.applyDiagnosticResult(result); err != nil {
		t.Fatal(err)
	}
	frames = decodeFrames(t, output.Bytes())
	if len(frames) != 2 {
		t.Fatalf("diagnostic frames=%d, want immediate and analyzed clears: %s", len(frames), output.String())
	}
	var analyzed publishDiagnosticsParams
	decodeParamsFrame(t, frames[1], &analyzed)
	if analyzed.Version == nil || *analyzed.Version != 3 || len(analyzed.Diagnostics) != 0 {
		t.Fatalf("analyzed diagnostics=%#v", analyzed)
	}
}

func TestServerDoesNotPublishObsoleteBackgroundDiagnostics(t *testing.T) {
	filename := cleanPath("main.trb")
	valid := "def run()\n\treturn\nend\n"
	oldInvalid := "def run()\n\tmissing_old()\n\treturn\nend\n"
	finalInvalid := "def run()\n\tmissing_final()\n\treturn\nend\n"
	uri := uriFromPath(filename)
	output := &synchronizedBuffer{}
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(nil), Output: output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Package: "main", Source: []byte(valid)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.open(textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: valid}); err != nil {
		t.Fatal(err)
	}
	enableTestBackgroundDiagnostics(server)
	output.Reset()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	calls := 0
	originalAnalyze := server.diagnostics.analyze
	server.diagnostics.analyze = func() (compilerservice.Snapshot, bool) {
		calls++
		if calls == 1 {
			generation := server.compiler.Generation()
			close(started)
			<-release
			return compilerservice.Snapshot{Version: generation}, true
		}
		return originalAnalyze()
	}
	server.diagnostics.Start()
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		server.diagnostics.Stop(diagnosticShutdownGrace)
	})
	if err := server.change(didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []contentChange{{Text: oldInvalid}},
	}); err != nil {
		t.Fatal(err)
	}
	waitForTestSignal(t, started, "obsolete diagnostics analysis to start")
	for index, source := range []string{valid, finalInvalid} {
		if err := server.change(didChangeParams{
			TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: index + 3},
			ContentChanges: []contentChange{{Text: source}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	releaseOnce.Do(func() { close(release) })
	result := waitForDiagnosticResult(t, server.diagnostics.Results(), "current diagnostics analysis")
	if err := server.applyDiagnosticResult(result); err != nil {
		t.Fatal(err)
	}
	server.diagnostics.Stop(diagnosticShutdownGrace)
	if calls != 2 {
		t.Fatalf("analysis calls=%d, want obsolete and current analyses", calls)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 1 {
		t.Fatalf("published frames=%d, want only the current result: %s", len(frames), output.String())
	}
	var published publishDiagnosticsParams
	decodeParamsFrame(t, frames[0], &published)
	if published.Version == nil || *published.Version != 4 || len(published.Diagnostics) != 1 || !strings.Contains(published.Diagnostics[0].Message, "missing_final") || strings.Contains(published.Diagnostics[0].Message, "missing_old") {
		t.Fatalf("published obsolete diagnostics: %#v", published.Diagnostics)
	}
}

func TestServerSuppressesEditsFromAStaleSnapshot(t *testing.T) {
	filename := cleanPath("user.trb")
	original := "record User\n\tid: Int\nend\n"
	changed := "\n" + original
	uri := uriFromPath(filename)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(nil), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "user", Package: "main", Source: []byte(original)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.open(textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: original}); err != nil {
		t.Fatal(err)
	}
	enableTestBackgroundDiagnostics(server)
	output.Reset()
	if err := server.change(didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []contentChange{{Text: changed}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, current := server.currentAnalyzedSnapshot(); current {
		t.Fatal("snapshot unexpectedly remained current after the edit")
	}
	if err := server.codeActions(message{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "textDocument/codeAction",
		Params: rawParams(t, codeActionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			Range:        rangeValue{Start: position{Line: 2, Character: 5}, End: position{Line: 2, Character: 8}},
		}),
	}); err != nil {
		t.Fatal(err)
	}
	userPosition := position{Line: 1, Character: 8}
	if err := server.prepareRename(message{
		JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/prepareRename",
		Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: userPosition}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.rename(message{
		JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "textDocument/rename",
		Params: rawParams(t, renameParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: userPosition, NewName: "Account"}),
	}); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 4 {
		t.Fatalf("response count=%d, want one diagnostic clear and three responses: %s", len(frames), output.String())
	}
	var cleared publishDiagnosticsParams
	decodeParamsFrame(t, frames[0], &cleared)
	if cleared.Version == nil || *cleared.Version != 2 || len(cleared.Diagnostics) != 0 {
		t.Fatalf("stale diagnostic clear=%#v", cleared)
	}
	var actions []codeAction
	decodeResult(t, frames[1], &actions)
	if len(actions) != 0 {
		t.Fatalf("stale quick fixes=%#v, want none", actions)
	}
	if string(frames[2]["result"]) != "null" || string(frames[3]["result"]) != "null" {
		t.Fatalf("stale rename results=%s / %s, want null", frames[2]["result"], frames[3]["result"])
	}
}

func TestServerRepublishesDiagnosticsForAnAcceptedVersionOnlyChange(t *testing.T) {
	filename := cleanPath("main.trb")
	source := "missing()\n"
	uri := uriFromPath(filename)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(nil), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.open(textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := server.change(didChangeParams{TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := server.change(didChangeParams{TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: 1}}); err != nil {
		t.Fatal(err)
	}
	if version := server.documents[filename].version; version != 2 {
		t.Fatalf("document version=%d, want 2 after ignoring an out-of-order empty change", version)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 1 {
		t.Fatalf("published frames=%d, want one accepted version-only update: %s", len(frames), output.String())
	}
	var published publishDiagnosticsParams
	decodeParamsFrame(t, frames[0], &published)
	if published.Version == nil || *published.Version != 2 || len(published.Diagnostics) != 1 {
		t.Fatalf("version-only diagnostics=%#v", published)
	}
}

func TestServerStopsWithoutWaitingForBlockedDiagnostics(t *testing.T) {
	filename := cleanPath("main.trb")
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(nil), Output: io.Discard,
		Units:                 []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Package: "main", Source: []byte("puts(\"ok\")\n")}},
		CompilerOptions:       compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
		BackgroundDiagnostics: true,
	})
	server.diagnostics.delay = 0
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	server.diagnostics.analyze = func() (compilerservice.Snapshot, bool) {
		close(started)
		<-release
		return compilerservice.Snapshot{}, false
	}
	server.diagnostics.Start()
	if err := server.requestDiagnostics(); err != nil {
		t.Fatal(err)
	}
	waitForTestSignal(t, started, "blocked diagnostics analysis to start")

	stopped := make(chan struct{}, 1)
	go func() {
		server.diagnostics.Stop(diagnosticShutdownGrace)
		stopped <- struct{}{}
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stopping the server waited for blocked diagnostics")
	}

	releaseOnce.Do(func() { close(release) })
	waitForTestSignal(t, server.diagnostics.done, "diagnostics worker to exit after release")
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
}

func TestServerRunReturnsBackgroundOutputErrorWithoutMoreInput(t *testing.T) {
	filename := cleanPath("main.trb")
	source := "puts(\"ok\")\n"
	uri := uriFromPath(filename)
	inputReader, inputWriter := io.Pipe()
	t.Cleanup(func() {
		_ = inputWriter.Close()
		_ = inputReader.Close()
	})
	outputError := errors.New("background output failed")
	output := &failAfterWriter{successfulWrites: 2, err: outputError}
	server := New(Options{
		Mode: "go", Input: inputReader, Output: output,
		Units:                 []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Package: "main", Source: []byte(source)}},
		CompilerOptions:       compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
		BackgroundDiagnostics: true,
	})
	server.diagnostics.delay = 0

	runResult := make(chan error, 1)
	go func() { runResult <- server.Run() }()
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
	)
	writeResult := make(chan error, 1)
	go func() {
		_, err := inputWriter.Write(input)
		writeResult <- err
	}()

	select {
	case err := <-runResult:
		if !errors.Is(err, outputError) {
			t.Fatalf("Run error=%v, want %v", err, outputError)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not observe the background output failure")
	}
	if err := <-writeResult; err != nil {
		t.Fatal(err)
	}
}

func TestServerReturnsRunCodeLensForTopLevelMain(t *testing.T) {
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
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var initialized initializeResult
	decodeResult(t, frames[0], &initialized)
	if initialized.Capabilities.CodeLensProvider == nil || initialized.Capabilities.CodeLensProvider.ResolveProvider {
		t.Fatalf("code lens capability=%#v", initialized.Capabilities.CodeLensProvider)
	}
	var lenses []codeLens
	decodeResult(t, frames[2], &lenses)
	if len(lenses) != 1 || lenses[0].Range.Start != (position{Line: 0, Character: 4}) || lenses[0].Range.End != (position{Line: 0, Character: 8}) {
		t.Fatalf("code lenses=%#v", lenses)
	}
	if lenses[0].Command.Title != "▶ Run" || lenses[0].Command.Command != "typerb.runProject" || !reflect.DeepEqual(lenses[0].Command.Arguments, []interface{}{uri}) {
		t.Fatalf("run command=%#v", lenses[0].Command)
	}
}

func TestServerDiscoversTestsAndReturnsTestCodeLens(t *testing.T) {
	filename := cleanPath("calculator_test.trb")
	source := `import { describe, expect, test } from trb/std/test

describe("Calculator") do
	test("adds numbers") do
		expect(1 + 2).to_equal(3)
	end
end
`
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "typerb/discoverTests", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "textDocument/codeLens", Params: rawParams(t, documentParams{TextDocument: textDocumentIdentifier{URI: uri}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "calculator_test", Package: "main", Source: []byte(source), TestRegistration: "trb_test_register_sample"}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var items []testItem
	decodeResult(t, frames[1], &items)
	if len(items) != 2 || items[0].Kind != "suite" || items[0].FullName != "Calculator" || items[1].Kind != "test" || items[1].FullName != "Calculator / adds numbers" || items[1].ParentID != items[0].ID {
		t.Fatalf("test discovery=%#v", items)
	}
	var lenses []codeLens
	decodeResult(t, frames[2], &lenses)
	if len(lenses) != 2 || lenses[0].Command.Command != "typerb.runTest" || lenses[1].Command.Command != "typerb.debugTest" ||
		!reflect.DeepEqual(lenses[0].Command.Arguments, []interface{}{uri, "Calculator / adds numbers"}) ||
		!reflect.DeepEqual(lenses[1].Command.Arguments, []interface{}{uri, "Calculator / adds numbers"}) {
		t.Fatalf("test code lenses=%#v", lenses)
	}
}

func TestServerOmitsRunCodeLensForBrowserProjects(t *testing.T) {
	filename := cleanPath("main.trb")
	source := "def main()\n\treturn\nend\n"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/codeLens", Params: rawParams(t, documentParams{TextDocument: textDocumentIdentifier{URI: uri}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "typescript", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "typescript", TypeScriptRuntime: "browser"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var lenses []codeLens
	decodeResult(t, frames[1], &lenses)
	if len(lenses) != 0 {
		t.Fatalf("browser code lenses=%#v", lenses)
	}
}

func TestServerDiagnosesUnknownTypesAndCompletesCanonicalNames(t *testing.T) {
	filename := cleanPath("user.trb")
	source := "record User\n\tid: Int\nend\n"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/completion", Params: rawParams(t, documentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri}, Position: position{Line: 1, Character: 8},
		})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "textDocument/codeAction", Params: rawParams(t, codeActionParams{
			TextDocument: textDocumentIdentifier{URI: uri}, Range: rangeValue{Start: position{Line: 1, Character: 5}, End: position{Line: 1, Character: 8}},
		})},
		message{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "user", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var published publishDiagnosticsParams
	decodeParamsFrame(t, frames[1], &published)
	if len(published.Diagnostics) != 1 || !strings.Contains(published.Diagnostics[0].Message, "use Integer") {
		t.Fatalf("type diagnostics=%#v", published.Diagnostics)
	}
	var completions completionList
	decodeResult(t, frames[2], &completions)
	if !containsCompletion(completions.Items, "Integer") {
		t.Fatalf("type completion response=%#v", completions)
	}
	var actions []codeAction
	decodeResult(t, frames[3], &actions)
	if len(actions) != 1 || actions[0].Kind != "quickfix" {
		t.Fatalf("canonical type actions=%#v", actions)
	}
	edits := actions[0].Edit.Changes[uri]
	if len(edits) != 1 || edits[0].NewText != "Integer" || edits[0].Range != (rangeValue{Start: position{Line: 1, Character: 5}, End: position{Line: 1, Character: 8}}) {
		t.Fatalf("canonical type edits=%#v", edits)
	}
}

func TestServerDiagnosesAndAutoImportsStandardTypes(t *testing.T) {
	filename := cleanPath("result.trb")
	source := "def checked(value: Integer): Result<Integer, String>\n\treturn Result<Integer, String>::Ok(value)\nend\n"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/completion", Params: rawParams(t, documentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri}, Position: position{Line: 0, Character: 35},
		})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "result", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var published publishDiagnosticsParams
	decodeParamsFrame(t, frames[1], &published)
	if len(published.Diagnostics) == 0 || !strings.Contains(published.Diagnostics[0].Message, "Result is not declared or imported") {
		t.Fatalf("missing import diagnostics=%#v", published.Diagnostics)
	}
	var completions completionList
	decodeResult(t, frames[2], &completions)
	for _, item := range completions.Items {
		if item.Label != "Result" {
			continue
		}
		if len(item.AdditionalTextEdits) != 1 || item.AdditionalTextEdits[0].NewText != "import { Result } from trb/std/result\n" || item.AdditionalTextEdits[0].Range != (rangeValue{}) {
			t.Fatalf("Result completion=%#v", item)
		}
		return
	}
	t.Fatalf("Result completion is missing: %#v", completions)
}

func TestServerAutoImportsAnUnambiguousProjectType(t *testing.T) {
	userFilename := cleanPath("models/user.trb")
	mainFilename := cleanPath("main.trb")
	userSource := "record User\n\tname: String\nend\n"
	valid := "import { User } from models/user\n\ndef inspect(user: User)\n\tputs(user.name)\n\treturn\nend\n"
	invalid := "def inspect(user: User)\n\tputs(user.name)\n\treturn\nend\n"
	uri := uriFromPath(mainFilename)
	userOffset := strings.Index(invalid, "User")
	cursor := positionAt([]byte(invalid), userOffset+len("User"))
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: valid}})},
		message{JSONRPC: "2.0", Method: "textDocument/didChange", Params: rawParams(t, didChangeParams{
			TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: 2}, ContentChanges: []contentChange{{Text: invalid}},
		})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/completion", Params: rawParams(t, documentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri}, Position: cursor,
		})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "textDocument/codeAction", Params: rawParams(t, codeActionParams{
			TextDocument: textDocumentIdentifier{URI: uri}, Range: rangeValue{Start: cursor, End: cursor},
		})},
		message{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units: []compiler.SourceUnit{
			{Filename: userFilename, ModulePath: "models/user", Package: "models", Source: []byte(userSource)},
			{Filename: mainFilename, ModulePath: "main", Package: "main", Source: []byte(valid)},
		},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var completions completionList
	decodeResult(t, frames[3], &completions)
	for _, item := range completions.Items {
		if item.Label == "User" && len(item.AdditionalTextEdits) == 1 && item.AdditionalTextEdits[0].NewText == "import { User } from models/user\n" {
			var actions []codeAction
			decodeResult(t, frames[4], &actions)
			if len(actions) != 1 || actions[0].Title != "Add import for User" || actions[0].Kind != "quickfix" {
				t.Fatalf("project auto-import actions=%#v", actions)
			}
			edits := actions[0].Edit.Changes[uri]
			if len(edits) != 1 || edits[0].NewText != "import { User } from models/user\n" || edits[0].Range != (rangeValue{}) {
				t.Fatalf("project auto-import action edits=%#v", edits)
			}
			return
		}
	}
	t.Fatalf("project auto-import completion is missing: %#v", completions)
}

func TestServerOnlyOffersAutoImportForUndeclaredNameDiagnostics(t *testing.T) {
	userFilename := cleanPath("models/user.trb")
	mainFilename := cleanPath("main.trb")
	valid := "import { User } from models/user\n\ndef inspect(user: User)\n\treturn\nend\n"
	invalid := "def inspect(user: User)\n\treturn\nend\n"
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(nil), Output: io.Discard,
		Units: []compiler.SourceUnit{
			{Filename: userFilename, ModulePath: "models/user", Package: "models", Source: []byte("record User\n\tname: String\nend\n")},
			{Filename: mainFilename, ModulePath: "main", Package: "main", Source: []byte(valid)},
		},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	snapshot := server.compiler.Analyze()
	if snapshot.HasErrors() {
		t.Fatalf("valid import snapshot has diagnostics: %#v", snapshot.Diagnostics)
	}
	offset := strings.Index(invalid, "User")
	span := token.Span{Start: token.Position{Offset: offset}, End: token.Position{Offset: offset + len("User")}}
	document := document{
		unit:   compiler.SourceUnit{Filename: mainFilename, ModulePath: "main", Package: "main", Source: []byte(invalid)},
		source: []byte(invalid), version: 2,
	}

	tests := []struct {
		name       string
		code       diagnostic.Code
		message    string
		wantAction bool
	}{
		{name: "undeclared name", code: diagnostic.TypeError, message: "type User is not declared or imported", wantAction: true},
		{name: "unrelated type error", code: diagnostic.TypeError, message: "expected String but got User"},
		{name: "different undeclared name", code: diagnostic.TypeError, message: "type AdminUser is not declared or imported"},
		{name: "syntax error", code: diagnostic.SyntaxError, message: "type User is not declared or imported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot.Diagnostics = []diagnostic.Diagnostic{{
				Code: test.code, Severity: diagnostic.Error, Message: test.message, Path: mainFilename, Span: span,
			}}
			actions := server.autoImportCodeActions(mainFilename, document, snapshot, offset+len("User"), offset+len("User"))
			if test.wantAction {
				if len(actions) != 1 || actions[0].Title != "Add import for User" {
					t.Fatalf("auto-import actions=%#v, want User quick fix", actions)
				}
				return
			}
			if len(actions) != 0 {
				t.Fatalf("auto-import actions=%#v, want none", actions)
			}
		})
	}
}

func TestServerBoundsCompletionCandidatesToTheMostRecentModule(t *testing.T) {
	server := New(Options{
		Mode: "typescript", Input: bytes.NewReader(nil), Output: io.Discard,
		Units: []compiler.SourceUnit{
			{Filename: cleanPath("first.trb"), ModulePath: "first", Source: []byte("record First\nend\n")},
			{Filename: cleanPath("second.trb"), ModulePath: "second", Source: []byte("record Second\nend\n")},
		},
		CompilerOptions: compiler.Options{Mode: "typescript"},
	})
	if err := server.publish(); err != nil {
		t.Fatal(err)
	}
	snapshot := server.currentSnapshot()
	server.candidatesForCompletion(snapshot, "first")
	if !server.completionCandidates.valid || server.completionCandidates.modulePath != "first" {
		t.Fatalf("first completion cache=%#v", server.completionCandidates)
	}
	server.candidatesForCompletion(snapshot, "second")
	if !server.completionCandidates.valid || server.completionCandidates.modulePath != "second" {
		t.Fatalf("second completion cache=%#v", server.completionCandidates)
	}

	server.setSnapshot(compilerservice.Snapshot{Version: snapshot.Version + 1})
	if server.completionCandidates.valid {
		t.Fatalf("completion cache survived a snapshot change: %#v", server.completionCandidates)
	}
}

func TestServerReturnsNestedDocumentSymbols(t *testing.T) {
	filename := cleanPath("outline.trb")
	source := "record User\n\tname: String\nend\n\ndef main()\n\tuser := User.new(name: \"Ada\")\n\tputs(user.name)\n\treturn\nend\n"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/documentSymbol", Params: rawParams(t, documentParams{TextDocument: textDocumentIdentifier{URI: uri}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "outline", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var initialized initializeResult
	decodeResult(t, frames[0], &initialized)
	if !initialized.Capabilities.DocumentSymbolProvider {
		t.Fatalf("document symbol capability=%#v", initialized.Capabilities)
	}
	var symbols []documentSymbol
	decodeResult(t, frames[2], &symbols)
	if len(symbols) != 2 || symbols[0].Name != "User" || symbols[0].Kind != 23 || len(symbols[0].Children) != 1 {
		t.Fatalf("document symbols=%#v", symbols)
	}
	if symbols[0].Children[0].Name != "name" || symbols[0].Children[0].Kind != 8 || symbols[1].Name != "main" || symbols[1].Kind != 12 {
		t.Fatalf("document symbols=%#v", symbols)
	}
	if symbols[0].SelectionRange.Start != (position{Line: 0, Character: 7}) || symbols[0].SelectionRange.End != (position{Line: 0, Character: 11}) {
		t.Fatalf("record selection=%#v", symbols[0].SelectionRange)
	}
}

func TestServerReturnsStructuralFoldingRanges(t *testing.T) {
	filename := cleanPath("folding.trb")
	source := "class User\n\tdef name(): String\n\t\treturn \"Ada\"\n\tend\nend\n"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/foldingRange", Params: rawParams(t, documentParams{TextDocument: textDocumentIdentifier{URI: uri}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "folding", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var ranges []foldingRange
	decodeResult(t, frames[2], &ranges)
	if len(ranges) != 2 || ranges[0].StartLine != 0 || ranges[0].EndLine != 4 || ranges[1].StartLine != 1 || ranges[1].EndLine != 3 {
		t.Fatalf("folding ranges=%#v", ranges)
	}
}

func TestServerHighlightsCheckedReferencesInTheCurrentDocument(t *testing.T) {
	filename := cleanPath("highlights.trb")
	source := "def main()\n\tuser := \"Ada\"\n\tputs(user)\nend\n\ndef label(user: String): String\n\treturn user\nend\n"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/documentHighlight", Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: position{Line: 2, Character: 7}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "highlights", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var highlights []documentHighlight
	decodeResult(t, frames[2], &highlights)
	if len(highlights) != 2 || highlights[0].Range.Start != (position{Line: 1, Character: 1}) || highlights[1].Range.Start != (position{Line: 2, Character: 6}) {
		t.Fatalf("document highlights=%#v", highlights)
	}
}

func TestServerReturnsNestedSelectionRanges(t *testing.T) {
	filename := cleanPath("selection.trb")
	source := "class User\n\tdef name(): String\n\t\tvalue := \"Ada\"\n\t\treturn value\n\tend\nend\n"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/selectionRange", Params: rawParams(t, selectionRangeParams{TextDocument: textDocumentIdentifier{URI: uri}, Positions: []position{{Line: 3, Character: 10}}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "selection", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var ranges []selectionRange
	decodeResult(t, frames[2], &ranges)
	if len(ranges) != 1 || ranges[0].Range.Start != (position{Line: 3, Character: 9}) || ranges[0].Parent == nil || ranges[0].Parent.Parent == nil {
		t.Fatalf("selection ranges=%#v", ranges)
	}
	if ranges[0].Parent.Range.Start != (position{Line: 3, Character: 0}) || ranges[0].Parent.Parent.Range.Start != (position{Line: 1, Character: 1}) {
		t.Fatalf("selection hierarchy=%#v", ranges[0])
	}
}

func TestServerAppliesOrderedIncrementalDocumentChanges(t *testing.T) {
	filename := cleanPath("incremental.trb")
	source := "record User\n\tname: String\nend\n"
	uri := uriFromPath(filename)
	userRange := rangeValue{Start: position{Line: 0, Character: 7}, End: position{Line: 0, Character: 11}}
	fieldRange := rangeValue{Start: position{Line: 1, Character: 1}, End: position{Line: 1, Character: 5}}
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", Method: "textDocument/didChange", Params: rawParams(t, didChangeParams{
			TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: 2},
			ContentChanges: []contentChange{
				{Range: &userRange, Text: "Account"},
				{Range: &fieldRange, Text: "display_name"},
			},
		})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/documentSymbol", Params: rawParams(t, documentParams{TextDocument: textDocumentIdentifier{URI: uri}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "incremental", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var symbols []documentSymbol
	decodeResult(t, frames[3], &symbols)
	if len(symbols) != 1 || symbols[0].Name != "Account" || len(symbols[0].Children) != 1 || symbols[0].Children[0].Name != "display_name" {
		t.Fatalf("incremental document symbols=%#v", symbols)
	}
}

func TestApplyContentChangesUsesUTF16Ranges(t *testing.T) {
	source := []byte("value := \"😀\"\nputs(value)\n")
	emoji := rangeValue{Start: position{Line: 0, Character: 10}, End: position{Line: 0, Character: 12}}
	call := rangeValue{Start: position{Line: 1, Character: 0}, End: position{Line: 1, Character: 4}}
	result, err := applyContentChanges(source, []contentChange{
		{Range: &emoji, Text: "Ada"},
		{Range: &call, Text: "print"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "value := \"Ada\"\nprint(value)\n" {
		t.Fatalf("result=%q", result)
	}

	insideSurrogate := rangeValue{Start: position{Line: 0, Character: 11}, End: position{Line: 0, Character: 12}}
	if _, err := applyContentChanges(source, []contentChange{{Range: &insideSurrogate, Text: "x"}}); err == nil {
		t.Fatal("expected a range inside a UTF-16 surrogate pair to fail")
	}
}

func TestServerTracksCreatedAndDeletedWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "src")
	filename := filepath.Join(sourceRoot, "models", "account.trb")
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	source := []byte("record Account\n\tname: String\nend\n")
	if err := os.WriteFile(filename, source, 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(nil), Output: &output,
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp", SourceRoot: sourceRoot},
		ResolveUnit: func(path string, contents []byte) (compiler.SourceUnit, error) {
			return compiler.SourceUnit{Filename: path, ModulePath: "models/account", Package: "models", Source: contents}, nil
		},
	})
	outside := filepath.Join(root, "other.trb")
	if err := os.WriteFile(outside, []byte("record Outside\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := server.changeWorkspaceFiles(didChangeWatchedFilesParams{Changes: []fileEvent{
		{URI: uriFromPath(outside), Type: fileChangeCreated},
		{URI: uriFromPath(filename), Type: fileChangeCreated},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, exists := server.base[cleanPath(outside)]; exists {
		t.Fatal("workspace watcher added a TypeRB file outside sourceRoot")
	}
	context, ok := server.snapshot.Context("models/account")
	if !ok || !containsLanguageCompletion(languageservice.Complete(languageservice.CompletionRequest{Source: "Acc", Cursor: 3, Mode: "go", Context: context}), "Account") {
		t.Fatalf("created workspace file context=%#v", context)
	}
	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	if err := server.changeWorkspaceFiles(didChangeWatchedFilesParams{Changes: []fileEvent{{URI: uriFromPath(filename), Type: fileChangeDeleted}}}); err != nil {
		t.Fatal(err)
	}
	if _, exists := server.base[cleanPath(filename)]; exists || len(server.snapshot.Artifacts) != 0 {
		t.Fatalf("deleted workspace file remains: base=%#v snapshot=%#v", server.base, server.snapshot)
	}
}

func TestServerIgnoresOpenDocumentsOutsideSourceRoot(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "src")
	outside := filepath.Join(root, "web", "todo.trb")
	generated := filepath.Join(sourceRoot, "build", "todo.trb")
	resolved := false
	server := New(Options{
		Mode: "go",
		CompilerOptions: compiler.Options{
			Mode: "go", GoModule: "example.com/lsp", SourceRoot: sourceRoot,
		},
		ExcludedRoots: []string{filepath.Join(sourceRoot, "build")},
		ResolveUnit: func(path string, contents []byte) (compiler.SourceUnit, error) {
			resolved = true
			return compiler.SourceUnit{Filename: path, ModulePath: "web/todo", Package: "web", Source: contents}, nil
		},
	})
	item := textDocumentItem{
		URI: uriFromPath(outside), LanguageID: "trb", Version: 1,
		Text: "record Todo\n\tid: Integer\nend\n",
	}
	if err := server.open(item); err != nil {
		t.Fatal(err)
	}
	item.URI = uriFromPath(generated)
	if err := server.open(item); err != nil {
		t.Fatal(err)
	}
	if resolved || len(server.documents) != 0 {
		t.Fatalf("outside document entered project: resolved=%v documents=%#v", resolved, server.documents)
	}
	if err := server.change(didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: item.URI, Version: 2},
		ContentChanges: []contentChange{{Text: item.Text}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.close(item.URI); err != nil {
		t.Fatal(err)
	}
}

func TestServerRestrictsStandaloneSessionToIncludedFile(t *testing.T) {
	root := t.TempDir()
	included := filepath.Join(root, "included.trb")
	sibling := filepath.Join(root, "sibling.trb")
	resolved := make([]string, 0, 1)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Output: &output,
		CompilerOptions: compiler.Options{
			Mode: "go", GoModule: "trb.local/standalone", SourceRoot: root,
		},
		IncludedFiles: []string{included},
		ResolveUnit: func(path string, contents []byte) (compiler.SourceUnit, error) {
			resolved = append(resolved, path)
			return compiler.SourceUnit{Filename: path, ModulePath: "included", Package: "main", Source: contents}, nil
		},
	})
	for _, filename := range []string{sibling, included} {
		if err := server.open(textDocumentItem{
			URI: uriFromPath(filename), LanguageID: "trb", Version: 1,
			Text: "def main()\n\treturn\nend\n",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if len(resolved) != 1 || cleanPath(resolved[0]) != cleanPath(included) {
		t.Fatalf("standalone resolver received files %v", resolved)
	}
	if len(server.documents) != 1 {
		t.Fatalf("standalone documents=%#v", server.documents)
	}
}

func TestServerReturnsFilteredWorkspaceSymbols(t *testing.T) {
	accountsFilename := cleanPath("accounts.trb")
	accountsSource := "module Accounts\n\tclass User\n\tend\nend\n"
	productsFilename := cleanPath("products.trb")
	productsSource := "record Product\n\tname: String\nend\n"
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "workspace/symbol", Params: rawParams(t, workspaceSymbolParams{Query: "us"})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units: []compiler.SourceUnit{
			{Filename: accountsFilename, ModulePath: "accounts", Package: "main", Source: []byte(accountsSource)},
			{Filename: productsFilename, ModulePath: "products", Package: "main", Source: []byte(productsSource)},
		},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var initialized initializeResult
	decodeResult(t, frames[0], &initialized)
	if !initialized.Capabilities.WorkspaceSymbolProvider {
		t.Fatalf("workspace symbol capability=%#v", initialized.Capabilities)
	}
	var symbols []symbolInformation
	decodeResult(t, frames[1], &symbols)
	if len(symbols) != 1 || symbols[0].Name != "User" || symbols[0].Kind != 5 || symbols[0].ContainerName != "Accounts" {
		t.Fatalf("workspace symbols=%#v", symbols)
	}
	if symbols[0].Location.URI != uriFromPath(accountsFilename) || symbols[0].Location.Range.Start != (position{Line: 1, Character: 7}) {
		t.Fatalf("workspace symbol location=%#v", symbols[0].Location)
	}
}

func TestServerReturnsSemanticTokensWithUTF16Positions(t *testing.T) {
	filename := cleanPath("semantic.trb")
	source := "puts(\"😀\")\nMAX := 1\nrecord User\n\tname: String\nend\n"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/semanticTokens/full", Params: rawParams(t, documentParams{TextDocument: textDocumentIdentifier{URI: uri}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "semantic", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var initialized initializeResult
	decodeResult(t, frames[0], &initialized)
	provider := initialized.Capabilities.SemanticTokensProvider
	if !provider.Full || len(provider.Legend.TokenTypes) != len(semanticTokenTypes) || len(provider.Legend.TokenModifiers) != 1 {
		t.Fatalf("semantic token capability=%#v", provider)
	}
	var result semanticTokens
	decodeResult(t, frames[2], &result)
	tokens := decodeSemanticTokenData(t, result.Data)
	if !containsSemanticToken(tokens, decodedSemanticToken{Line: 0, Start: 5, Length: 4, Type: 2}) {
		t.Fatalf("UTF-16 string token missing: %#v", tokens)
	}
	if !containsSemanticToken(tokens, decodedSemanticToken{Line: 1, Start: 0, Length: 3, Type: 1, Modifiers: 1}) {
		t.Fatalf("readonly constant token missing: %#v", tokens)
	}
	if !containsSemanticToken(tokens, decodedSemanticToken{Line: 2, Start: 7, Length: 4, Type: 0}) {
		t.Fatalf("type token missing: %#v", tokens)
	}
}

type decodedSemanticToken struct {
	Line      int
	Start     int
	Length    int
	Type      int
	Modifiers int
}

func decodeSemanticTokenData(t *testing.T, data []int) []decodedSemanticToken {
	t.Helper()
	if len(data)%5 != 0 {
		t.Fatalf("semantic token data has %d entries", len(data))
	}
	result := make([]decodedSemanticToken, 0, len(data)/5)
	line := 0
	start := 0
	for index := 0; index < len(data); index += 5 {
		line += data[index]
		if data[index] == 0 {
			start += data[index+1]
		} else {
			start = data[index+1]
		}
		result = append(result, decodedSemanticToken{
			Line: line, Start: start, Length: data[index+2], Type: data[index+3], Modifiers: data[index+4],
		})
	}
	return result
}

func containsSemanticToken(tokens []decodedSemanticToken, expected decodedSemanticToken) bool {
	for _, token := range tokens {
		if token == expected {
			return true
		}
	}
	return false
}

func TestSemanticTokenDataSplitsMultilineSpans(t *testing.T) {
	source := []byte("# one\n# 二")
	data := semanticTokenData(source, []languageservice.HighlightSpan{{
		Range: languageservice.OffsetRange{Start: 0, End: len(source)}, Kind: languageservice.HighlightComment,
	}})
	tokens := decodeSemanticTokenData(t, data)
	expected := []decodedSemanticToken{
		{Line: 0, Start: 0, Length: 5, Type: 4},
		{Line: 1, Start: 0, Length: 3, Type: 4},
	}
	if !reflect.DeepEqual(tokens, expected) {
		t.Fatalf("semantic tokens=%#v, want %#v", tokens, expected)
	}
}

func TestServerOffersDiagnosticFixesAsCodeActions(t *testing.T) {
	filename := cleanPath("unused.trb")
	source := "import trb/std/strings\n\ndef main()\n\treturn\nend\n"
	uri := uriFromPath(filename)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "typerb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/codeAction", Params: rawParams(t, codeActionParams{
			TextDocument: textDocumentIdentifier{URI: uri}, Range: rangeValue{Start: position{Line: 0}, End: position{Line: 0, Character: 22}},
		})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "unused", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var published publishDiagnosticsParams
	decodeParamsFrame(t, frames[1], &published)
	if len(published.Diagnostics) != 1 || published.Diagnostics[0].Code != "TRB3002" {
		t.Fatalf("unused import diagnostic=%#v", published.Diagnostics)
	}
	var actions []codeAction
	decodeResult(t, frames[2], &actions)
	if len(actions) != 1 || actions[0].Kind != "quickfix" {
		t.Fatalf("unexpected code actions: %#v", actions)
	}
	edits := actions[0].Edit.Changes[uri]
	if len(edits) != 1 || edits[0].NewText != "" || edits[0].Range.Start.Line != 0 {
		t.Fatalf("unexpected quick fix edits: %#v", edits)
	}
}

func TestServerProvidesHoverAndSignatureHelp(t *testing.T) {
	filename := cleanPath("semantic.trb")
	source := "def greet(name: String, suffix: String): String\n\treturn \"Hello, \" + name + suffix\nend\n\ngreet(\"Ada\", \"!\")\n"
	uri := uriFromPath(filename)
	hoverOffset := strings.LastIndex(source, "greet") + len("gr")
	signatureOffset := strings.LastIndex(source, `"!"`) + len(`"!"`)
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/hover", Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: positionAt([]byte(source), hoverOffset)})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "textDocument/signatureHelp", Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: positionAt([]byte(source), signatureOffset)})},
		message{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Version: "test", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "semantic", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", Package: "main", ModulePath: "semantic", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 5 {
		t.Fatalf("response count=%d, want 5: %s", len(frames), output.String())
	}

	var initialized initializeResult
	decodeResult(t, frames[0], &initialized)
	if !initialized.Capabilities.HoverProvider || len(initialized.Capabilities.SignatureHelpProvider.TriggerCharacters) != 2 {
		t.Fatalf("semantic capabilities=%#v", initialized.Capabilities)
	}
	var hover hoverResult
	decodeResult(t, frames[2], &hover)
	if !strings.Contains(hover.Contents.Value, "greet(name: String, suffix: String): String") {
		t.Fatalf("hover=%#v", hover)
	}
	var signatures signatureHelpResult
	decodeResult(t, frames[3], &signatures)
	if len(signatures.Signatures) != 1 || signatures.ActiveParameter != 1 || signatures.Signatures[0].Parameters[1].Label != "suffix: String" {
		t.Fatalf("signature help=%#v", signatures)
	}
}

func TestServerNavigatesToImportedTypeAndMemberDefinitions(t *testing.T) {
	root := t.TempDir()
	modelPath := cleanPath(filepath.Join(root, "models", "user.trb"))
	mainPath := cleanPath(filepath.Join(root, "main.trb"))
	modelSource := "record User\n\tname: String\nend\n"
	mainSource := "import { User } from models/user\n\ndef user_name(user: User): String\n\treturn user.name\nend\n\ndef main()\n\tuser := User.new(name: \"Ada\")\n\tputs(user_name(user))\n\treturn\nend\n"
	uri := uriFromPath(mainPath)
	typeOffset := strings.LastIndex(mainSource, "User.new") + len("Us")
	memberOffset := strings.Index(mainSource, "user.name") + len("user.na")
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: mainSource}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/definition", Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: positionAt([]byte(mainSource), typeOffset)})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "textDocument/definition", Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: positionAt([]byte(mainSource), memberOffset)})},
		message{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Version: "test", Input: bytes.NewReader(input), Output: &output,
		Units: []compiler.SourceUnit{
			{Filename: modelPath, ModulePath: "models/user", Package: "main", Source: []byte(modelSource)},
			{Filename: mainPath, ModulePath: "main", Package: "main", Source: []byte(mainSource)},
		},
		CompilerOptions: compiler.Options{Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 5 {
		t.Fatalf("response count=%d, want 5: %s", len(frames), output.String())
	}
	var initialized initializeResult
	decodeResult(t, frames[0], &initialized)
	if !initialized.Capabilities.DefinitionProvider {
		t.Fatalf("definition capability=%#v", initialized.Capabilities)
	}
	var typeLocation location
	decodeResult(t, frames[2], &typeLocation)
	if typeLocation.URI != uriFromPath(modelPath) || typeLocation.Range.Start != (position{Line: 0, Character: 7}) {
		t.Fatalf("type definition=%#v", typeLocation)
	}
	var memberLocation location
	decodeResult(t, frames[3], &memberLocation)
	if memberLocation.URI != uriFromPath(modelPath) || memberLocation.Range.Start != (position{Line: 1, Character: 1}) {
		t.Fatalf("member definition=%#v", memberLocation)
	}
}

func TestServerNavigatesFromJSXUseToImportedComponentDefinition(t *testing.T) {
	root := t.TempDir()
	pagePath := cleanPath(filepath.Join(root, "features", "insurers", "components", "InsurerPage", "index.trb"))
	entryPath := cleanPath(filepath.Join(root, "routes", "insurers.trb"))
	pageSource := "import { ReactNode } from trb/platform/typescript/react\n\ndef InsurerPage(): ReactNode\n\treturn <p>Ready</p>\nend\n"
	modulePath := "features/insurers/components/InsurerPage"
	entrySource := "import { InsurerPage } from " + modulePath + "\nimport { ReactNode } from trb/platform/typescript/react\n\ndef InsurerListRoutePage(): ReactNode\n\treturn <InsurerPage />\nend\n"
	uri := uriFromPath(entryPath)
	pathStart := strings.Index(entrySource, modulePath)
	pathOffset := pathStart + strings.Index(modulePath, "/components")
	componentOffset := strings.LastIndex(entrySource, "InsurerPage") + len("Insurer")
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: entrySource}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/definition", Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: positionAt([]byte(entrySource), pathOffset)})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "textDocument/definition", Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: positionAt([]byte(entrySource), componentOffset)})},
		message{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "typescript", Version: "test", Input: bytes.NewReader(input), Output: &output,
		Units: []compiler.SourceUnit{
			{Filename: pagePath, ModulePath: "features/insurers/components/InsurerPage/index", Source: []byte(pageSource)},
			{Filename: entryPath, ModulePath: "routes/insurers", Source: []byte(entrySource)},
		},
		CompilerOptions: compiler.Options{Mode: "typescript", ModulePath: "routes/insurers", TypeScriptRuntime: "browser"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 5 {
		t.Fatalf("response count=%d, want 5: %s", len(frames), output.String())
	}
	var moduleLink locationLink
	decodeResult(t, frames[2], &moduleLink)
	wantOrigin := offsetRange([]byte(entrySource), pathStart, pathStart+len(modulePath))
	if moduleLink.TargetURI != uriFromPath(pagePath) || moduleLink.OriginSelectionRange != wantOrigin || moduleLink.TargetSelectionRange.Start != (position{}) {
		t.Fatalf("module definition link=%#v, want complete import path to %s", moduleLink, pagePath)
	}
	var componentLocation location
	decodeResult(t, frames[3], &componentLocation)
	if componentLocation.URI != uriFromPath(pagePath) || componentLocation.Range.Start != (position{Line: 2, Character: 4}) {
		t.Fatalf("JSX component definition=%#v", componentLocation)
	}
}

func TestServerFindsInterfaceImplementations(t *testing.T) {
	filename := cleanPath(filepath.Join(t.TempDir(), "renderers.trb"))
	source := `interface Renderer
	render(input: String): String
end

class HTMLRenderer implements Renderer
	def render(input: String): String
		return "<p>" + input + "</p>"
	end
end

class TextRenderer implements Renderer
	def render(input: String): String
		return input
	end
end
`
	uri := uriFromPath(filename)
	methodOffset := strings.Index(source, "render(input") + len("render")
	typeOffset := strings.Index(source, "Renderer") + len("Ren")
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: source}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/implementation", Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: positionAt([]byte(source), methodOffset)})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "textDocument/implementation", Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: positionAt([]byte(source), typeOffset)})},
		message{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Version: "test", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "renderers", Package: "main", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "go", Package: "main", ModulePath: "renderers", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 5 {
		t.Fatalf("response count=%d, want 5: %s", len(frames), output.String())
	}
	var initialized initializeResult
	decodeResult(t, frames[0], &initialized)
	if !initialized.Capabilities.ImplementationProvider {
		t.Fatalf("implementation capability=%#v", initialized.Capabilities)
	}
	for index, wantLines := range [][]int{{5, 11}, {4, 10}} {
		var locations []location
		decodeResult(t, frames[index+2], &locations)
		if len(locations) != 2 {
			t.Fatalf("implementations[%d]=%#v", index, locations)
		}
		for locationIndex, wantLine := range wantLines {
			if locations[locationIndex].URI != uri || locations[locationIndex].Range.Start.Line != wantLine {
				t.Fatalf("implementations[%d][%d]=%#v, want line %d", index, locationIndex, locations[locationIndex], wantLine)
			}
		}
	}
}

func TestServerFindsReferencesAndRenamesAcrossProjectFiles(t *testing.T) {
	root := t.TempDir()
	modelPath := cleanPath(filepath.Join(root, "models", "user.trb"))
	mainPath := cleanPath(filepath.Join(root, "main.trb"))
	modelSource := "record User\n\tname: String\nend\n"
	mainSource := "import { User } from models/user\n\ndef user_name(user: User): String\n\tlocal_name := user.name\n\treturn local_name\nend\n\ndef main()\n\tuser := User.new(name: \"Ada\")\n\tputs(user_name(user))\n\treturn\nend\n"
	uri := uriFromPath(mainPath)
	memberOffset := strings.Index(mainSource, "user.name") + len("user.na")
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "textDocument/didOpen", Params: rawParams(t, didOpenParams{TextDocument: textDocumentItem{URI: uri, LanguageID: "trb", Version: 1, Text: mainSource}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "textDocument/references", Params: rawParams(t, referenceParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: positionAt([]byte(mainSource), memberOffset), Context: referenceContext{IncludeDeclaration: true}})},
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "textDocument/prepareRename", Params: rawParams(t, documentPositionParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: positionAt([]byte(mainSource), memberOffset)})},
		message{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "textDocument/rename", Params: rawParams(t, renameParams{TextDocument: textDocumentIdentifier{URI: uri}, Position: positionAt([]byte(mainSource), memberOffset), NewName: "display_name"})},
		message{JSONRPC: "2.0", ID: json.RawMessage("5"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "go", Version: "test", Input: bytes.NewReader(input), Output: &output,
		Units: []compiler.SourceUnit{
			{Filename: modelPath, ModulePath: "models/user", Package: "main", Source: []byte(modelSource)},
			{Filename: mainPath, ModulePath: "main", Package: "main", Source: []byte(mainSource)},
		},
		CompilerOptions: compiler.Options{Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/lsp"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 6 {
		t.Fatalf("response count=%d, want 6: %s", len(frames), output.String())
	}
	var initialized initializeResult
	decodeResult(t, frames[0], &initialized)
	if !initialized.Capabilities.ReferencesProvider || !initialized.Capabilities.RenameProvider.PrepareProvider {
		t.Fatalf("semantic navigation capabilities=%#v", initialized.Capabilities)
	}
	var references []location
	decodeResult(t, frames[2], &references)
	if len(references) != 3 {
		t.Fatalf("references=%#v, want field declaration, member use, and constructor keyword", references)
	}
	var prepare prepareRenameResult
	decodeResult(t, frames[3], &prepare)
	if prepare.Placeholder != "name" || prepare.Range != offsetRange([]byte(mainSource), memberOffset-len("na"), memberOffset+len("me")) {
		t.Fatalf("prepare rename=%#v", prepare)
	}
	var edit workspaceEdit
	decodeResult(t, frames[4], &edit)
	if len(edit.Changes[uriFromPath(modelPath)]) != 1 || len(edit.Changes[uri]) != 2 {
		t.Fatalf("rename changes=%#v", edit.Changes)
	}
	for _, edits := range edit.Changes {
		for _, item := range edits {
			if item.NewText != "display_name" {
				t.Fatalf("rename edit=%#v", item)
			}
		}
	}
}

func TestRenameIdentifierValidationRejectsKeywordsAndPunctuation(t *testing.T) {
	for _, name := range []string{"", "class", "try", "catch", "user-name", "@name", "two words"} {
		if validRenameIdentifier(name) {
			t.Fatalf("validRenameIdentifier(%q)=true", name)
		}
	}
	for _, name := range []string{"user", "User", "ready?", "save!", "attempt", "fails", "利用者"} {
		if !validRenameIdentifier(name) {
			t.Fatalf("validRenameIdentifier(%q)=false", name)
		}
	}
}

func TestServerPublishesProjectDiagnosticsAfterInitialized(t *testing.T) {
	filename := cleanPath("broken.trb")
	source := "def broken(): String\n\treturn missing()\nend\n"
	input := framedMessages(t,
		message{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", Method: "initialized", Params: json.RawMessage(`{}`)},
		message{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "shutdown", Params: json.RawMessage(`null`)},
		message{JSONRPC: "2.0", Method: "exit"},
	)
	var output bytes.Buffer
	server := New(Options{
		Mode: "ruby", Input: bytes.NewReader(input), Output: &output,
		Units:           []compiler.SourceUnit{{Filename: filename, ModulePath: "broken", Source: []byte(source)}},
		CompilerOptions: compiler.Options{Mode: "ruby"},
	})
	if err := server.Run(); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	if len(frames) != 3 {
		t.Fatalf("response count=%d, want 3", len(frames))
	}
	var published publishDiagnosticsParams
	decodeParamsFrame(t, frames[1], &published)
	if published.URI != uriFromPath(filename) || len(published.Diagnostics) != 1 || published.Diagnostics[0].Code != "TRB3000" {
		t.Fatalf("unexpected initialized diagnostics: %#v", published)
	}
}

func TestFileRootServerRebuildsImportsFromOpenBuffers(t *testing.T) {
	root := t.TempDir()
	entry := cleanPath(filepath.Join(root, "main.trb"))
	helper := cleanPath(filepath.Join(root, "helper.trb"))
	entrySource := "def main()\n\treturn\nend\n"
	importedEntry := "import { helper } from helper\n\ndef main()\n\tputs(helper())\n\treturn\nend\n"
	helperSource := "def helper(): Integer\n\treturn 1\nend\n"
	workspace := &testFileRootWorkspace{
		root: root, entry: entry,
		disk: map[string][]byte{entry: []byte(entrySource)},
	}
	var output bytes.Buffer
	server := newTestFileRootServer(t, workspace, &output)

	if err := server.open(textDocumentItem{URI: uriFromPath(helper), LanguageID: "trb", Version: 1, Text: helperSource}); err != nil {
		t.Fatal(err)
	}
	if err := server.open(textDocumentItem{URI: uriFromPath(entry), LanguageID: "trb", Version: 1, Text: entrySource}); err != nil {
		t.Fatal(err)
	}
	if len(server.base) != 1 || server.fileRootFiles[helper] {
		t.Fatalf("unimported open helper entered file-root graph: base=%#v files=%#v", server.base, server.currentFileRootFiles())
	}

	output.Reset()
	if err := server.change(didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uriFromPath(entry), Version: 2},
		ContentChanges: []contentChange{{Text: importedEntry}},
	}); err != nil {
		t.Fatal(err)
	}
	if len(server.base) != 2 || !server.fileRootFiles[helper] || server.snapshot.HasErrors() {
		t.Fatalf("entry import did not add helper: base=%#v snapshot=%#v", server.base, server.snapshot)
	}
	if countMethod(decodeFrames(t, output.Bytes()), "typerb/fileRootFilesChanged") != 1 {
		t.Fatalf("adding an import did not emit one graph notification: %s", output.String())
	}

	invalidHelper := "def helper(): Integer\n\treturn \"dirty\"\nend\n"
	output.Reset()
	if err := server.change(didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uriFromPath(helper), Version: 2},
		ContentChanges: []contentChange{{Text: invalidHelper}},
	}); err != nil {
		t.Fatal(err)
	}
	if !server.snapshot.HasErrors() || string(server.base[helper].Source) != invalidHelper {
		t.Fatalf("dirty helper overlay was not compiled: base=%q snapshot=%#v", server.base[helper].Source, server.snapshot)
	}
	if countMethod(decodeFrames(t, output.Bytes()), "typerb/fileRootFilesChanged") != 0 {
		t.Fatalf("content-only helper edit emitted a graph notification: %s", output.String())
	}

	savedHelper := "def helper(): Integer\n\treturn 2\nend\n"
	if _, err := server.handle(message{
		JSONRPC: "2.0", Method: "textDocument/didSave",
		Params: rawParams(t, didSaveParams{TextDocument: textDocumentIdentifier{URI: uriFromPath(helper)}, Text: &savedHelper}),
	}); err != nil {
		t.Fatal(err)
	}
	if server.snapshot.HasErrors() || string(server.base[helper].Source) != savedHelper {
		t.Fatalf("saved helper text was not compiled from the open buffer: base=%q snapshot=%#v", server.base[helper].Source, server.snapshot)
	}

	output.Reset()
	if err := server.change(didChangeParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uriFromPath(entry), Version: 3},
		ContentChanges: []contentChange{{Text: entrySource}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, exists := server.documents[helper]; !exists {
		t.Fatal("removing an import discarded the helper's open-buffer state")
	}
	if _, exists := server.base[helper]; exists || server.fileRootFiles[helper] {
		t.Fatalf("open helper remained in compilation after import removal: base=%#v files=%#v", server.base, server.currentFileRootFiles())
	}
	if server.snapshot.HasErrors() || len(server.snapshot.Artifacts) != 1 {
		t.Fatalf("removed helper remained in compiler snapshot: %#v", server.snapshot)
	}
	if _, visible := server.document(helper); visible {
		t.Fatal("unreachable helper remained visible to language-service requests")
	}
	if countMethod(decodeFrames(t, output.Bytes()), "typerb/fileRootFilesChanged") != 1 {
		t.Fatalf("removing an import did not emit one graph notification: %s", output.String())
	}

	output.Reset()
	if _, err := server.handle(message{JSONRPC: "2.0", ID: json.RawMessage("7"), Method: "typerb/fileRootFiles"}); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var files []string
	decodeResult(t, frames[0], &files)
	if !reflect.DeepEqual(files, []string{entry}) {
		t.Fatalf("file-root ownership response=%#v", files)
	}
}

func TestFileRootServerRebuildsOnHelperCreateAndDelete(t *testing.T) {
	root := t.TempDir()
	entry := cleanPath(filepath.Join(root, "main.trb"))
	helper := cleanPath(filepath.Join(root, "helper.trb"))
	entrySource := "import { helper } from helper\n\ndef main()\n\tputs(helper())\n\treturn\nend\n"
	helperSource := "def helper(): Integer\n\treturn 1\nend\n"
	workspace := &testFileRootWorkspace{
		root: root, entry: entry,
		disk: map[string][]byte{entry: []byte(entrySource)},
	}
	var output bytes.Buffer
	server := newTestFileRootServer(t, workspace, &output)
	if server.fileRootFiles[helper] {
		t.Fatal("missing helper entered the initial graph")
	}

	workspace.disk[helper] = []byte(helperSource)
	if err := server.changeWorkspaceFiles(didChangeWatchedFilesParams{Changes: []fileEvent{{URI: uriFromPath(helper), Type: fileChangeCreated}}}); err != nil {
		t.Fatal(err)
	}
	if !server.fileRootFiles[helper] || len(server.base) != 2 || server.snapshot.HasErrors() || len(server.snapshot.Artifacts) != 2 {
		t.Fatalf("created helper was not added: files=%#v snapshot=%#v", server.currentFileRootFiles(), server.snapshot)
	}

	delete(workspace.disk, helper)
	if err := server.changeWorkspaceFiles(didChangeWatchedFilesParams{Changes: []fileEvent{{URI: uriFromPath(helper), Type: fileChangeDeleted}}}); err != nil {
		t.Fatal(err)
	}
	if server.fileRootFiles[helper] || len(server.base) != 1 || !server.snapshot.HasErrors() {
		t.Fatalf("deleted helper was not removed: files=%#v snapshot=%#v", server.currentFileRootFiles(), server.snapshot)
	}
}

func TestFileRootServerHandlesImportCycles(t *testing.T) {
	root := t.TempDir()
	entry := cleanPath(filepath.Join(root, "main.trb"))
	helper := cleanPath(filepath.Join(root, "helper.trb"))
	entrySource := "import { helper_value } from helper\n\ndef entry_value(): Integer\n\treturn 1\nend\n\ndef main()\n\tputs(helper_value())\n\treturn\nend\n"
	helperSource := "import { entry_value } from main\n\ndef helper_value(): Integer\n\treturn entry_value()\nend\n"
	workspace := &testFileRootWorkspace{
		root: root, entry: entry,
		disk: map[string][]byte{entry: []byte(entrySource), helper: []byte(helperSource)},
	}
	var output bytes.Buffer
	server := newTestFileRootServer(t, workspace, &output)
	if err := server.open(textDocumentItem{URI: uriFromPath(entry), LanguageID: "trb", Version: 1, Text: entrySource}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(server.currentFileRootFiles(), []string{helper, entry}) {
		t.Fatalf("cyclic graph files=%#v", server.currentFileRootFiles())
	}
	if workspace.resolutions != 2 {
		t.Fatalf("cyclic graph resolution count=%d, want initial load plus open refresh", workspace.resolutions)
	}
}

func TestFileRootServerOmitsRunCodeLensFromImportedHelper(t *testing.T) {
	root := t.TempDir()
	entry := cleanPath(filepath.Join(root, "main.trb"))
	helper := cleanPath(filepath.Join(root, "helper.trb"))
	entrySource := "import { value } from helper\n\ndef answer(): Integer\n\treturn value()\nend\n"
	helperSource := "def value(): Integer\n\treturn 1\nend\n\ndef main()\n\treturn\nend\n"
	workspace := &testFileRootWorkspace{
		root: root, entry: entry,
		disk: map[string][]byte{entry: []byte(entrySource), helper: []byte(helperSource)},
	}
	var output bytes.Buffer
	server := newTestFileRootServer(t, workspace, &output)
	request := message{
		JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "textDocument/codeLens",
		Params: rawParams(t, documentParams{TextDocument: textDocumentIdentifier{URI: uriFromPath(helper)}}),
	}
	if err := server.codeLenses(request); err != nil {
		t.Fatal(err)
	}
	frames := decodeFrames(t, output.Bytes())
	var lenses []codeLens
	decodeResult(t, frames[0], &lenses)
	if len(lenses) != 0 {
		t.Fatalf("imported helper code lenses=%#v", lenses)
	}
}

func TestProtocolPositionsUseUTF16CharactersAndByteOffsets(t *testing.T) {
	source := []byte("value := \"😀\"\n")
	offset := len("value := \"😀")
	position := positionAt(source, offset)
	if position.Line != 0 || position.Character != len("value := \"")+2 {
		t.Fatalf("position=%#v", position)
	}
	if restored := offsetAt(source, position); restored != offset {
		t.Fatalf("offsetAt(%#v)=%d, want %d", position, restored, offset)
	}
}

type testFileRootWorkspace struct {
	root        string
	entry       string
	disk        map[string][]byte
	resolutions int
}

func (w *testFileRootWorkspace) resolve(overlays map[string][]byte) ([]compiler.SourceUnit, error) {
	w.resolutions++
	read := func(path string) ([]byte, bool) {
		path = cleanPath(path)
		if source, exists := overlays[path]; exists {
			return append([]byte(nil), source...), true
		}
		source, exists := w.disk[path]
		return append([]byte(nil), source...), exists
	}
	entrySource, exists := read(w.entry)
	if !exists {
		return nil, fmt.Errorf("missing test entry %s", w.entry)
	}
	sources := map[string][]byte{w.entry: entrySource}
	queue := []string{w.entry}
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		program, _ := parser.Parse(sources[path])
		for _, statement := range program.Statements {
			imported, ok := statement.(*ast.ImportStatement)
			if !ok || strings.HasPrefix(imported.Path, "trb/") {
				continue
			}
			module := filepath.Clean(filepath.FromSlash(strings.TrimSuffix(imported.Path, ".trb")))
			if module == "." || module == ".." || strings.HasPrefix(module, ".."+string(filepath.Separator)) || filepath.IsAbs(module) {
				continue
			}
			candidates := []string{filepath.Join(w.root, module+".trb"), filepath.Join(w.root, module, "index.trb")}
			for _, candidate := range candidates {
				candidate = cleanPath(candidate)
				source, found := read(candidate)
				if !found {
					continue
				}
				if _, seen := sources[candidate]; !seen {
					sources[candidate] = source
					queue = append(queue, candidate)
				}
				break
			}
		}
	}
	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	units := make([]compiler.SourceUnit, 0, len(paths))
	for _, path := range paths {
		unit, err := w.unit(path, sources[path])
		if err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	return units, nil
}

func (w *testFileRootWorkspace) unit(path string, source []byte) (compiler.SourceUnit, error) {
	relative, err := filepath.Rel(w.root, cleanPath(path))
	if err != nil {
		return compiler.SourceUnit{}, err
	}
	return compiler.SourceUnit{
		Filename: cleanPath(path), ModulePath: filepath.ToSlash(strings.TrimSuffix(relative, filepath.Ext(relative))),
		Package: "main", Source: append([]byte(nil), source...),
	}, nil
}

func newTestFileRootServer(t *testing.T, workspace *testFileRootWorkspace, output io.Writer) *Server {
	t.Helper()
	units, err := workspace.resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return New(Options{
		Mode: "go", Units: units, Output: output,
		CompilerOptions: compiler.Options{Mode: "go", GoModule: "example.com/file-root-lsp", SourceRoot: workspace.root},
		IncludedFiles:   []string{workspace.entry}, ResolveUnit: workspace.unit, ResolveWorkspace: workspace.resolve,
	})
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

type failAfterWriter struct {
	mu               sync.Mutex
	writes           int
	successfulWrites int
	err              error
}

func (w *failAfterWriter) Write(contents []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writes >= w.successfulWrites {
		return 0, w.err
	}
	w.writes++
	return len(contents), nil
}

func (b *synchronizedBuffer) Write(contents []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(contents)
}

func (b *synchronizedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buffer.Bytes()...)
}

func (b *synchronizedBuffer) Reset() {
	b.mu.Lock()
	b.buffer.Reset()
	b.mu.Unlock()
}

func (b *synchronizedBuffer) String() string {
	return string(b.Bytes())
}

func enableTestBackgroundDiagnostics(server *Server) {
	server.diagnostics = newDiagnosticCoordinator(0, server.compiler.AnalyzeOnce, server.compiler.Generation)
}

func waitForDiagnosticResult(t *testing.T, results <-chan diagnosticResult, description string) diagnosticResult {
	t.Helper()
	select {
	case result, ok := <-results:
		if !ok {
			t.Fatalf("diagnostics coordinator stopped while waiting for %s", description)
		}
		return result
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
		return diagnosticResult{}
	}
}

func waitForTestSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForTestCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", description)
		case <-ticker.C:
		}
	}
}

func countMethod(frames []map[string]json.RawMessage, method string) int {
	count := 0
	for _, frame := range frames {
		var current string
		if err := json.Unmarshal(frame["method"], &current); err == nil && current == method {
			count++
		}
	}
	return count
}

func framedMessages(t *testing.T, messages ...message) []byte {
	t.Helper()
	var result bytes.Buffer
	for _, item := range messages {
		payload, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&result, "Content-Length: %d\r\n\r\n", len(payload))
		result.Write(payload)
	}
	return result.Bytes()
}

func rawParams(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func decodeFrames(t *testing.T, data []byte) []map[string]json.RawMessage {
	t.Helper()
	reader := bufio.NewReader(bytes.NewReader(data))
	var result []map[string]json.RawMessage
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			return result
		}
		if err != nil {
			t.Fatal(err)
		}
		name, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found || !strings.EqualFold(name, "Content-Length") {
			t.Fatalf("invalid frame header %q", line)
		}
		length, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			t.Fatal(err)
		}
		if blank, err := reader.ReadString('\n'); err != nil || strings.TrimSpace(blank) != "" {
			t.Fatalf("invalid frame separator %q err=%v", blank, err)
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(reader, payload); err != nil {
			t.Fatal(err)
		}
		var frame map[string]json.RawMessage
		if err := json.Unmarshal(payload, &frame); err != nil {
			t.Fatalf("invalid frame %s: %v", payload, err)
		}
		result = append(result, frame)
	}
}

func decodeResult(t *testing.T, frame map[string]json.RawMessage, target any) {
	t.Helper()
	if err := json.Unmarshal(frame["result"], target); err != nil {
		t.Fatalf("invalid result %s: %v", frame["result"], err)
	}
}

func decodeParamsFrame(t *testing.T, frame map[string]json.RawMessage, target any) {
	t.Helper()
	if err := json.Unmarshal(frame["params"], target); err != nil {
		t.Fatalf("invalid params %s: %v", frame["params"], err)
	}
}

func containsCompletion(items []completionItem, label string) bool {
	for _, item := range items {
		if item.Label == label {
			return true
		}
	}
	return false
}

func containsLanguageCompletion(items []languageservice.CompletionItem, label string) bool {
	for _, item := range items {
		if item.Label == label {
			return true
		}
	}
	return false
}
