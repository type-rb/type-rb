package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunCatchWithTransitiveResultAliasAcrossBackends(t *testing.T) {
	files := map[string]string{
		"domain/insurer_member.trb": `record InsurerMemberEntity
	id: Integer
end
`,
		"domain/insurer_member_error.trb": `enum InsurerMemberError
	NotFound
end
`,
		"domain/insurer_member_result.trb": `import { InsurerMemberEntity } from domain/insurer_member
import { InsurerMemberError } from domain/insurer_member_error
import { Result } from trb/std/result

type InsurerMemberResult = Result<InsurerMemberEntity, InsurerMemberError>
`,
		"application/load_member.trb": `import { InsurerMemberEntity } from domain/insurer_member
import { InsurerMemberError } from domain/insurer_member_error
import { InsurerMemberResult } from domain/insurer_member_result

def load_member(found: Boolean): InsurerMemberResult
	if found
		return InsurerMemberResult::Ok(InsurerMemberEntity.new(id: 7))
	end
	return InsurerMemberResult::Err(InsurerMemberError::NotFound)
end
`,
		"main.trb": `import { load_member } from application/load_member

def member_id(found: Boolean): Integer
	member := load_member(found) catch |_error|
		return 41
	end
	return member.id
end

def main()
	puts(member_id(true))
	puts(member_id(false))
	return
end
`,
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			required := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
			if _, err := exec.LookPath(required); err != nil {
				t.Skipf("%s is unavailable: %v", required, err)
			}
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/transitive-result-alias-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			for name, source := range files {
				filename := filepath.Join(config.SourcePath(), filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("%s status=%d stdout=%s stderr=%s", mode, status, stdout.String(), stderr.String())
			}
			if got, want := stdout.String(), "7\n41\n"; got != want || stderr.Len() != 0 {
				t.Fatalf("%s output=%q, want %q; stderr=%q", mode, got, want, stderr.String())
			}
		})
	}
}
