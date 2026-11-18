// The helper replaces itself with the requested executable. execv does not
// interpret ENOEXEC as a request to run /bin/sh, unlike execvp on some hosts.
const fs = require("node:fs");
try {
  const payload = JSON.parse(fs.readFileSync(0, "utf8"));
  for (const name of Object.keys(process.env)) delete process.env[name];
  Object.assign(process.env, payload.environment);
  const koffi = require(payload.adapter);
  const libc = koffi.load(null);
  const execv = libc.func("int execv(const char *path, const char **argv)");
  const fcntl = libc.func("int fcntl(int fd, int command, ...)");
  const signal = libc.func("void *signal(int signum, void *handler)");
  // Node marks standard descriptors close-on-exec at startup. Preserve only
  // stdin/stdout/stderr and close the launch-error channel on a successful exec.
  for (const fd of [0, 1, 2, 3]) {
    const flags = fcntl(fd, 1);
    if (flags === -1 || fcntl(fd, 2, "int", fd === 3 ? flags | 1 : flags & ~1) === -1) {
      throw new Error("process descriptor setup failed: " + koffi.errno());
    }
    if (fd !== 3) {
      // A host runtime can make its stdio nonblocking. The target is an
      // ordinary executable and must not lose output when a pipe fills.
      const status = fcntl(fd, 3);
      if (status === -1 || fcntl(fd, 4, "int", status & ~fs.constants.O_NONBLOCK) === -1) {
        throw new Error("process blocking I/O setup failed: " + koffi.errno());
      }
    }
  }
  signal(13, null); // SIGPIPE / SIG_DFL on supported POSIX hosts.
  const command = payload.command;
  const args = [command, ...payload.arguments, null];
  const candidates = command.includes("/") ? [command] : (process.env.PATH ?? "").split(":").map((directory) => (directory || ".") + "/" + command);
  let failure = koffi.os.errno.ENOENT;
  fs.writeSync(3, "R");
  for (const candidate of candidates) {
    execv(candidate, args);
    const code = koffi.errno();
    if (code === koffi.os.errno.EACCES) failure = code;
    else if (code !== koffi.os.errno.ENOENT && code !== koffi.os.errno.ENOTDIR) {
      throw new Error("cannot execute command: errno " + code);
    }
  }
  throw new Error("cannot execute command: errno " + failure);
} catch (error) {
  fs.writeSync(3, "\nE" + String(error instanceof Error ? error.message : error).slice(0, 500));
  process.exit(127);
}
