#!/usr/bin/env python3
"""Exercise checkout cache invalidation in an isolated copy.

Content-key decisions use `tools/build-native.sh --plan`, which shares the
build's decision. The CLI-only cache path has one real rebuild under concurrent
callers; the build job independently verifies core publication and fixed points.
"""
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import time

repository = Path(__file__).resolve().parent.parent
BUILD_TIMEOUT_SECONDS = 600

def stop_on_signal(signum, _frame):
    # Let the active build's finally block reap its whole process group.
    raise SystemExit(128 + signum)

signal.signal(signal.SIGTERM, stop_on_signal)

with tempfile.TemporaryDirectory(prefix='native bootstrap ') as temporary:
    root = Path(temporary)
    for directory in ['compiler', 'bin', '.trb/bootstrap']:
        shutil.copytree(repository / directory, root / directory)
    (root / 'tools').mkdir()
    for file in ['trbn', 'tools/build-native.sh', 'tools/compiler_sources.py', 'tools/qbe_toolchain.py']:
        shutil.copy2(repository / file, root / file)
    shutil.copytree(repository / 'tools/qbe', root / 'tools/qbe')
    env = {key: value for key, value in os.environ.items()
           if key not in ['TRBN_CC', 'TRBN_QBE', 'TRBN_BOOTSTRAP_SEED', 'TRBN_CRYPTO_LIB_DIR']}

    def stop_owned(child):
        if child.poll() is not None:
            return
        try:
            os.killpg(child.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            child.wait(timeout=5)
        except subprocess.TimeoutExpired:
            try:
                os.killpg(child.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            child.wait()

    def communicate(child, timeout):
        try:
            return child.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            # The launcher waits for a shell builder and compiler descendants.
            # Terminating just the launcher would leave them in the test copy.
            try:
                os.killpg(child.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                stdout, stderr = child.communicate(timeout=5)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(child.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                stdout, stderr = child.communicate()
            print(stdout, stderr, flush=True)
            raise

    def run(*command, timeout=120, check=True, environment=env, directory=root):
        child = subprocess.Popen(command, cwd=directory, env=environment, text=True,
                                 stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                 start_new_session=True)
        try:
            stdout, stderr = communicate(child, timeout)
        except BaseException:
            stop_owned(child)
            raise
        result = subprocess.CompletedProcess(command, child.returncode, stdout, stderr)
        if check:
            if result.returncode != 0:
                print(stdout, stderr, flush=True)
            result.check_returncode()
        return result

    def build(directory=root, label=''):
        start = time.monotonic()
        # This watchdog bounds a complete core/CLI fixed-point rebuild, not a
        # performance acceptance measurement. Retain the elapsed observation.
        result = run('./trbn', '--version', timeout=BUILD_TIMEOUT_SECONDS,
                     directory=directory).stderr
        print(f'Native cache build{(" " + label) if label else ""}: '
              f'{time.monotonic() - start:.2f}s', flush=True)
        return result

    def plan(directory=root):
        # The same content keys and published-state checks as a build, without building.
        return run('tools/build-native.sh', '--plan', directory=directory).stdout.strip()

    def snapshot(directory=root):
        return tuple((directory / file).stat().st_mtime_ns for file in
                     ['bin/trbn', '.trb/bootstrap/core/compiler', '.trb/bootstrap/inputs'])

    def edit(file, text, directory=root):
        path = directory / file
        before = path.stat()
        path.write_text(path.read_text() + text)
        # Content changes must invalidate even when timestamps are preserved.
        os.utime(path, ns=(before.st_atime_ns, before.st_mtime_ns))

    # An old same-name cache must not shadow the immutable release-scoped seed.
    seeds = list((root / '.trb/bootstrap/bootstrap-seed-2026-09-29-module-cycles').glob('type-rb-native-bootstrap-*'))
    assert len(seeds) == 1, seeds
    legacy_seed = root / '.trb/bootstrap' / seeds[0].name
    legacy_seed.write_bytes(b'synthetic stale legacy seed\n')

    build()
    baseline = snapshot()
    invalid_seed = run('./trbn', '--version', check=False,
                       environment={**env, 'TRBN_BOOTSTRAP_SEED': str(legacy_seed)})
    assert invalid_seed.returncode != 0 and 'checksum mismatch' in invalid_seed.stderr
    assert snapshot() == baseline
    assert not (root / '.trb/bootstrap/build.lock').exists()
    assert not list((root / '.trb/bootstrap').glob('build.*'))
    assert build() == '' and snapshot() == baseline
    (root / 'compiler/cli/main.trb').touch()
    (root / 'compiler/cli/ignored_test.trb').write_text('invalid test source\n')
    for name in ('testing/nested/fixture.trb', 'tests/arrays/ignored_test.trb'):
        ignored = root / 'compiler/src' / name
        ignored.parent.mkdir(parents=True, exist_ok=True)
        ignored.write_text('invalid test-only source\n')
    assert build() == '' and snapshot() == baseline
    run('git', 'init', '--quiet')
    for message in ['first', 'documentation-only']:
        run('git', '-c', 'user.name=Cache Test', '-c', 'user.email=cache@example.com',
            '-c', 'commit.gpgsign=false', 'commit', '--quiet', '--allow-empty', '-m', message)
        assert build() == '' and snapshot() == baseline

    def expect_plan(expected, label):
        actual = plan()
        assert actual == expected, f'{label}: expected {expected}, got {actual}'
        assert snapshot() == baseline, label

    def restore(file, text):
        path = root / file
        before = path.stat()
        path.write_text(text)
        os.utime(path, ns=(before.st_atime_ns, before.st_mtime_ns))

    # Each input change selects the rebuild scope that a build would take.
    # Restoring the content returns to the published cache without a rebuild.
    for file in ('bin/crypto/libcrypto.a', 'bin/crypto/crypto-identity.o'):
        path = root / file
        original, before = path.read_bytes(), path.stat()
        path.write_bytes(original + b'changed dependency')
        os.utime(path, ns=(before.st_atime_ns, before.st_mtime_ns))
        expect_plan('cli', f'{file} content edit')
        path.write_bytes(original)
        os.utime(path, ns=(before.st_atime_ns, before.st_mtime_ns))
        expect_plan('cached', f'{file} restored')
    for file, scope in [('compiler/cli/main.trb', 'cli'), ('compiler/src/compiler.trb', 'core'),
                        ('tools/compiler_sources.py', 'cli'), ('tools/qbe_toolchain.py', 'core'),
                        ('tools/qbe/backend.json', 'core'), ('tools/qbe/sqrt.patch', 'core'),
                        ('tools/qbe/arm64-immediates.patch', 'core'), ('tools/qbe/arm64-memory.patch', 'core'),
                        ('tools/qbe/gcm.patch', 'core'), ('tools/qbe/fma.patch', 'core')]:
        original = (root / file).read_text()
        edit(file, '\n# Content-only edit\n')
        expect_plan(scope, f'{file} edit')
        restore(file, original)
        expect_plan('cached', f'{file} restored')
    for directory, scope in [('compiler/cli', 'cli'), ('compiler/src', 'core'), ('compiler/src/unused', 'core')]:
        added = root / directory / 'extra.trb'
        added.parent.mkdir(exist_ok=True)
        added.write_text('# Added input\n')
        expect_plan(scope, f'{directory} addition')
        added.unlink()
        if added.parent.name == 'unused':
            added.parent.rmdir()
        expect_plan('cached', f'{directory} addition removed')
    for file, scope in [('compiler/cli/repl/hash.trb', 'cli'), ('compiler/src/support/literals.trb', 'core')]:
        path = root / file
        moved = root / (file + '.moved')
        path.rename(moved)
        expect_plan(scope, f'{file} removal')
        moved.rename(path)
        expect_plan('cached', f'{file} restored after removal')
    with tempfile.TemporaryDirectory(prefix='native bootstrap relocated ') as temporary_relocated:
        relocated = Path(temporary_relocated)
        shutil.copytree(root, relocated, dirs_exist_ok=True)
        assert plan(relocated) == 'cached'
        assert build(relocated, 'relocated cache hit') == ''
    with tempfile.TemporaryDirectory(prefix='native bootstrap empty ') as temporary_empty:
        empty = Path(temporary_empty)
        shutil.copytree(root / 'compiler', empty / 'compiler')
        (empty / 'tools').mkdir()
        for file in ['trbn', 'tools/build-native.sh', 'tools/compiler_sources.py', 'tools/qbe_toolchain.py']:
            shutil.copy2(repository / file, empty / file)
        shutil.copytree(repository / 'tools/qbe', empty / 'tools/qbe')
        assert plan(empty) == 'core'
        assert not (empty / 'bin').exists() or not any((empty / 'bin').iterdir())

    # Core invalidation is covered by --plan above. The companion CLI build
    # job already rebuilds the core from the pinned seed and checks fixed points.
    assert legacy_seed.read_bytes() == b'synthetic stale legacy seed\n'
    assert plan() == 'cached' and build() == ''

    # Failed compilation must leave all published binaries and keys untouched.
    baseline = snapshot()
    cli = root / 'compiler/cli/main.trb'
    valid = cli.read_text()
    cli.write_text(valid + '\ndef broken(\n')
    failure = run('./trbn', '--version', check=False)
    assert failure.returncode != 0 and snapshot() == baseline, failure
    assert not (root / '.trb/bootstrap/build.lock').exists()
    assert not list((root / '.trb/bootstrap').glob('build.*'))
    cli.write_text(valid)
    assert build() == '' and snapshot() == baseline

    # Concurrent callers share one successful build and receive the same binary.
    edit('compiler/cli/main.trb', '\n# Concurrent edit\n')
    started = time.monotonic()
    deadline = started + BUILD_TIMEOUT_SECONDS
    children = [subprocess.Popen(['./trbn', '--version'], cwd=root, env=env,
                                 text=True, stdout=subprocess.PIPE,
                                 stderr=subprocess.PIPE, start_new_session=True)
                for _ in range(2)]
    try:
        outputs = [communicate(child, max(0.001, deadline - time.monotonic()))
                   for child in children]
    finally:
        for child in children:
            if child.poll() is None:
                try:
                    os.killpg(child.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
                communicate(child, 5)
    assert all(child.returncode == 0 for child in children), outputs
    assert outputs[0][0] == outputs[1][0], outputs
    assert sum('bootstrapping' in stderr for _, stderr in outputs) == 1, outputs
    # The single builder rebuilt only the CLI around the published core.
    assert sum('reusing the verified core' in stderr for _, stderr in outputs) == 1, outputs
    assert snapshot()[1] == baseline[1] and snapshot()[0] != baseline[0]
    print(f'Native concurrent cache build: {time.monotonic() - started:.2f}s', flush=True)
    assert build() == '' and plan() == 'cached'

print('Native bootstrap content cache, core reuse, failure and concurrency checks passed')
