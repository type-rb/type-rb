#!/usr/bin/env python3
"""Four-host archive ABI, static provider and isolated configuration probe."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

from network_artifact_smoke import imports
from network_artifacts import inspect, install, pack, target_name, verify

PROBE = r'''
#include <assert.h>
#include <stdint.h>
#include <stddef.h>
#include <stdio.h>
#include <string.h>
extern int OPENSSL_init_crypto(uint64_t, const void *);
extern const char *OpenSSL_version(int);
extern int EVP_Q_digest(void *, const char *, const char *, const void *, size_t, unsigned char *, size_t *);
extern unsigned char *EVP_Q_mac(void *, const char *, const char *, const char *, const void *,
    const void *, size_t, const unsigned char *, size_t, unsigned char *, size_t, size_t *);
extern int CRYPTO_memcmp(const void *, const void *, size_t);
extern const unsigned char trbn_crypto_build_identity[];
int main(void) {
    assert(sizeof(void *) == 8 && sizeof(size_t) == 8 && sizeof(int) == 4);
    assert(trbn_crypto_build_identity[0] == 'T');
    assert(OPENSSL_init_crypto(128, NULL));
    assert(!strncmp(OpenSSL_version(0), "OpenSSL 3.5.9 ", 14));
    unsigned char output[64]; size_t used = 0;
    assert(EVP_Q_digest(NULL, "SHA256", NULL, "abc", 3, output, &used) && used == 32);
    assert(fwrite(output, 1, used, stdout) == used);
    assert(EVP_Q_mac(NULL, "HMAC", NULL, "SHA256", NULL, "", 0,
        (const unsigned char *)"", 0, output, sizeof(output), &used) && used == 32);
    assert(fwrite(output, 1, used, stdout) == used);
    assert(!CRYPTO_memcmp(output, output, used));
    output[0] ^= 1;
    assert(CRYPTO_memcmp(output, "01234567890123456789012345678901", used));
}
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('libraries', type=Path)
    parser.add_argument('--receipt', type=Path, required=True)
    args = parser.parse_args()
    target = target_name()
    verify(args.libraries, target, 'crypto')
    with tempfile.TemporaryDirectory(prefix='crypto artifact ') as temporary:
        root = Path(temporary)
        archive = root / 'crypto.tar.gz'
        digest = pack(args.libraries, archive, 'crypto')
        library = root / 'installed crypto'
        install(archive, library, digest, target, 'crypto')
        source = root / 'probe.c'
        source.write_text(PROBE)
        executable = root / 'crypto-probe'
        flags = ['-pthread', '-ldl'] if target.startswith('linux-') else []
        subprocess.run(['/usr/bin/cc', '-std=c11', '-Werror', '-O2', str(source), str(library / 'libcrypto.a'),
                        str(library / 'crypto-identity.o'), *flags, '-o', str(executable)], check=True)
        configuration = root / 'configuration fifo'
        os.mkfifo(configuration)
        shutil.rmtree(library)
        result = subprocess.check_output([str(executable)], env={'PATH': '', 'OPENSSL_CONF': str(configuration),
                                         'OPENSSL_MODULES': str(root / 'absent')}, timeout=10)
        assert result.hex() == ('ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad' +
                                'b613679a0814d9ec772f95d778c35fc5ff1697c493715653c6c712144292c5ad')
        linked = imports(executable)
        assert not any('libcrypto' in name or 'libssl' in name for name in linked), linked
        args.receipt.write_text(json.dumps({'target': target, 'executable_bytes': executable.stat().st_size,
                                           'imports': linked, 'identity': inspect(executable, 'crypto'),
                                           'configuration_isolation': True}, indent=2) + '\n')
    print('static crypto installation, ABI, configuration isolation and execution passed')


if __name__ == '__main__':
    main()
