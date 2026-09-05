package repl

import (
	"runtime"
	"strconv"
	"testing"

	"github.com/type-rb/type-rb/internal/pathvalue/pathfixture"
)

func TestEvaluatePathValuesAcrossModes(t *testing.T) {
	const helpers = `import { Path, RelativePath } from trb/std/path

def inspect_path(source: String): String
	path := RelativePath.parse(source) catch |error|
		return error.to_s()
	end
	leaf := path.child("leaf.md") catch |error|
		return error.to_s()
	end
	parent := leaf.parent()
	if parent != nil
		return leaf.to_s() + ":" + (parent == path).to_s()
	end
	return "unexpected root"
end

def host_join(parent: String): String
	child := RelativePath.parse("a/b") catch |_error|
		return "invalid fixture"
	end
	return Path.new(parent).join(child).to_s()
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			for _, fixture := range []struct{ expression, result string }{
				{`inspect_path("日本語/docs")`, `"日本語/docs/leaf.md:true"`},
				{`inspect_path("../x")`, `"dot and parent components are not allowed"`},
				{`inspect_path("con.txt")`, `"con.txt/leaf.md:true"`},
				{`inspect_path("x/")`, `"relative path components must not be empty"`},
				{`Path.new("a/../").to_s()`, `"a/../"`},
			} {
				if got := evaluateDirBoundarySource(t, mode, helpers+fixture.expression+"\n"); got != fixture.result {
					t.Errorf("%s = %s, want %s", fixture.expression, got, fixture.result)
				}
			}
			for _, fixture := range pathfixture.Cases {
				if fixture.Child != "a/b" {
					continue
				}
				source := helpers + "host_join(" + strconv.Quote(fixture.Parent) + ")\n"
				want := strconv.Quote(fixture.Expected(runtime.GOOS == "windows"))
				if got := evaluateDirBoundarySource(t, mode, source); got != want {
					t.Errorf("parent %q: %s, want %s", fixture.Parent, got, want)
				}
			}
		})
	}
}
