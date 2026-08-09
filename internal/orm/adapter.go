package orm

import (
	"fmt"
	"strconv"
	"strings"
)

type ExplainStyle string

const (
	ExplainSQLite ExplainStyle = "sqlite"
	ExplainText   ExplainStyle = "text"
	ExplainJSON   ExplainStyle = "json"
)

// Adapter describes the database-specific behavior shared by schema
// introspection and backend runtime generation.
type Adapter struct {
	Name           string
	DriverName     string
	GoDriverImport string
	IdentifierMark string
	NumberedBinds  bool
	ExplainStyle   ExplainStyle
}

func (a Adapter) QuoteIdentifier(name string) string {
	return a.IdentifierMark + strings.ReplaceAll(name, a.IdentifierMark, a.IdentifierMark+a.IdentifierMark) + a.IdentifierMark
}

func (a Adapter) Placeholder(position int) string {
	if a.NumberedBinds {
		return "$" + strconv.Itoa(position)
	}
	return "?"
}

type adapterDefinition struct {
	Adapter
	Introspector Introspector
}

func AdapterFor(name string) (Adapter, error) {
	definition, ok := adapterDefinitionFor(strings.ToLower(strings.TrimSpace(name)))
	if !ok {
		return Adapter{}, fmt.Errorf("unsupported trb/orm adapter %q", name)
	}
	return definition.Adapter, nil
}

func adapterDefinitionFor(name string) (adapterDefinition, bool) {
	switch name {
	case "sqlite":
		return adapterDefinition{
			Adapter: Adapter{
				Name: "sqlite", DriverName: "sqlite", GoDriverImport: "modernc.org/sqlite",
				IdentifierMark: `"`, ExplainStyle: ExplainSQLite,
			},
			Introspector: sqliteIntrospector{},
		}, true
	default:
		return adapterDefinition{}, false
	}
}
