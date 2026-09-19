package typescript

import "encoding/hex"

// Source bindings are portable identifiers, not JavaScript keywords. The dollar
// namespace cannot be authored in TypeRB and keeps this encoding collision-free.
func tsBindingName(name string) string {
	switch name {
	case "await", "break", "case", "catch", "class", "const", "continue",
		"debugger", "default", "delete", "do", "else", "enum", "export",
		"extends", "false", "finally", "for", "function", "if", "import",
		"in", "instanceof", "new", "null", "return", "super", "switch",
		"this", "throw", "true", "try", "typeof", "var", "void", "while",
		"with", "yield", "let", "static", "implements", "interface",
		"package", "private", "protected", "public", "arguments", "eval":
		return "$trb$binding$" + hex.EncodeToString([]byte(name))
	}
	return name
}
