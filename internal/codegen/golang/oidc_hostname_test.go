package golang

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/type-rb/type-rb/internal/testsupport/idnacorpus"
)

func TestOidcUnicode17Conformance(t *testing.T) {
	root := t.TempDir()
	module := "module example.com/idna-conformance\n\ngo 1.27\n\nrequire (\n golang.org/x/net v0.62.0\n golang.org/x/text v0.43.0\n)\n"
	source := `package main
import("bufio";"os";"encoding/json";"strings";"fmt";"net/netip";"net/url";nethttp "net/http";"golang.org/x/net/idna")
` + goOidcHostnameRuntime + `
func main() { scanner := bufio.NewScanner(os.Stdin); encoder := json.NewEncoder(os.Stdout)
 for scanner.Scan() { var input string; if err := json.Unmarshal(scanner.Bytes(), &input); err != nil { panic(err) }
  output, err := trbOidcIDNA.ToASCII(input)
  if err != nil { encoder.Encode(map[string]string{"error":err.Error()}) } else { encoder.Encode(map[string]string{"output":output}) }
 }
}
`
	for name, data := range map[string]string{"go.mod": module, "main.go": source} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(root, "probe")
	build := exec.Command("go", "build", "-mod=mod", "-o", binary, ".")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	idnacorpus.Check(t, root, binary)
}
