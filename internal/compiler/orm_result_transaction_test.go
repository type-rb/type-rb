package compiler

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

func TestORMResultTransactionWrappersLowerAsDirectStatementValues(t *testing.T) {
	root := t.TempDir()
	prepareORMResultTransactionCompilerDatabase(t, root)
	source := SourceUnit{
		Filename:   filepath.Join(root, "src", "main.trb"),
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { Database, DbResult } from trb/orm

def caught(): Integer
	value := Database.transaction() do |_tx|
		11
	end catch |_error|
		-1
	end
	return value
end

def propagated(): DbResult<Integer>
	value := try Database.transaction() do |_tx|
		22
	end
	return DbResult<Integer>::Ok(value)
end

def returned(): Integer
	return Database.transaction() do |_tx|
		33
	end catch |_error|
		-1
	end
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{source}, Options{
				Mode: mode, GoModule: "example.com/result-transaction-lowering", RubyLoader: "require_relative", TypeScriptRuntime: "bun",
				SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
				PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
			})
			if err != nil {
				t.Fatalf("%s rejected direct Result transaction wrappers: %v", mode, err)
			}
			var consumer *Artifact
			for _, artifact := range artifacts {
				if artifact.IR.ModulePath == source.ModulePath {
					consumer = artifact
					break
				}
			}
			if consumer == nil {
				t.Fatalf("%s did not produce the transaction consumer", mode)
			}
			methods := map[string]*ir.Method{}
			for _, statement := range consumer.IR.Statements {
				if method, ok := statement.(*ir.Method); ok {
					methods[method.Name] = method
				}
			}
			for _, name := range []string{"caught", "propagated", "returned"} {
				method := methods[name]
				if method == nil || len(method.Body) < 3 {
					t.Fatalf("%s IR is missing the structured %s() body: %#v", mode, name, method)
				}
				if _, ok := method.Body[0].(*ir.Temporary); !ok {
					t.Fatalf("%s %s() first statement is %T, want *ir.Temporary", mode, name, method.Body[0])
				}
				block, ok := method.Body[1].(*ir.StructuredBlock)
				if !ok {
					t.Fatalf("%s %s() second statement is %T, want *ir.StructuredBlock", mode, name, method.Body[1])
				}
				if block.Result == nil || block.Result.Target == nil || block.Result.Type.String() != "Result<Integer, DbError>" {
					t.Fatalf("%s %s() structured Result target=%#v", mode, name, block.Result)
				}
				if name == "returned" {
					if _, ok := method.Body[2].(*ir.Return); !ok {
						t.Fatalf("%s %s() third statement is %T, want authored *ir.Return", mode, name, method.Body[2])
					}
				} else if _, ok := method.Body[2].(*ir.Variable); !ok {
					t.Fatalf("%s %s() third statement is %T, want authored *ir.Variable", mode, name, method.Body[2])
				}
			}
		})
	}
}

func TestORMResultTransactionWrappersRejectDeeperExpressionNesting(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "try as if branch result",
			source: `import { Database, DbResult } from trb/orm

def invalid(): DbResult<Integer>
	value := if true
		try Database.transaction() do |_tx|
			1
		end
	else
		0
	end
	return DbResult<Integer>::Ok(value)
end
`,
		},
		{
			name: "catch as if branch result",
			source: `import { Database } from trb/orm

def invalid(): Integer
	value := if true
		Database.transaction() do |_tx|
			1
		end catch |_error|
			0
		end
	else
		0
	end
	return value
end
`,
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				root := t.TempDir()
				prepareORMResultTransactionCompilerDatabase(t, root)
				_, err := CompileProject([]SourceUnit{{
					Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main", Source: []byte(test.source),
				}}, Options{
					Mode: mode, GoModule: "example.com/result-transaction-diagnostic", RubyLoader: "require_relative", TypeScriptRuntime: "bun",
					SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
					PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
				})
				want := "structured block transaction() must be the direct value"
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("expected %q diagnostic, got %v", want, err)
				}
			})
		}
	}
}

func TestORMResultTransactionRejectsAuthoredReturnAcrossBackends(t *testing.T) {
	sources := map[string]string{
		"direct": `import { Database, DbResult } from trb/orm

def invalid(): DbResult<Integer>
	return Database.transaction() do |_tx|
		if true
			return 1
		end
		2
	end
end
	`,
		"catch handler": `import { Database, DbResult } from trb/orm

def invalid(): DbResult<Integer>
	return Database.transaction() do |tx|
		value := tx.transaction() do |_nested|
			1
		end catch |_error|
			return 0
		end
		value
	end
end
`,
		"iteration": `import { Database, DbResult } from trb/orm

def invalid(): DbResult<Integer>
	return Database.transaction() do |_tx|
		[1].each do |_value|
			return 1
		end
		2
	end
end
`,
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for name, source := range sources {
			t.Run(mode+"/"+name, func(t *testing.T) {
				root := t.TempDir()
				prepareORMResultTransactionCompilerDatabase(t, root)
				_, err := CompileProject([]SourceUnit{{
					Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main", Source: []byte(source),
				}}, Options{
					Mode: mode, GoModule: "example.com/result-transaction-return", RubyLoader: "require_relative", TypeScriptRuntime: "bun",
					SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
					PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
				})
				want := "return is not supported inside Result-boundary structured blocks"
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("expected %q diagnostic, got %v", want, err)
				}
			})
		}
	}
}

func TestORMResultTransactionRejectsUnsupportedPlainAndAssignmentPositions(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "plain call in branch",
			source: `import { Database, DbResult } from trb/orm

def invalid(): DbResult<Integer>
	value := if true
		Database.transaction() do |_tx|
			1
		end
	else
		DbResult<Integer>::Ok(0)
	end
	return value
end
`,
			want: "structured block transaction() must be the direct value",
		},
		{
			name: "catch assignment",
			source: `import { Database, DbResult } from trb/orm

def invalid(): Integer
	value := 0
	value = Database.transaction() do |_tx|
		1
	end catch |_error|
		0
	end
	return value
end
`,
			want: "catch over a structured block must be the direct value of a variable declaration or return",
		},
		{
			name: "plain structured assignment requires variable target",
			source: `import { Database, DbResult } from trb/orm

def invalid(): DbResult<Integer>
	mut values := [DbResult<Integer>::Ok(0)]
	values[0] = Database.transaction() do |_tx|
		1
	end
	return values[0]
end
`,
			want: "a structured block assignment target must be a variable name",
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				root := t.TempDir()
				prepareORMResultTransactionCompilerDatabase(t, root)
				_, err := CompileProject([]SourceUnit{{
					Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main", Source: []byte(test.source),
				}}, Options{
					Mode: mode, GoModule: "example.com/result-transaction-placement", RubyLoader: "require_relative", TypeScriptRuntime: "bun",
					SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
					PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
				})
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("expected %q diagnostic, got %v", test.want, err)
				}
			})
		}
	}
}

func TestORMResultTransactionKeepsLambdaFailureOwnershipAcrossBackends(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "pure lambda rejects captured ORM failure",
			source: `import { Database, DbResult, Model } from trb/orm

class ResultTransactionItem < Model
end

def invalid(): DbResult<Integer>
	return Database.transaction() do |_tx|
		callback := fn(): Integer
			return ResultTransactionItem.count()
		end
		callback()
	end
end
`,
			want: "operation may fail with DbError, but the enclosing function does not declare it",
		},
		{
			name: "fallible lambda owns and forwards ORM failure",
			source: `import { Database, DbError, DbResult, Model } from trb/orm

class ResultTransactionItem < Model
end

def valid(): DbResult<Integer>
	return Database.transaction() do |_tx|
		callback := fn(): Integer fails DbError
			return ResultTransactionItem.count()
		end
		callback()
	end
end
`,
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				root := t.TempDir()
				prepareORMResultTransactionCompilerDatabase(t, root)
				_, err := CompileProject([]SourceUnit{{
					Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main", Source: []byte(test.source),
				}}, Options{
					Mode: mode, GoModule: "example.com/result-transaction-lambda", RubyLoader: "require_relative", TypeScriptRuntime: "bun",
					SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
					PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
				})
				if test.want == "" {
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("expected %q diagnostic, got %v", test.want, err)
				}
			})
		}
	}
}

func prepareORMResultTransactionCompilerDatabase(t *testing.T, root string) {
	t.Helper()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE result_transaction_items (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}
