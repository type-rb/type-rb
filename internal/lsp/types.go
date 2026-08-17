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
	Range       *rangeValue `json:"range,omitempty"`
	RangeLength *int        `json:"rangeLength,omitempty"`
	Text        string      `json:"text"`
}

type didChangeParams struct {
	TextDocument   versionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []contentChange                 `json:"contentChanges"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type fileEvent struct {
	URI  string `json:"uri"`
	Type int    `json:"type"`
}

type didChangeWatchedFilesParams struct {
	Changes []fileEvent `json:"changes"`
}

type documentPositionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     position               `json:"position"`
}

type selectionRangeParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Positions    []position             `json:"positions"`
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

type workspaceSymbolParams struct {
	Query string `json:"query"`
}

type testItem struct {
	ID       string     `json:"id"`
	ParentID string     `json:"parentId,omitempty"`
	Kind     string     `json:"kind"`
	Name     string     `json:"name"`
	FullName string     `json:"fullName"`
	URI      string     `json:"uri"`
	Range    rangeValue `json:"range"`
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

type documentHighlight struct {
	Range rangeValue `json:"range"`
	Kind  int        `json:"kind,omitempty"`
}

type selectionRange struct {
	Range  rangeValue      `json:"range"`
	Parent *selectionRange `json:"parent,omitempty"`
}

type textEdit struct {
	Range   rangeValue `json:"range"`
	NewText string     `json:"newText"`
}

type completionItem struct {
	Label               string     `json:"label"`
	Kind                int        `json:"kind,omitempty"`
	Detail              string     `json:"detail,omitempty"`
	TextEdit            textEdit   `json:"textEdit"`
	AdditionalTextEdits []textEdit `json:"additionalTextEdits,omitempty"`
}

type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          rangeValue       `json:"range"`
	SelectionRange rangeValue       `json:"selectionRange"`
	Children       []documentSymbol `json:"children,omitempty"`
}

type foldingRange struct {
	StartLine      int `json:"startLine"`
	StartCharacter int `json:"startCharacter"`
	EndLine        int `json:"endLine"`
	EndCharacter   int `json:"endCharacter"`
}

type semanticTokens struct {
	Data []int `json:"data"`
}

type symbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
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

type command struct {
	Title     string        `json:"title"`
	Command   string        `json:"command"`
	Arguments []interface{} `json:"arguments,omitempty"`
}

type codeLens struct {
	Range   rangeValue `json:"range"`
	Command command    `json:"command"`
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
	TextDocumentSync           int                   `json:"textDocumentSync"`
	CompletionProvider         completionOptions     `json:"completionProvider"`
	HoverProvider              bool                  `json:"hoverProvider"`
	SignatureHelpProvider      signatureOptions      `json:"signatureHelpProvider"`
	DefinitionProvider         bool                  `json:"definitionProvider"`
	ImplementationProvider     bool                  `json:"implementationProvider"`
	ReferencesProvider         bool                  `json:"referencesProvider"`
	DocumentHighlightProvider  bool                  `json:"documentHighlightProvider"`
	SelectionRangeProvider     bool                  `json:"selectionRangeProvider"`
	RenameProvider             renameOptions         `json:"renameProvider"`
	DocumentSymbolProvider     bool                  `json:"documentSymbolProvider"`
	FoldingRangeProvider       bool                  `json:"foldingRangeProvider"`
	WorkspaceSymbolProvider    bool                  `json:"workspaceSymbolProvider"`
	SemanticTokensProvider     semanticTokensOptions `json:"semanticTokensProvider"`
	DocumentFormattingProvider bool                  `json:"documentFormattingProvider"`
	CodeActionProvider         bool                  `json:"codeActionProvider"`
	CodeLensProvider           *codeLensOptions      `json:"codeLensProvider"`
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

type semanticTokensOptions struct {
	Legend semanticTokensLegend `json:"legend"`
	Full   bool                 `json:"full"`
}

type codeLensOptions struct {
	ResolveProvider bool `json:"resolveProvider"`
}

type semanticTokensLegend struct {
	TokenTypes     []string `json:"tokenTypes"`
	TokenModifiers []string `json:"tokenModifiers"`
}

func decodeParams[T any](raw json.RawMessage) (T, error) {
	var result T
	err := json.Unmarshal(raw, &result)
	return result, err
}
