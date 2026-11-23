#!/usr/bin/env python3
"""Qualify a prebuilt archive installation and static link on its actual host."""
import argparse
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import tempfile

from network_artifacts import identity, inspect, install, pack, target_name, verify


def imports(executable):
    if platform.system() == 'Darwin':
        output = subprocess.check_output(['otool', '-l', str(executable)], text=True)
        return re.findall(r'cmd LC_LOAD_DYLIB\n\s+cmdsize \d+\n\s+name (\S+)', output)
    output = subprocess.check_output(['readelf', '-d', str(executable)], text=True)
    return re.findall(r'\(NEEDED\).*?\[(.*?)\]', output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('libraries', type=Path)
    parser.add_argument('--receipt', type=Path, required=True)
    args = parser.parse_args()
    receipt = verify(args.libraries, target_name())
    with tempfile.TemporaryDirectory(prefix='network artifact ') as temporary:
        root = Path(temporary)
        archive = root / 'network.tar.gz'
        digest = pack(args.libraries, archive)
        installed = root / 'installed with spaces' / 'network'
        install(archive, installed, digest, target_name())
        source = root / 'abi-link.c'
        source.write_text('''extern unsigned uv_version(void);
extern void llhttp_init(void *, int, const void *);
extern const unsigned char trbn_network_build_identity[];
int main(void) {
    _Alignas(8) unsigned char parser[96] = {0};
    _Alignas(8) unsigned char settings[200] = {0};
    llhttp_init(parser, 1, settings);
    return uv_version() != 79104 || trbn_network_build_identity[0] != 'T';
}
''')
        executable = root / 'probe'
        flags = ['-pthread', '-ldl', '-lrt'] if platform.system() == 'Linux' else []
        subprocess.run([os.environ.get('CC', 'cc'), str(source),
                        *[str(installed / name) for name in ('libuv.a', 'libllhttp.a', 'network-identity.o')],
                        *flags, '-o', str(executable)], check=True)
        subprocess.run([str(executable)], check=True, env={'PATH': ''})
        assert inspect(executable) == identity(receipt)
        linked = imports(executable)
        assert not any('libuv' in path or 'llhttp' in path for path in linked), linked
        args.receipt.write_text(json.dumps({'target': target_name(), 'artifact_sha256': digest,
                                           'executable_bytes': executable.stat().st_size,
                                           'imports': linked, 'identity': inspect(executable)}, indent=2) + '\n')
    print('prebuilt archive installation, ABI link and isolated execution passed')


if __name__ == '__main__':
    main()
