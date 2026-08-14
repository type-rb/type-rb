package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
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
	if initialized.ServerInfo.Name != "TypeRB" || initialized.Capabilities.TextDocumentSync != 1 || !initialized.Capabilities.CodeActionProvider {
		t.Fatalf("unexpected initialize result: %#v", initialized)
	}
	var opened publishDiagnosticsParams
	decodeParamsFrame(t, frames[1], &opened)
	if len(opened.Diagnostics) != 0 {
		t.Fatalf("valid document diagnostics=%#v", opened.Diagnostics)
	}
	var changed publishDiagnosticsParams
	decodeParamsFrame(t, frames[2], &changed)
	if len(changed.Diagnostics) == 0 || changed.Diagnostics[0].Code != "TRB3000" {
		t.Fatalf("invalid document diagnostics=%#v", changed.Diagnostics)
	}

	var completions []completionItem
	decodeResult(t, frames[3], &completions)
	if !containsCompletion(completions, "greet") {
		t.Fatalf("completion response does not contain greet: %#v", completions)
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
		message{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "shutdown", Params: json.RawMessage(`null`)},
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
	var completions []completionItem
	decodeResult(t, frames[2], &completions)
	if !containsCompletion(completions, "Integer") {
		t.Fatalf("type completion response=%#v", completions)
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
	for _, name := range []string{"", "class", "user-name", "@name", "two words"} {
		if validRenameIdentifier(name) {
			t.Fatalf("validRenameIdentifier(%q)=true", name)
		}
	}
	for _, name := range []string{"user", "User", "ready?", "save!", "利用者"} {
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
