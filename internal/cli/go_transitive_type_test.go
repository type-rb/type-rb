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

func TestRunGoFormatsTransitivelyImportedNullableDate(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	config := project.New(t.TempDir(), "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/calendar"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{
		"models/event.trb": `import { Date } from trb/std/time

record Event
	date: Date?
end
`,
		"render/event.trb": `import { Event } from models/event

def label(event: Event): String
	return event.date.to_s()
end

def optional_label(event: Event): String?
	return event.date&.to_s()
end
`,
		"main.trb": `import { Date } from trb/std/time
import { Event } from models/event
import { label, optional_label } from render/event

def main()
	empty := Event.new(date: nil)
	dated := Event.new(date: Date.new(2030, 4, 5))
	puts(label(dated))
	puts(optional_label(empty) == nil)
	puts(optional_label(dated))
end
`,
	}
	for name, source := range sources {
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
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "2030-04-05\ntrue\n2030-04-05\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
