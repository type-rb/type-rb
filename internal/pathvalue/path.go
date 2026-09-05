// Package pathvalue implements host path composition without filesystem access.
package pathvalue

import "strings"

// Join appends an already validated slash-relative descendant. The parent is
// preserved byte for byte, including dot components and drive-relative roots.
func Join(parent, relative string, windows bool) string {
	separator := "/"
	child := relative
	if windows {
		separator = "\\"
		child = strings.ReplaceAll(relative, "/", separator)
	}
	drive := windows && len(parent) == 2 && parent[1] == ':' &&
		(parent[0] >= 'A' && parent[0] <= 'Z' || parent[0] >= 'a' && parent[0] <= 'z')
	if parent == "" || drive || strings.HasSuffix(parent, "/") || windows && strings.HasSuffix(parent, "\\") {
		return parent + child
	}
	return parent + separator + child
}
