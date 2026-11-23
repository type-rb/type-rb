#!/usr/bin/env python3
"""Qualify libc layouts used by the scoped filesystem adapter, not its semantics."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import signal
import subprocess
import tempfile
import time


# Host-tool startup is not an application performance measurement. Keep its
# watchdog aligned with compilation; the compiled ABI probe still has ten seconds.
HOST_TOOL_TIMEOUT = 60


def run_command(command, root, checks, timeout=HOST_TOOL_TIMEOUT):
    record = {'command': [str(part).replace(str(root), '<work>') for part in command],
              'timeout_seconds': timeout, 'complete': False}
    checks.append(record)
    started = time.monotonic()
    process = None
    try:
        process = subprocess.Popen(command, cwd=root, text=True, errors='replace',
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   start_new_session=True)
        try:
            stdout, stderr = process.communicate(timeout=timeout)
        except BaseException:
            # A tool or wrapper can leave children holding the output pipes.
            # Terminate the owned group before collecting the final diagnostics.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            stdout, stderr = process.communicate()
            record.update(code=process.returncode, stdout=stdout, stderr=stderr)
            raise
        record.update(code=process.returncode, stdout=stdout, stderr=stderr)
        if process.returncode:
            raise subprocess.CalledProcessError(process.returncode, command, stdout, stderr)
        record['complete'] = True
        return stdout
    except BaseException as error:
        record['error'] = type(error).__name__ + ': ' + str(error).replace(str(root), '<work>')
        record['timed_out'] = isinstance(error, subprocess.TimeoutExpired)
        raise
    finally:
        record['elapsed_seconds'] = time.monotonic() - started


def abi_profile(target):
    if target not in ('darwin-arm64', 'darwin-amd64', 'linux-arm64', 'linux-amd64'):
        raise ValueError('unsupported filesystem ABI target: ' + target)
    darwin = target.startswith('darwin-')
    arm = target.endswith('arm64')
    return {
        'sizeof(struct stat)': 144 if darwin or not arm else 128,
        'sizeof(mode_t)': 2 if darwin else 4,
        'offsetof(struct utsname, machine)': 1024 if darwin else 260,
        'sizeof(off_t)': 8, 'sizeof(ssize_t)': 8, 'sizeof(void *)': 8,
        'offsetof(struct stat, st_mode)': 4 if darwin else (16 if arm else 24),
        'offsetof(struct stat, st_size)': 96 if darwin else 48,
        'offsetof(struct dirent, d_name)': 21 if darwin else 19,
        'offsetof(struct dirent, d_type)': 20 if darwin else 18,
        'FTW_F': 0, 'FTW_D': 1, 'FTW_DNR': 2, 'FTW_NS': 4 if darwin else 3, 'FTW_PHYS': 1,
        'O_RDONLY': 0, 'O_WRONLY': 1, 'O_RDWR': 2,
        'O_CREAT': 512 if darwin else 64, 'O_EXCL': 2048 if darwin else 128,
        'O_NONBLOCK': 4 if darwin else 2048, 'O_NOCTTY': 131072 if darwin else 256,
        'O_CLOEXEC': 16777216 if darwin else 524288,
        'AT_SYMLINK_NOFOLLOW': 32 if darwin else 256,
        'S_IFMT': 61440, 'S_IFREG': 32768, 'S_IFDIR': 16384, 'S_IFLNK': 40960,
        'EIO': 5, 'ENXIO': 6, 'EINTR': 4, 'ENOENT': 2, 'EACCES': 13, 'EEXIST': 17,
        'ENOTDIR': 20, 'EISDIR': 21, 'EINVAL': 22,
    }


def probe_source(profile, selected_symbols):
    source = ('#define _GNU_SOURCE\n#define _DARWIN_C_SOURCE\n'
              '#include <stddef.h>\n#include <sys/stat.h>\n#include <fcntl.h>\n'
              '#include <ftw.h>\n#include <unistd.h>\n#include <dirent.h>\n#include <errno.h>\n#include <sys/utsname.h>\n#include <dlfcn.h>\n')
    source += ''.join('_Static_assert(' + name + ' == ' + str(value) + ', "' + name + '");\n'
                      for name, value in profile.items())
    # Compiler-owned test declarations keep prototypes and actual link symbols
    # visible. They are not a C implementation of the TypeRB runtime.
    source += '''int (*volatile stat_handle)(int, struct stat *) = &fstat;
int (*volatile truncate_handle)(int, off_t) = &ftruncate;
int (*volatile acquire)(const char *, int, ...) = &open;
int (*volatile release)(int) = &close;
ssize_t (*volatile read_bytes)(int, void *, size_t) = &read;
ssize_t (*volatile write_bytes)(int, const void *, size_t) = &write;
DIR *(*volatile open_directory)(const char *) = &opendir;
struct dirent *(*volatile next_entry)(DIR *) = &readdir;
int (*volatile close_directory)(DIR *) = &closedir;
int (*volatile directory_handle)(DIR *) = &dirfd;
int (*volatile entry_info)(int, const char *, struct stat *, int) = &fstatat;
int (*volatile create_directory)(const char *, mode_t) = &mkdir;
int (*volatile host_identity)(struct utsname *) = &uname;
void *(*volatile dynamic_symbol)(void *, const char *) = &dlsym;
'''
    # The raw backend cannot inherit the C headers' symbol renaming. Verify
    # that each explicitly selected libc entry is the header-qualified one.
    source += 'int main(void) {\n'
    for number, (name, symbol) in enumerate(selected_symbols.items(), 1):
        source += f'  if (dlsym(RTLD_DEFAULT, "{symbol}") != (void *)&{name}) return {number};\n'
    source += '  return 0;\n}\n'
    return source


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--receipt', type=Path)
    args = parser.parse_args()
    system = platform.system().lower()
    machine = {'aarch64': 'arm64', 'x86_64': 'amd64'}.get(platform.machine(), platform.machine())
    target = system + '-' + machine
    receipt = {'schemaVersion': 1, 'target': target, 'complete': False, 'checks': [],
               'scope': 'libc ABI only; Native filesystem semantics require separate execution'}
    try:
        profile = abi_profile(target)
        suffix = '$INODE64' if target == 'darwin-amd64' else ''
        selected_symbols = {name: name + suffix for name in ('fstat', 'stat', 'lstat', 'readdir')}
        source = probe_source(profile, selected_symbols)
        receipt.update(assertions=profile, source_sha256=hashlib.sha256(source.encode()).hexdigest(),
                       header_verified_symbols=selected_symbols)
        compiler = shutil.which(os.environ.get('CC', 'cc'))
        inspector = shutil.which('nm')
        receipt['tools'] = {'compiler': compiler, 'symbols': inspector}
        if not compiler or not inspector:
            raise ValueError('filesystem ABI qualification requires both a C compiler and nm')
        with tempfile.TemporaryDirectory(prefix='native-filesystem-abi-') as temporary:
            root = Path(temporary)
            probe = root / 'probe.c'
            executable = root / 'probe'
            probe.write_text(source)
            flags = ['-ldl'] if system == 'linux' else []
            run_command([compiler, '-std=c11', '-Werror', str(probe), '-o', str(executable)] + flags,
                        root, receipt['checks'])
            run_command([str(executable)], root, receipt['checks'], timeout=10)
            symbols = run_command([inspector, '-u', str(executable)], root, receipt['checks']).splitlines()
            receipt['undefined_symbols'] = [line.strip() for line in symbols]
            receipt['compiler'] = run_command([compiler, '--version'], root, receipt['checks']).splitlines()[0]
            receipt['complete'] = True
    except BaseException as error:
        receipt['error'] = type(error).__name__ + ': ' + str(error)
        raise
    finally:
        if args.receipt:
            args.receipt.parent.mkdir(parents=True, exist_ok=True)
            args.receipt.write_text(json.dumps(receipt, indent=2) + '\n')
        print(json.dumps(receipt, indent=2))


if __name__ == '__main__':
    main()
