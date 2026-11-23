#!/usr/bin/env python3
"""Four-host installed static gzip archive ABI and isolated execution probe."""
import argparse
import gzip
import json
from pathlib import Path
import subprocess
import tempfile

from network_artifact_smoke import imports
from network_artifacts import inspect, install, pack, target_name, verify

PROBE = r'''
#include <assert.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
struct libdeflate_compressor;
extern struct libdeflate_compressor *libdeflate_alloc_compressor(int);
extern size_t libdeflate_gzip_compress_bound(struct libdeflate_compressor *, size_t);
extern size_t libdeflate_gzip_compress(struct libdeflate_compressor *, const void *, size_t, void *, size_t);
extern void libdeflate_free_compressor(struct libdeflate_compressor *);
extern const unsigned char trbn_compression_build_identity[];
int main(void) {
  assert(sizeof(void *) == 8 && sizeof(size_t) == 8 && sizeof(int) == 4);
  assert(trbn_compression_build_identity[0] == 'T');
  struct libdeflate_compressor *state = libdeflate_alloc_compressor(-1); assert(state);
  const char input[] = "portable gzip\0binary";
  size_t capacity = libdeflate_gzip_compress_bound(state, sizeof(input) - 1);
  void *output = malloc(capacity); assert(output);
  size_t used = libdeflate_gzip_compress(state, input, sizeof(input) - 1, output, capacity); assert(used);
  libdeflate_free_compressor(state);
  assert(fwrite(output, 1, used, stdout) == used);
  free(output);
}
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('libraries', type=Path)
    parser.add_argument('--receipt', type=Path, required=True)
    args = parser.parse_args()
    verify(args.libraries, target_name(), 'compression')
    with tempfile.TemporaryDirectory(prefix='gzip artifact ') as temporary:
        root = Path(temporary)
        archive = root / 'gzip.tar.gz'
        digest = pack(args.libraries, archive, 'compression')
        library = root / 'installed gzip'
        install(archive, library, digest, target_name(), 'compression')
        source = root / 'probe.c'
        source.write_text(PROBE)
        executable = root / 'gzip-probe'
        subprocess.run(['/usr/bin/cc', '-O2', str(source), str(library / 'libdeflate.a'),
                        str(library / 'compression-identity.o'), '-o', str(executable)], check=True)
        result = subprocess.check_output([str(executable)], env={'PATH': ''}, timeout=10)
        assert gzip.decompress(result) == b'portable gzip\0binary'
        linked = imports(executable)
        assert not any('libdeflate' in name for name in linked), linked
        args.receipt.write_text(json.dumps({'target': target_name(), 'executable_bytes': executable.stat().st_size,
                                           'imports': linked, 'identity': inspect(executable, 'compression')}, indent=2) + '\n')
    print('static gzip installation, ABI and isolated execution passed')


if __name__ == '__main__':
    main()
