#!/usr/bin/env python3
"""Check portable Process results and ownership across Native execution paths."""

import argparse
from contextlib import ExitStack
import errno
import json
import os
import re
import select
import signal
from pathlib import Path
import subprocess
import sys
import tempfile
import time


def checked(command, cwd, env, source=None, expected=None):
    result = subprocess.run(command, input=source, text=True, capture_output=True, cwd=cwd, env=env, timeout=45)
    if result.returncode != 0 or (expected is not None and (result.stdout != expected or result.stderr)):
        raise AssertionError((command, result.returncode, result.stdout, result.stderr, expected))
    return result


def verify(binary):
    with tempfile.TemporaryDirectory(prefix='native-process-cases-') as temporary, ExitStack() as cleanup:
        root = Path(temporary).resolve()
        plain = root / 'plain script'
        plain.write_text('printf explicit\n')
        plain.chmod(0o700)
        script = root / 'literal script'
        script.write_text('#!/bin/sh\nprintf "<%s>" "$@"\nprintf captured >&2\nexit 127\n')
        script.chmod(0o700)
        first = root / 'first-bin'
        second = root / 'second-bin'
        first.mkdir()
        second.mkdir()
        broken = first / 'process-path-probe'
        broken.write_text(f'#!{root / "missing-interpreter"}\n')
        broken.chmod(0o700)
        fallback = second / broken.name
        fallback.write_text('#!/bin/sh\nprintf fallback\n')
        fallback.chmod(0o700)
        directory = first / 'process-directory-probe'
        directory.mkdir()
        cleanup.callback(directory.chmod, 0o700)
        directory.chmod(0o111)
        executable = second / directory.name
        executable.write_text('#!/bin/sh\nprintf selected\n')
        executable.chmod(0o700)
        alias = root / 'cwd alias'
        alias.symlink_to(root, target_is_directory=True)
        env = dict(os.environ, NO_COLOR='1', TERM='dumb', PWD=str(alias),
                   PATH=os.pathsep.join([str(first), str(second), os.environ['PATH']]),
                   TRBN_HISTORY=str(root / 'history'), TRB_PROCESS_PROBE_EMPTY='',
                   TRB_PROCESS_PROBE_PRESENT='value 日本語')
        env.pop('TRB_PROCESS_PROBE_ABSENT', None)
        source = r'''import trb/std/process
import trb/std/result

def report(command: String, args: Array<String>)
	case Process.run(command, args)
	when Result::Ok(value)
		puts(value.status)
		puts(value.stdout)
		puts(value.stderr)
		puts(value.success)
	when Result::Err(error)
		puts(error.operation)
		puts(error.command == command)
		puts(!error.message.empty?())
	end
end

def main()
	mut copy := Process.argv()
	copy.push("appended")
	puts(copy.size() == Process.argv().size() + 1)
	present := Process.environment("TRB_PROCESS_PROBE_PRESENT")
	if present != nil
		puts(present)
	end
	empty := Process.environment("TRB_PROCESS_PROBE_EMPTY")
	if empty != nil
		puts(empty.empty?())
	end
	puts(Process.environment("TRB_PROCESS_PROBE_ABSENT") == nil)
	case Process.working_directory()
	when Result::Ok(path)
		puts(path)
	when Result::Err(error)
		puts(error.message)
	end
	report(PLAIN_PATH, [])
	report("/bin/sh", [PLAIN_PATH])
	report(LITERAL_PATH, ["two words", "$(printf expanded)"])
	report("command-that-does-not-exist", [])
	report("process-path-probe", [])
	report("process-directory-probe", [])
	report("/bin/sh\u0000ignored", [])
	report("/bin/sh", ["-c", "printf ignored\u0000"])
	report("sh", ["-c", "printf out; printf err >&2; kill -TERM $$"])
	report("/bin/cat", [])
	report("/bin/sh", ["-c", "printf '\\357\\273\\277\\377\\377'; printf '\\342'; printf '\\202\\254'; printf '\\355\\240\\200' >&2"])
	case Process.run("/bin/sh", ["-c", "i=0; while [ $i -lt 5000 ]; do printf 0123456789abcdef; printf 0123456789abcdef >&2; i=$((i+1)); done"])
	when Result::Ok(value)
		puts(value.status)
		puts(value.stdout.size())
		puts(value.stderr.size())
	when Result::Err(error)
		puts(error.message)
	end
end
'''.replace('PLAIN_PATH', json.dumps(str(plain), ensure_ascii=False)).replace('LITERAL_PATH', json.dumps(str(script), ensure_ascii=False))
        launch_error = 'run\ntrue\ntrue\n'
        want = (
            f'true\nvalue 日本語\ntrue\ntrue\n{alias}\n'
            + launch_error
            + '0\nexplicit\n\ntrue\n'
            + '127\n<two words><$(printf expanded)>\ncaptured\nfalse\n'
            + launch_error * 2
            + '0\nselected\n\ntrue\n'
            + launch_error * 2
            + '-1\nout\nerr\nfalse\n'
            + '0\n\n\ntrue\n'
            + '0\n\ufeff��€\n���\ntrue\n'
            + '0\n80000\n80000\n'
        )
        entry = root / 'main.trb'
        entry.write_text(source)
        program = root / 'result'
        checked([binary, 'check', str(entry)], root, env)
        checked([binary, 'build', str(entry), '--compile', '--outfile', str(program)], root, env)
        checked([str(program)], root, env, expected=want)
        print('ordinary check, build and execution passed', flush=True)
        entry.unlink()
        repl_source = source.replace('def main()', 'def run_process_probe()', 1)
        checked([binary, 'repl'], root, env, source=repl_source + '\nrun_process_probe()\n:quit\n', expected=want)
        print('retained REPL output passed', flush=True)


def send(child, source):
    child.stdin.write(source.encode())
    child.stdin.flush()


def observe(child, marker, output):
    deadline = time.monotonic() + 10
    while marker not in output:
        remaining = deadline - time.monotonic()
        if remaining <= 0 or child.poll() is not None:
            raise AssertionError((marker, bytes(output), child.poll()))
        ready, _, _ = select.select([child.stdout], [], [], remaining)
        if not ready:
            raise AssertionError((marker, bytes(output)))
        data = os.read(child.stdout.fileno(), 65536)
        if not data:
            raise AssertionError((marker, bytes(output)))
        output.extend(data)


def kill_group(child):
    # The new session contains only the REPL and its synthetic fixture child.
    try:
        os.killpg(child.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    child.communicate(timeout=10)


def descriptor_numbers(pid):
    if sys.platform.startswith('linux'):
        return sorted(int(path.name) for path in Path(f'/proc/{pid}/fd').iterdir())
    output = subprocess.check_output(
        ['/usr/sbin/lsof', '-a', '-p', str(pid), '-d', '0-65535', '-Ff'],
        text=True, timeout=10,
    )
    return sorted(int(fd) for fd in re.findall(r'^f(\d+)$', output, re.MULTILINE))


def verify_resources(binary, root):
    cwd = root / 'working'
    cwd.mkdir()
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', PWD=str(cwd),
               TRBN_HISTORY=str(root / 'history'))
    with subprocess.Popen([binary, "repl"], cwd=cwd, env=env,
                          stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, start_new_session=True,
                          bufsize=0) as child:
        output = bytearray()
        try:
            send(child, '''import trb/std/process as ProbeProcess
import trb/std/result
def descriptor_probe()
	mut count := 0
	while count < 12
		case ProbeProcess.run("/bin/sh", ["-c", "exit 13"])
		when Result::Ok(value)
			puts(value.status)
		when Result::Err(error)
			puts(error.operation)
		end
		case ProbeProcess.run("./missing-process-fixture", [])
		when Result::Ok(value)
			puts(value.status)
		when Result::Err(error)
			puts(error.operation)
		end
		count += 1
	end
end
held := 321
puts("session ready")
''')
            observe(child, b'session ready\n', output)
            descriptors = descriptor_numbers(child.pid)
            assert {0, 1, 2}.issubset(descriptors), descriptors
            send(child, 'descriptor_probe()\nputs("process calls complete")\n')
            observe(child, b'13\nrun\n' * 12 + b'process calls complete\n', output)
            after_calls = descriptor_numbers(child.pid)
            assert after_calls == descriptors, (descriptors, after_calls)
            interrupt(child, cwd, output)
            after_interrupt = descriptor_numbers(child.pid)
            assert after_interrupt == descriptors, (descriptors, after_interrupt)
            (cwd / 'owned-child').unlink()
            cwd.rmdir()
            send(child, '''case ProbeProcess.working_directory()
when Result::Ok(path)
puts(path)
when Result::Err(error)
puts(error.operation)
puts(error.command.empty?())
puts(!error.message.empty?())
end
puts("directory check complete")
''')
            observe(child, b'working_directory\ntrue\ntrue\ndirectory check complete\n', output)
            send(child, ':quit\n')
            tail, error = child.communicate(timeout=10)
            assert child.returncode == 0, (child.returncode, tail, error)
        except BaseException:
            kill_group(child)
            raise
    assert b'Interrupted' in error, (output, tail, error)
    print('REPL closes owned descriptors, joins the interrupted child and retains state')


def interrupt(child, cwd, output):
    ready = cwd / 'owned-child'
    send(child, 'case ProbeProcess.run("/bin/sh", '
                '["-c", "printf %s $$ > owned-child; exec /bin/sleep 60"])\n'
                'when Result::Ok(value)\nputs(value.status)\n'
                'when Result::Err(error)\nputs(error.operation)\nend\n')
    deadline = time.monotonic() + 10
    pid = 0
    while not pid:
        if time.monotonic() > deadline or child.poll() is not None:
            raise AssertionError('fixture did not publish child identity')
        try:
            raw = ready.read_text()
            if raw:
                pid = int(raw)
        except FileNotFoundError:
            pass
        if not pid:
            time.sleep(0.01)
    # Send SIGINT to the interpreter only. The owned child must be joined
    # before the next statement executes, with the interpreter still alive.
    assert os.getpgid(pid) == child.pid
    child.send_signal(signal.SIGINT)
    send(child, 'puts(held)\nputs("session resumed")\n')
    observe(child, b'321\nsession resumed\n', output)
    assert child.poll() is None
    try:
        os.kill(pid, 0)
    except OSError as exc:
        assert exc.errno == errno.ESRCH, exc
    else:
        raise AssertionError(f'owned child {pid} still exists after resumption')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    binary = str(parser.parse_args().binary.resolve(strict=True))
    verify(binary)
    with tempfile.TemporaryDirectory(prefix='native-process-interrupt-') as temp:
        verify_resources(binary, Path(temp))
