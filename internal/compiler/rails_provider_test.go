package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

const railsInsurersController = `activate trb/platform/ruby/rails

module Api
  module V1
    module Internal
      class InsurersController < ActionController::API
        def index()
          insurers := Insurer.all()
          render(json: insurers)
          return
        end

        def show()
          insurer := Insurer.find_by!(code: params[:code])
          render(json: insurer.as_json())
          return
        end
      end
    end
  end
end
`

func TestRailsProviderTypesControllerWithoutApplicationSignatures(t *testing.T) {
	root := railsProject(t)
	artifact, err := CompileWithOptions("insurers_controller.trb", []byte(railsInsurersController), Options{Mode: "ruby", ProjectRoot: root, RubyLoader: "zeitwerk"})
	if err != nil {
		t.Fatal(err)
	}
	variables := map[string]string{}
	collectIRVariables(artifact.IR.Statements, variables)
	for name, want := range map[string]string{
		"insurers": "ActiveRecord::Relation<Insurer>",
		"insurer":  "Insurer",
	} {
		if got := variables[name]; got != want {
			t.Fatalf("%s type = %q, want %q; all=%v", name, got, want, variables)
		}
	}
}

func TestRailsProviderChecksSchemaDerivedFinderArguments(t *testing.T) {
	root := railsProject(t)
	wrongType := strings.Replace(railsInsurersController, "code: params[:code]", "code: 123", 1)
	if _, err := CompileWithOptions("insurers_controller.trb", []byte(wrongType), Options{Mode: "ruby", ProjectRoot: root}); err == nil || !strings.Contains(err.Error(), "expected String") {
		t.Fatalf("expected schema-derived column diagnostic, got %v", err)
	}
	unknownColumn := strings.Replace(railsInsurersController, "code: params[:code]", "missing: params[:code]", 1)
	if _, err := CompileWithOptions("insurers_controller.trb", []byte(unknownColumn), Options{Mode: "ruby", ProjectRoot: root}); err == nil || !strings.Contains(err.Error(), "has no named argument missing") {
		t.Fatalf("expected unknown finder column diagnostic, got %v", err)
	}
	unknownMember := strings.Replace(railsInsurersController, "insurer.as_json()", "insurer.not_a_method()", 1)
	if _, err := CompileWithOptions("insurers_controller.trb", []byte(unknownMember), Options{Mode: "ruby", ProjectRoot: root}); err == nil || !strings.Contains(err.Error(), "externally provided type Insurer has no member not_a_method") {
		t.Fatalf("expected unknown ActiveRecord member diagnostic, got %v", err)
	}
}

func railsProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	schema := `ActiveRecord::Schema[8.1].define do
  create_table "insurers", force: :cascade do |t|
    t.string "code", null: false
    t.string "name", null: false
    t.datetime "created_at", null: false
  end
end
`
	if err := os.WriteFile(filepath.Join(root, "db", "schema.rb"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func collectIRVariables(statements []ir.Statement, result map[string]string) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Variable:
			result[node.Name] = node.Type.String()
		case *ir.Module:
			collectIRVariables(node.Body, result)
		case *ir.Class:
			collectIRVariables(node.Body, result)
		case *ir.Method:
			collectIRVariables(node.Body, result)
		}
	}
}
