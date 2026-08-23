package golang

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/jobs"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
)

func TestMySQLJobsRuntimeRemovesBunPublicKeyOptionFromGoDSN(t *testing.T) {
	output := Generate(&ir.Program{
		Mode: "go", Package: "sql", ModulePath: jobssql.ModulePath, GoModule: "example.com/jobs",
		Extensions: []ir.Extension{
			&jobs.Manifest{},
			&jobssql.Manifest{Config: jobssql.Config{
				Dialect: "mysql", Source: "mysql://user:password@localhost/jobs?allowPublicKeyRetrieval=true",
			}},
		},
	})
	for _, want := range []string{
		`trbmysql "github.com/go-sql-driver/mysql"`,
		`trbmysql.ParseDSN(source)`,
		`trbMySQLConfig.Params["allowPublicKeyRetrieval"]`,
		`delete(trbMySQLConfig.Params, "allowPublicKeyRetrieval")`,
		`source = trbMySQLConfig.FormatDSN()`,
		`errors.New("MySQL allowPublicKeyRetrieval must be true or false")`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated MySQL Jobs runtime is missing %q:\n%s", want, output)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "jobs.go", output, parser.AllErrors); err != nil {
		t.Fatalf("generated invalid MySQL Jobs Go: %v\n%s", err, output)
	}
}
