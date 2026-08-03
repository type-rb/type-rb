package selfhost

// Target-independent compiler values are the first self-hosted boundary.
// They deliberately contain no Go-native syntax.
type Position struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

type Span struct {
	Start  Position `json:"start"`
	Finish Position `json:"finish"`
}

type Token struct {
	Kind   string `json:"kind"`
	Lexeme string `json:"lexeme"`
	Span   Span   `json:"span"`
}

type Diagnostic struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Span     Span   `json:"span"`
}

type TypeRef struct {
	Name      string    `json:"name"`
	Arguments []TypeRef `json:"arguments"`
	Nullable  bool      `json:"nullable"`
	Array     bool      `json:"array"`
}
