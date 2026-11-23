#!/usr/bin/env python3
"""Verify Linux CI tools, acquiring only missing packages from the configured OS."""
import argparse
import json
import os
from pathlib import Path
import platform
import shutil
import signal
import subprocess
import tempfile
import time


PACKAGES = {
    'cc': 'build-essential', 'ld.lld': 'lld', 'time': 'time',
    'nm': 'binutils', 'objdump': 'binutils', 'readelf': 'binutils',
    'file': 'file', 'jq': 'jq', 'strace': 'strace',
    'clang': 'clang', 'valgrind': 'valgrind',
}
PROFILES = {
    'linker': ['cc', 'ld.lld'],
    'cli': ['cc', 'ld.lld', 'time'],
    'targets': ['cc', 'ld.lld', 'nm', 'objdump', 'readelf', 'file', 'jq', 'strace', 'time'],
    'memory': ['cc', 'ld.lld', 'clang', 'file', 'strace', 'valgrind'],
}


class Preparation:
    def __init__(self, profile, bin_dir):
        self.profile = profile
        self.bin_dir = bin_dir.resolve()
        self.environment = dict(os.environ)
        self.receipt = {'profile': profile, 'complete': False, 'commands': [], 'tools': {}}

    def run(self, command, timeout=60):
        row = {'command': list(map(str, command)), 'complete': False}
        self.receipt['commands'].append(row)
        started = time.monotonic()
        child = subprocess.Popen(row['command'], env=self.environment, text=True,
                                 stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                 start_new_session=True)
        try:
            stdout, stderr = child.communicate(timeout=timeout)
        except BaseException as error:
            try:
                os.killpg(child.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            stdout, stderr = child.communicate()
            row.update(exit=child.returncode, stdout=stdout, stderr=stderr,
                       error=type(error).__name__)
            raise
        finally:
            row['elapsedSeconds'] = time.monotonic() - started
        row.update(exit=child.returncode, stdout=stdout, stderr=stderr,
                   complete=child.returncode == 0)
        if child.returncode:
            raise RuntimeError(f"tool exited {child.returncode}: {row['command']!r}\n{stderr}")
        return stdout

    def which(self, name):
        return shutil.which(name, path=self.environment.get('PATH', ''))

    def expose_linker(self):
        if self.which('ld.lld'):
            return
        # Ubuntu 24.04's lld metapackage selects LLD 18. Hosted images already
        # install lld-18 with Clang, but need not install the unversioned alias.
        installed = self.which('ld.lld-18')
        if not installed:
            return
        version = self.run([installed, '--version'])
        if not version.startswith(('LLD 18.', 'Debian LLD 18.', 'Ubuntu LLD 18.')):
            raise RuntimeError(f'expected installed LLD 18, got {version!r}')
        self.bin_dir.mkdir(parents=True, exist_ok=False)
        (self.bin_dir / 'ld.lld').symlink_to(installed)
        self.environment['PATH'] = str(self.bin_dir) + os.pathsep + self.environment.get('PATH', '')
        self.receipt['linkerAlias'] = {'path': str(self.bin_dir / 'ld.lld'),
                                      'target': installed, 'version': version.strip()}

    def asan_archive(self):
        clang = self.which('clang')
        if not clang:
            return None
        resource = Path(self.run([clang, '-print-resource-dir']).strip())
        architecture = {'aarch64': 'aarch64', 'x86_64': 'x86_64'}.get(platform.machine())
        if architecture is None:
            raise RuntimeError('unsupported Linux sanitizer architecture')
        archive = resource / 'lib' / 'linux' / f'libclang_rt.asan-{architecture}.a'
        return archive if archive.is_file() else None

    def prepare(self):
        self.expose_linker()
        missing = {PACKAGES[name] for name in PROFILES[self.profile] if not self.which(name)}
        if self.profile == 'memory' and not self.asan_archive():
            missing.add('libclang-rt-dev')
        self.receipt['missingPackages'] = sorted(missing)
        if missing:
            # Fail on any failed index acquisition; never continue with stale
            # indexes after a DNS error or retry until an observation passes.
            # Bound privileged children inside sudo as well: an unprivileged
            # Python controller cannot reliably signal root-owned apt workers.
            apt = ['sudo', '-n', 'timeout', '--kill-after=10s', '840s', 'apt-get']
            self.run([*apt, 'update', '-o', 'APT::Update::Error-Mode=any'], 900)
            self.run([*apt, 'install', '-y', *sorted(missing)], 900)
            self.expose_linker()
        for name in PROFILES[self.profile]:
            path = self.which(name)
            if not path:
                raise RuntimeError(f'required executable still missing: {name}')
            version = self.run([path, '--version'])
            if name == 'ld.lld' and 'LLD' not in version:
                raise RuntimeError('required linker is not LLD')
            self.receipt['tools'][name] = {'path': path, 'resolvedPath': str(Path(path).resolve()),
                                           'version': version.strip()}
        with tempfile.TemporaryDirectory(prefix='linux-ci-probe-') as directory:
            root = Path(directory)
            source = root / 'main.c'
            source.write_text('int main(void) { return 0; }\n')
            linked = root / 'linked'
            self.run([self.which('cc'), source, '-fuse-ld=lld', '-o', linked])
            self.run([linked])
            if self.profile == 'memory':
                archive = self.asan_archive()
                if archive is None:
                    raise RuntimeError('required Clang AddressSanitizer archive is missing')
                self.receipt['asanArchive'] = str(archive)
                sanitized = root / 'sanitized'
                self.run([self.which('clang'), '-fsanitize=address', source, '-o', sanitized])
                self.run([sanitized])
        # Publish the job-local alias only after every required probe passes.
        if 'linkerAlias' in self.receipt:
            with Path(os.environ['GITHUB_PATH']).open('a') as stream:
                stream.write(str(self.bin_dir) + '\n')
        self.receipt['complete'] = True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('profile', choices=PROFILES)
    parser.add_argument('--bin-dir', type=Path, required=True)
    parser.add_argument('--receipt', type=Path, required=True)
    args = parser.parse_args()
    preparation = Preparation(args.profile, args.bin_dir)
    try:
        if platform.system() != 'Linux':
            raise RuntimeError('this prerequisite check requires Linux')
        preparation.prepare()
        print(json.dumps(preparation.receipt, indent=2))
    except BaseException as error:
        preparation.receipt['error'] = f'{type(error).__name__}: {error}'
        raise
    finally:
        args.receipt.parent.mkdir(parents=True, exist_ok=True)
        args.receipt.write_text(json.dumps(preparation.receipt, indent=2) + '\n')


if __name__ == '__main__':
    main()
