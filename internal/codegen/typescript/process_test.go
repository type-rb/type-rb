//go:build darwin || linux

package typescript

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestProcessAdapterCleanup(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm unavailable")
	}
	root := t.TempDir()
	install := exec.Command("npm", "install", "--prefix", root, "--ignore-scripts", "--no-audit", "--no-fund", "koffi@3.3.7", "typescript@7.0.2")
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("install: %v\n%s", err, output)
	}
	support := "const __trbProcessHelper = " + strconv.Quote(tsProcessHelper) + ";\n" + tsProcessRuntime
	probe := support + `
const host = Reflect.get(globalThis, "process");
const fs = host.getBuiltinModule("fs");
__trbProcessCapture("/bin/sh", ["-c", "exit 0"]);
const before = fs.readdirSync("/dev/fd").length;
for (let index = 0; index < 12; index++) {
  const output = __trbProcessCapture("/bin/sh", ["-c", "printf $$"]);
  const pid = Number(new TextDecoder().decode(output.stdout));
  if (output.status !== 0 || !Number.isInteger(pid) || pid <= 0) throw new Error("missing child identity");
  try { host.kill(pid, 0); throw new Error("owned child still exists"); }
  catch (error) { if (Reflect.get(error as object, "code") !== "ESRCH") throw error; }
  try { __trbProcessCapture("/type-rb-process-does-not-exist", []); throw new Error("missing launch error"); }
  catch (error) { if (!String(error).includes("cannot execute command")) throw error; }
}
if (fs.readdirSync("/dev/fd").length !== before) throw new Error("process descriptors leaked");
console.log("joined and closed");
`
	for name, source := range map[string]string{"support.ts": support, "probe.mts": probe} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	check := exec.Command("node", filepath.Join(root, "node_modules/typescript/bin/tsc"), "--noEmit", "--strict", "--target", "ES2022", "--module", "ESNext", "support.ts")
	check.Dir = root
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("typecheck: %v\n%s", err, output)
	}
	for _, runtime := range []string{"node", "bun"} {
		t.Run(runtime, func(t *testing.T) {
			if _, err := exec.LookPath(runtime); err != nil {
				t.Skipf("%s unavailable", runtime)
			}
			command := exec.Command(runtime, filepath.Join(root, "probe.mts"))
			command.Dir = root
			if output, err := command.CombinedOutput(); err != nil || string(output) != "joined and closed\n" {
				t.Fatalf("cleanup: %v\n%s", err, output)
			}
		})
	}
}
