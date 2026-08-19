package compiler

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

func TestORMResultStreamingWrappersLowerAsDirectStatementValues(t *testing.T) {
	root := t.TempDir()
	prepareORMResultStreamingCompilerDatabase(t, root)
	source := SourceUnit{
		Filename:   filepath.Join(root, "src", "main.trb"),
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import { DbResult, Model } from trb/orm

class Product < Model
end

def caught(): Integer
	value := Product.find_each(batch_size: 2) do |_product|
	end catch |_error|
		-1
	end
	return value
end

def propagated(failure: DbResult<Integer>): DbResult<Integer>
	value := try Product.find_each(batch_size: 2) do |product|
		puts(product.id)
		puts(try failure)
	end
	return DbResult<Integer>::Ok(value)
end

def raw(): DbResult<Integer>
	return Product.find_in_batches(batch_size: 2) do |products|
		puts(products.size())
	end
end
`),
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{source}, Options{
				Mode: mode, GoModule: "example.com/result-streaming-lowering", RubyLoader: "require_relative", TypeScriptRuntime: "bun",
				SourceRoot: filepath.Join(root, "src"), ProjectRoot: root,
				PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3"}`)},
			})
			if err != nil {
				t.Fatalf("%s rejected direct Result streaming wrappers: %v", mode, err)
			}
			var consumer *Artifact
			for _, artifact := range artifacts {
				if artifact.IR.ModulePath == source.ModulePath {
					consumer = artifact
					break
				}
			}
			if consumer == nil {
				t.Fatalf("%s did not produce the streaming consumer", mode)
			}
			methods := map[string]*ir.Method{}
			for _, statement := range consumer.IR.Statements {
				if method, ok := statement.(*ir.Method); ok {
					methods[method.Name] = method
				}
			}
			for _, name := range []string{"caught", "propagated"} {
				method := methods[name]
				if method == nil || len(method.Body) < 3 {
					t.Fatalf("%s IR is missing the structured %s() body: %#v", mode, name, method)
				}
				if _, ok := method.Body[0].(*ir.Temporary); !ok {
					t.Fatalf("%s %s() first statement is %T, want *ir.Temporary", mode, name, method.Body[0])
				}
				iteration, ok := method.Body[1].(*ir.Iterate)
				if !ok {
					t.Fatalf("%s %s() second statement is %T, want *ir.Iterate", mode, name, method.Body[1])
				}
				if iteration.Result == nil || iteration.Result.Target == nil || iteration.Result.Type.String() != "Result<Integer, DbError>" || !iteration.CaptureEffect || !iteration.ResultBoundary || iteration.Fails.String() != "DbError" || iteration.EffectSuccess.String() != "Integer" {
					t.Fatalf("%s %s() structured Result iteration=%#v", mode, name, iteration)
				}
				if _, ok := method.Body[2].(*ir.Variable); !ok {
					t.Fatalf("%s %s() third statement is %T, want authored *ir.Variable", mode, name, method.Body[2])
				}
			}
			raw := methods["raw"]
			if raw == nil || len(raw.Body) == 0 {
				t.Fatalf("%s IR is missing raw()", mode)
			}
			iteration, ok := raw.Body[0].(*ir.Iterate)
			if !ok || iteration.Result == nil || !iteration.Result.Return || iteration.Result.Type.String() != "DbResult<Integer>" || !iteration.CaptureEffect || !iteration.ResultBoundary {
				t.Fatalf("%s raw streaming result=%#v", mode, iteration)
			}
		})
	}
}

func TestORMResultStreamingRejectsUnsupportedTransfersAndPlacement(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "authored return",
			source: `import { DbResult, Model } from trb/orm
class Product < Model
end
def invalid(): DbResult<Integer>
	return Product.find_each() do |_product|
		return DbResult<Integer>::Ok(0)
	end
end
`,
			want: "return is not supported inside Result-boundary structured blocks",
		},
		{
			name: "nested try",
			source: `import { DbResult, Model } from trb/orm
class Product < Model
end
def invalid(): DbResult<Integer>
	value := if true
		try Product.find_each() do |_product|
		end
	else
		0
	end
	return DbResult<Integer>::Ok(value)
end
`,
			want: "structured block find_each() must be the direct value",
		},
		{
			name: "nested catch",
			source: `import { Model } from trb/orm
class Product < Model
end
def invalid(): Integer
	value := if true
		Product.find_each() do |_product|
		end catch |_error|
			0
		end
	else
		0
	end
	return value
end
`,
			want: "structured block find_each() must be the direct value",
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				root := t.TempDir()
				prepareORMResultStreamingCompilerDatabase(t, root)
				_, err := CompileProject([]SourceUnit{{
					Filename: filepath.Join(root, "src", "main.trb"), ModulePath: "main", Package: "main", Source: []byte(test.source),
				}}, Options{
					Mode: mode, GoModule: "example.com/result-streaming-diagnostic", RubyLoader: "require_relative", TypeScriptRuntime: "bun",
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

func prepareORMResultStreamingCompilerDatabase(t *testing.T, root string) {
	t.Helper()
	database, err := sql.Open("sqlite", filepath.Join(root, "application.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
}
