package typescript

import (
	_ "embed"
	"strconv"
)

//go:embed process_helper.cjs
var tsProcessHelper string

func (g *generator) processRuntimeSupport() {
	g.b.WriteString("const __trbProcessHelper = " + strconv.Quote(tsProcessHelper) + ";\n")
	g.b.WriteString(tsProcessRuntime)
}

const tsProcessRuntime = `
function __trbProcessCapture(command: string, args: string[]): { status: number | null; stdout: Uint8Array; stderr: Uint8Array } {
  const host = Reflect.get(globalThis, "process");
  if (host?.platform !== "linux" && host?.platform !== "darwin") throw new Error("process execution is unavailable on this host");
  if (command.length === 0) throw new Error("command is empty");
  if ([command, ...args].some((value) => value.includes("\0"))) throw new Error("command or argument contains NUL");
  const childProcess = host.getBuiltinModule("child_process");
  const fs = host.getBuiltinModule("fs");
  const require = host.getBuiltinModule("module").createRequire(import.meta.url);
  const adapter = require.resolve("koffi");
  const koffi = require(adapter);
  const libc = koffi.load(null);
  const pipe = libc.func("int pipe(_Out_ int *descriptors)");
  const fcntl = libc.func("int fcntl(int fd, int command, ...)");
  const descriptors = [-1, -1];
  if (pipe(descriptors) !== 0) throw new Error("process error pipe failed: " + koffi.errno());
  const environment = { ...host.env };
  delete environment.NODE_OPTIONS;
  delete environment.BUN_OPTIONS;
  try {
    for (const fd of descriptors) {
      const flags = fcntl(fd, 1);
      if (flags === -1 || fcntl(fd, 2, "int", flags | 1) === -1) throw new Error("process error pipe setup failed: " + koffi.errno());
    }
    // Supply an owned descriptor: Bun's synchronous Node-compatible launcher
    // does not return captured data for additional "pipe" slots.
    const output = childProcess.spawnSync(host.execPath, ["--eval", __trbProcessHelper], {
      env: environment,
      input: JSON.stringify({ command, arguments: args, environment: host.env, adapter }),
      stdio: ["pipe", "pipe", "pipe", descriptors[1]],
      maxBuffer: Infinity,
    });
    fs.closeSync(descriptors[1]);
    descriptors[1] = -1;
    if (output.error !== undefined) throw output.error;
    const launch = new TextDecoder().decode(fs.readFileSync(descriptors[0]));
    if (launch !== "R") throw new Error(launch.includes("\nE") ? launch.split("\nE", 2)[1] : "process launcher failed");
    return output;
  } finally {
    for (const fd of descriptors) if (fd !== -1) fs.closeSync(fd);
  }
}
`
