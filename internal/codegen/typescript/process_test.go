//go:build darwin || linux

package typescript

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestProcessAdapterCleanup(t *testing.T) {
	if os.Getenv("TRB_PROCESS_DESCRIPTOR_PROBE") == "1" {
		for fd := uintptr(0); fd < 3; fd++ {
			flags, _, err := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
			if err != 0 || flags&syscall.O_NONBLOCK != 0 {
				os.Exit(20 + int(fd))
			}
		}
		os.Stdout.WriteString("blocking\n")
		os.Exit(0)
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm unavailable")
	}
	root := t.TempDir()
	install := exec.Command("npm", "install", "--prefix", root, "--ignore-scripts", "--no-audit", "--no-fund", "koffi@3.3.7", "typescript@7.0.2")
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("install: %v\n%s", err, output)
	}
	support := "const __trbProcessHelper = " + strconv.Quote(tsProcessHelper) + ";\n" + tsProcessRuntime
	// Simulate a runtime making its standard descriptors nonblocking before
	// handing them to the target. The exec boundary must restore blocking I/O.
	nonblocking := strings.Replace(tsProcessHelper, "  // Node marks standard", `  for (const fd of [0, 1, 2]) {
    const flags = fcntl(fd, 3);
    if (flags === -1 || fcntl(fd, 4, "int", flags | fs.constants.O_NONBLOCK) === -1) throw new Error("probe setup failed");
  }
  // Node marks standard`, 1)
	if nonblocking == tsProcessHelper {
		t.Fatal("missing descriptor setup boundary")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	trackedSupport := strings.Replace(support, "  const environment = { ...host.env };", `  __trbOwnedPipes.push(...descriptors.map((fd) => {
    const {dev, ino} = fs.fstatSync(fd, {bigint:true});
    return {fd, dev, ino};
  }));
  const environment = { ...host.env };`, 1)
	if trackedSupport == support {
		t.Fatal("missing owned pipe boundary")
	}
	probe := strings.Replace(trackedSupport, "const __trbProcessHelper", "let __trbProcessHelper", 1) + `
const host = Reflect.get(globalThis, "process");
const fs = host.getBuiltinModule("fs");
const __trbOwnedPipes: {fd:number; dev:bigint; ino:bigint}[] = [];
function assertOwnedPipesClosed(expected: number) {
  if (__trbOwnedPipes.length !== expected) throw new Error("owned pipe observation is incomplete");
  for (const owned of __trbOwnedPipes.splice(0)) {
    try {
      const actual = fs.fstatSync(owned.fd, {bigint:true});
      if (actual.dev === owned.dev && actual.ino === owned.ino) throw new Error("owned pipe remains open: " + owned.fd);
    } catch (error) { if (Reflect.get(error as object, "code") !== "EBADF") throw error; }
  }
}
function openStreams() {
  // Bun can initialize event-loop descriptors lazily, and enumeration can
  // include descriptors already closed before fstat. Compare live pipe/socket
  // identities, including the host launcher's stdio, independently of fd reuse.
  return fs.readdirSync("/dev/fd").flatMap((fd: string) => {
    try {
      const value = fs.fstatSync(Number(fd), {bigint:true});
      return value.isFIFO() || value.isSocket() ? [String(value.dev) + ":" + String(value.ino)] : [];
    } catch (error) { if (Reflect.get(error as object, "code") === "EBADF") return []; throw error; }
  }).sort();
}
__trbProcessCapture("/bin/sh", ["-c", "exit 0"]);
assertOwnedPipesClosed(2);
const before = JSON.stringify(openStreams());
for (let index = 0; index < 12; index++) {
  const output = __trbProcessCapture("/bin/sh", ["-c", "printf $$"]);
  const pid = Number(new TextDecoder().decode(output.stdout));
  if (output.status !== 0 || !Number.isInteger(pid) || pid <= 0) throw new Error("missing child identity");
  try { host.kill(pid, 0); throw new Error("owned child still exists"); }
  catch (error) { if (Reflect.get(error as object, "code") !== "ESRCH") throw error; }
  try { __trbProcessCapture("/type-rb-process-does-not-exist", []); throw new Error("missing launch error"); }
  catch (error) { if (!String(error).includes("cannot execute command")) throw error; }
  assertOwnedPipesClosed(4);
}
const after = JSON.stringify(openStreams());
if (after !== before) throw new Error("process streams leaked: " + before + " -> " + after);
__trbProcessHelper = ` + strconv.Quote(nonblocking) + `;
host.env.TRB_PROCESS_DESCRIPTOR_PROBE = "1";
const descriptors = __trbProcessCapture(` + strconv.Quote(executable) + `, ["-test.run=^TestProcessAdapterCleanup$"]);
if (descriptors.status !== 0 || new TextDecoder().decode(descriptors.stdout) !== "blocking\n") throw new Error("target inherited nonblocking I/O: " + descriptors.status);
assertOwnedPipesClosed(2);
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
