package golang

// The boolean is supplied from the target runtime, never the compiler host.
// Keeping composition separate also permits both OS grammars to be exercised
// without running filesystem operations or a foreign executable.
func pathJoinExpression(parent, child, windows string) string {
	return `func(parent, child string, windows bool) string {
		separator := "/"
		if windows { separator = "\\"; child = strings.ReplaceAll(child, "/", separator) }
		drive := windows && len(parent) == 2 && parent[1] == ':' && ((parent[0] >= 'A' && parent[0] <= 'Z') || (parent[0] >= 'a' && parent[0] <= 'z'))
		if parent == "" || drive || strings.HasSuffix(parent, "/") || (windows && strings.HasSuffix(parent, "\\")) { return parent + child }
		return parent + separator + child
	}(` + parent + ", " + child + ", " + windows + ")"
}
