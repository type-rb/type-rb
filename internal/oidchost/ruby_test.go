package oidchost

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/type-rb/type-rb/internal/testsupport/idnacorpus"
)

func TestRubyUnicode17Conformance(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby unavailable")
	}
	root := t.TempDir()
	filename := filepath.Join(root, "probe.rb")
	source := Ruby() + `
require "json"
STDIN.each_line do |line|
 begin
  output = TrbOidcIdna.to_ascii(JSON.parse(line))
  puts JSON.generate({output: output})
 rescue StandardError => error
  puts JSON.generate({error: error.message})
 end
end
`
	if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	idnacorpus.Check(t, root, "ruby", filename)
}
