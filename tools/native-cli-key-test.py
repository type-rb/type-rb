#!/usr/bin/env python3
"""Deterministic syscall-boundary checks of the actual CLI QBE host bridge."""
import argparse
import fcntl
import termios
import hashlib
import platform
import json
import os
from pathlib import Path
import re
import select
import subprocess
import tempfile
import time

parser = argparse.ArgumentParser()
parser.add_argument('--qbe', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
parser.add_argument('--source', type=Path, help='Alternate recorded host source for a regression control')
args = parser.parse_args()
repo = Path(__file__).resolve().parent.parent
source = (args.source or repo / 'compiler/cli/host.trb').read_text()
body = source.split('def native_cli_runtime_qbe(): String\n', 1)[1].split('\nend', 1)[0].strip()
bridge = json.loads(body.removeprefix('return '))
names = ('native_cli_key', 'native_cli_terminal_start', 'native_cli_terminal_end',
         'native_cli_signals', 'trbn_signal', 'trbn_key_restore')
functions = []
for name in names:
    matches = re.findall(r'^function(?: l)? \$' + name + r'\([^}]+\}', bridge, re.M)
    if not matches and name == 'trbn_key_restore' and args.source:
        continue  # The recorded pre-repair bridge has no restoration helper.
    assert len(matches) == 1, name
    functions.append('export ' + matches[0])
qbe = '\n'.join('export ' + line for line in bridge.splitlines() if line.startswith('data '))
qbe += '\n' + '\n'.join(functions) + '\n'
# Only the syscall boundary is intercepted; the repository's key-reader branches
# and terminal/signal implementation remain the code under test.
qbe = qbe.replace('call $poll(', 'call $test_poll(').replace('call $read(', 'call $test_read(').replace('call $fcntl(', 'call $test_fcntl(')
helper = r'''
#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <signal.h>
#include <stdint.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <termios.h>
#include <unistd.h>
extern int64_t native_cli_key(int64_t);
extern int64_t native_cli_signals(void);
extern int64_t native_cli_terminal_start(void);
extern int64_t native_cli_terminal_end(void);
extern volatile int64_t trbn_interrupted;
static const char *scenario;
static int restore_attempts;
int test_fcntl(int fd, int command, ...) {
    va_list ap; va_start(ap,command); int flags=va_arg(ap,int); va_end(ap);
    if (!strcmp(scenario,"get-flags-error") && command==F_GETFL) { errno=EBADF; return -1; }
    if (!strcmp(scenario,"set-flags-error") && command==F_SETFL && (flags&O_NONBLOCK)) { errno=EPERM; return -1; }
    if (!strcmp(scenario,"restore-interrupted") && command==F_SETFL && !(flags&O_NONBLOCK) && restore_attempts++==0) { errno=EINTR; return -1; }
    return fcntl(fd,command,flags);
}
int test_poll(struct pollfd *fds, nfds_t count, int timeout) {
    if (!strcmp(scenario, "poll-signal")) raise(SIGINT);
    if (!strcmp(scenario, "poll-error")) { errno = EBADF; return -1; }
    return poll(fds, count, timeout);
}
ssize_t test_read(int fd, void *buffer, size_t count) {
    if (!strcmp(scenario, "read-signal") || !strcmp(scenario, "read-drain-signal")) {
        if (!strcmp(scenario, "read-drain-signal")) tcflush(fd, TCIFLUSH);
        raise(SIGINT);
    }
    if (!strcmp(scenario, "read-drain")) tcflush(fd, TCIFLUSH);
    fprintf(stderr, "read-boundary pending=%lld flags=%d\n", (long long)trbn_interrupted, fcntl(fd,F_GETFL));
    if (!strcmp(scenario, "read-terminate")) raise(SIGTERM);
    if (!strcmp(scenario, "read-eof")) return 0;
    if (!strcmp(scenario, "read-error")) { errno = EIO; return -1; }
    return read(fd, buffer, count);
}
int main(int argc, char **argv) {
    if (argc != 2) return 64;
    scenario = argv[1];
    if (native_cli_signals() || native_cli_terminal_start()) return 65;
    if (!strcmp(scenario, "nonblocking")) {
        if (fcntl(0,F_SETFL,fcntl(0,F_GETFL)|O_NONBLOCK) < 0) return 66;
    }
    if (write(1,"ready\n",6) != 6) return 67;
    int original = fcntl(0,F_GETFL);
    if (!strcmp(scenario,"pending")) raise(SIGINT);
    if (!strcmp(scenario,"invalid")) close(0);
    int64_t result = native_cli_key(100);
    int after = fcntl(0,F_GETFL);
    native_cli_terminal_end();
    fprintf(stderr, "result=%lld original=%d after=%d pending=%lld\n", (long long)result, original, after, (long long)trbn_interrupted);
    return 0;
}
'''
expected = {'byte': 120, 'control-c': 3, 'idle': -2, 'pending': -3, 'poll-signal': -3,
            'read-signal': -3, 'read-drain-signal': -3, 'read-drain': -2,
            'poll-error': -1, 'read-error': -1, 'read-eof': -1, 'invalid': -1,
            'nonblocking': 120, 'get-flags-error': -1, 'set-flags-error': -1,
            'restore-interrupted': 120, 'read-terminate': None}
records = []
args.output.parent.mkdir(parents=True, exist_ok=True)
with tempfile.TemporaryDirectory(prefix='native-key-boundary-') as directory:
    root = Path(directory)
    (root/'host.ssa').write_text(qbe)
    (root/'helper.c').write_text(helper)
    subprocess.run([str(args.qbe.resolve()), '-o', str(root/'host.s'), str(root/'host.ssa')], check=True)
    subprocess.run(['cc', '-o', str(root/'probe'), str(root/'host.s'), str(root/'helper.c')], check=True)
    for scenario, want in expected.items():
        master, slave = os.openpty()
        original_terminal = termios.tcgetattr(slave)
        original_flags = fcntl.fcntl(slave, fcntl.F_GETFL)
        start = time.monotonic()
        process = subprocess.Popen([str(root/'probe'), scenario], stdin=slave,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        try:
            try:
                assert select.select([process.stdout], [], [], 1)[0], 'terminal setup did not complete'
                assert os.read(process.stdout.fileno(), 6) == b'ready\n', 'terminal setup failed'
                if scenario not in ('idle', 'pending', 'poll-signal', 'invalid'):
                    os.write(master, b'\x03' if scenario == 'control-c' else b'x')
                stdout, stderr = process.communicate(timeout=1)
                run = subprocess.CompletedProcess(process.args, process.returncode, stdout, stderr)
                transcript = run.stderr.decode(errors='replace')
                match = re.search(r'result=(-?\d+) original=(-?\d+) after=(-?\d+) pending=(-?\d+)', transcript)
                passed = (run.returncode == 0 and match is not None and int(match[1]) == want
                          and (scenario == 'invalid' or match[2] == match[3]))
                if scenario == 'read-terminate':
                    passed = run.returncode == 143
                flags = fcntl.fcntl(slave, fcntl.F_GETFL)
                expected_flags = original_flags | (os.O_NONBLOCK if scenario == 'nonblocking' else 0)
                terminal_after = termios.tcgetattr(slave)
                # Darwin marks input for reprocessing when canonical mode returns.
                # PENDIN is kernel queue state; compare every configured mode and
                # control character, retaining both observed flag words below.
                before_modes, after_modes = list(original_terminal), list(terminal_after)
                before_modes[3] &= ~getattr(termios, 'PENDIN', 0)
                after_modes[3] &= ~getattr(termios, 'PENDIN', 0)
                terminal_restored = after_modes == before_modes
                # Closing fd 0 deliberately prevents the bridge from restoring
                # that terminal; the parent retains and closes the isolated PTY.
                passed = passed and flags == expected_flags and (scenario == 'invalid' or terminal_restored)
                record = dict(scenario=scenario, expected=want, passed=passed,
                              flags=flags, expectedFlags=expected_flags,
                              terminalRestored=terminal_restored,
                              localFlagsBefore=original_terminal[3], localFlagsAfter=terminal_after[3],
                              status=run.returncode, transcript=transcript)
            except subprocess.TimeoutExpired:
                child_state = {'running': process.poll() is None}
                try:
                    state = subprocess.run(['ps', '-o', 'pid=,stat=,wchan=', '-p', str(process.pid)],
                                           text=True, capture_output=True, timeout=1)
                    child_state.update(psStatus=state.returncode, ps=state.stdout, psStderr=state.stderr)
                except (OSError, subprocess.TimeoutExpired) as error:
                    child_state['psUnavailable'] = type(error).__name__
                process.kill()
                _, stderr = process.communicate()
                record = dict(scenario=scenario, expected=want, passed=False,
                              status='timeout-killed-and-reaped',
                              transcript=stderr.decode(errors='replace'), childStateBeforeKill=child_state)
            except (AssertionError, OSError) as error:
                if process.poll() is None:
                    process.kill()
                _, stderr = process.communicate()
                record = dict(scenario=scenario, expected=want, passed=False,
                              status=process.returncode, setupError=str(error),
                              transcript=stderr.decode(errors='replace'))
            record['elapsedSeconds'] = time.monotonic()-start
            records.append(record)
            print(('PASS' if record['passed'] else 'FAIL'), scenario, record['status'], flush=True)
        finally:
            if process.poll() is None:
                process.kill()
            process.communicate()
            os.close(slave)
            os.close(master)
args.output.write_text(json.dumps(dict(schemaVersion=1, platform=platform.platform(),
    sourceSHA256=hashlib.sha256(source.encode()).hexdigest(),
    qbeSHA256=hashlib.sha256(args.qbe.read_bytes()).hexdigest(),
    observations=records), indent=2)+'\n')
assert all(row['passed'] for row in records), str(args.output)
