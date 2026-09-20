package ruby

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

// Sequential stores preserve all entry effects and avoid target warnings for
// duplicate literal keys, which are valid overwrites in portable TypeRB.
func (g *generator) hashLiteral(hash *ir.Hash) string {
	if len(hash.Entries) == 0 {
		return "{}"
	}
	g.temporary++
	result := "__trb_hash_" + strconv.Itoa(g.temporary)
	var body strings.Builder
	body.WriteString("(begin; " + result + " = {}; ")
	for _, entry := range hash.Entries {
		body.WriteString(result + "[" + g.expr(entry.Key) + "] = " + g.expr(entry.Value) + "; ")
	}
	body.WriteString(result + "; end)")
	return body.String()
}
