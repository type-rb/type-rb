package typescript

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/type-rb/type-rb/internal/testsupport/idnacorpus"
)

func TestOidcUnicode17Conformance(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm unavailable")
	}
	root := t.TempDir()
	module := `{"private":true,"type":"module","dependencies":{"tr46":"6.0.0","punycode":"2.3.1","@types/tr46":"5.0.1","typescript":"^6.0.0"}}`
	source := tsOidcHostnameRuntime + `
import { createInterface } from "node:readline";
const lines = createInterface({ input: process.stdin });
for await (const line of lines) {
 try {
  const output = trb_oidc_ascii(JSON.parse(line));
  console.log(JSON.stringify(output === null ? {error:"invalid"} : {output}));
 } catch(error) { console.log(JSON.stringify({error:String(error)})); }
}
`
	for name, data := range map[string]string{"package.json": module, "probe.ts": source, "typecheck.ts": tsOidcHostnameRuntime + tsOidcBearerRuntime} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	install := exec.Command("npm", "install", "--ignore-scripts", "--no-audit", "--no-fund")
	install.Dir = root
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("install: %v\n%s", err, output)
	}
	check := exec.Command("node", filepath.Join(root, "node_modules/typescript/bin/tsc"), "--noEmit", "--strict", "--target", "ES2022", "--module", "NodeNext", "--moduleResolution", "NodeNext", "typecheck.ts")
	check.Dir = root
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("OIDC support types: %v\n%s", err, output)
	}
	for _, runtime := range []string{"node", "bun"} {
		t.Run(runtime, func(t *testing.T) {
			if _, err := exec.LookPath(runtime); err != nil {
				t.Skipf("%s unavailable", runtime)
			}
			idnacorpus.Check(t, root, runtime, filepath.Join(root, "probe.ts"))
		})
	}
}
