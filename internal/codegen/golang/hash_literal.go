package golang

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

// A Go map literal rejects duplicate constant keys and does not require each
// variable key to be read before calls in its value. Store authored entries in
// sequence, capturing the key before evaluating the corresponding value.
func (g *generator) hashLiteral(hash *ir.Hash) string {
	typeName := g.goType(hash.ExprType())
	if len(hash.Entries) == 0 {
		return typeName + "{}"
	}
	g.temporary++
	result := "__trbHash" + strconv.Itoa(g.temporary)
	var body strings.Builder
	body.WriteString("func() " + typeName + " { " + result + " := make(" + typeName + ", " + strconv.Itoa(len(hash.Entries)) + "); ")
	for index, entry := range hash.Entries {
		key := result + "Key" + strconv.Itoa(index)
		body.WriteString(key + " := " + g.expr(entry.Key) + "; ")
		body.WriteString(result + "[" + key + "] = " + g.expr(entry.Value) + "; ")
	}
	body.WriteString("return " + result + " }()")
	return body.String()
}
