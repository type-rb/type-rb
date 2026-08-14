package lsp

import "encoding/json"

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type rangeValue struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type versionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type contentChange struct {
	Text string `json:"text"`
}

type didChangeParams struct {
	TextDocument   versionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []contentChange                 `json:"contentChanges"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type documentPositionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     position               `json:"position"`
}

type referenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

type referenceParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     position               `json:"position"`
	Context      referenceContext       `json:"context"`
}

type renameParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     position               `json:"position"`
	NewName      string                 `json:"newName"`
}

type documentParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type formattingParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type codeActionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Range        rangeValue             `json:"range"`
}

type publishDiagnosticsParams struct {
	URI         string               `json:"uri"`
	Diagnostics []protocolDiagnostic `json:"diagnostics"`
}

type protocolDiagnostic struct {
	Range              rangeValue             `json:"range"`
	Severity           int                    `json:"severity"`
	Code               string                 `json:"code"`
	Source             string                 `json:"source"`
	Message            string                 `json:"message"`
	RelatedInformation []relatedInformation   `json:"relatedInformation,omitempty"`
	Data               map[string]interface{} `json:"data,omitempty"`
}

type relatedInformation struct {
	Location location `json:"location"`
	Message  string   `json:"message"`
}

type location struct {
	URI   string     `json:"uri"`
	Range rangeValue `json:"range"`
}

type textEdit struct {
	Range   rangeValue `json:"range"`
	NewText string     `json:"newText"`
}

type completionItem struct {
	Label    string   `json:"label"`
	Kind     int      `json:"kind,omitempty"`
	Detail   string   `json:"detail,omitempty"`
	TextEdit textEdit `json:"textEdit"`
}

type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          rangeValue       `json:"range"`
	SelectionRange rangeValue       `json:"selectionRange"`
	Children       []documentSymbol `json:"children,omitempty"`
}

type markupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type hoverResult struct {
	Contents markupContent `json:"contents"`
	Range    rangeValue    `json:"range"`
}

type parameterInformation struct {
	Label string `json:"label"`
}

type signatureInformation struct {
	Label      string                 `json:"label"`
	Parameters []parameterInformation `json:"parameters,omitempty"`
}

type signatureHelpResult struct {
	Signatures      []signatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

type workspaceEdit struct {
	Changes map[string][]textEdit `json:"changes"`
}

type prepareRenameResult struct {
	Range       rangeValue `json:"range"`
	Placeholder string     `json:"placeholder"`
}

type codeAction struct {
	Title string        `json:"title"`
	Kind  string        `json:"kind"`
	Edit  workspaceEdit `json:"edit"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   serverInfo         `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type serverCapabilities struct {
	TextDocumentSync           int               `json:"textDocumentSync"`
	CompletionProvider         completionOptions `json:"completionProvider"`
	HoverProvider              bool              `json:"hoverProvider"`
	SignatureHelpProvider      signatureOptions  `json:"signatureHelpProvider"`
	DefinitionProvider         bool              `json:"definitionProvider"`
	ReferencesProvider         bool              `json:"referencesProvider"`
	RenameProvider             renameOptions     `json:"renameProvider"`
	DocumentSymbolProvider     bool              `json:"documentSymbolProvider"`
	DocumentFormattingProvider bool              `json:"documentFormattingProvider"`
	CodeActionProvider         bool              `json:"codeActionProvider"`
}

type completionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters"`
}

type signatureOptions struct {
	TriggerCharacters   []string `json:"triggerCharacters"`
	RetriggerCharacters []string `json:"retriggerCharacters,omitempty"`
}

type renameOptions struct {
	PrepareProvider bool `json:"prepareProvider"`
}

func decodeParams[T any](raw json.RawMessage) (T, error) {
	var result T
	err := json.Unmarshal(raw, &result)
	return result, err
}
