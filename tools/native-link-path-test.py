#!/usr/bin/env python3
"""Check actual compiler link helpers, including path-specific syscall failures."""
import argparse
import errno
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


FAMILIES = {
    'network': ('libuv.a', 'libllhttp.a', 'network-identity.o'),
    'compression': ('libdeflate.a', 'compression-identity.o'),
    'crypto': ('libcrypto.a', 'crypto-identity.o'),
    'oidc': ('libcurl.a', 'libssl.a', 'libz.a', 'libnghttp2.a', 'libicuuc.a', 'libicu-uts46.a', 'oidc-identity.o'),
    'sqlite': ('libsqlite3.a', 'sqlite-identity.o'),
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--qbe', type=Path, required=True)
    parser.add_argument('--source', type=Path, help='Runtime directory for a recorded regression control')
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    source = args.source or root / 'compiler/src/backend/qbe/runtime'
    chunks = []
    for family in FAMILIES:
        text = (source / (family + '_link.trb')).read_text()
        chunks.extend(json.loads(match.group()) for match in re.finditer(r'"(?:[^"\\]|\\.)*"', text))
    il = '\n'.join(chunks) + '''
data $trbn_project_selected_linker = align 8 { l 0 }
data $trbn_selected_dead_strip = align 8 { l 0 }
data $trbn_dead_strip_linux = { b "--gc-sections", b 0 }
'''
    il = re.sub(r'^(function(?: [lw])?|data) ', r'export \1 ', il, flags=re.M)
    # Intercept libc failures. All path selection, argv ownership,
    # failure propagation and diagnostics remain the production QBE functions.
    il = il.replace('call $realpath(', 'call $test_realpath(').replace('call $access(', 'call $test_access(')
    il = il.replace('call $strdup(', 'call $test_strdup(').replace('call $malloc(', 'call $test_malloc(')
    il = il.replace('call $free(', 'call $test_free(').replace('call $getenv(', 'call $test_getenv(')
    helper = r'''
#include <errno.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
#define FAMILY(name) extern int64_t trbn_##name##_link_required; \
 extern void *trbn_##name##_link_arguments(void *, const char *); \
 extern void trbn_##name##_link_cleanup(void *);
FAMILY(network) FAMILY(compression) FAMILY(crypto) FAMILY(oidc) FAMILY(sqlite)
static const char *scenario;
static int resolutions, reads;
/* Reserve virtual address space, then commit only two pages we own. This
 * makes low-word-zero pointers deterministic without MAP_FIXED or a 4 GiB
 * physical allocation. Never overwrite an existing process mapping. */
static struct { void *mapping, *value; size_t size; } mappings[48];
static int wide_hits;
static void *wide_allocate(size_t size, size_t before_boundary) {
 const size_t boundary=(size_t)UINT64_C(1)<<32;
 const size_t page=(size_t)sysconf(_SC_PAGESIZE);
 if(size>page || before_boundary>=page)abort();
 for(size_t i=0;i<48;i++)if(!mappings[i].mapping) {
  size_t reservation=boundary+2*page;
  void *mapping=mmap(NULL,reservation,PROT_NONE,MAP_PRIVATE|MAP_ANON,-1,0);
  if(mapping==MAP_FAILED){perror("reserve pointer boundary");exit(77);}
  uintptr_t aligned=((uintptr_t)mapping+page+boundary-1)&~(boundary-1);
  if(mprotect((void *)(aligned-page),2*page,PROT_READ|PROT_WRITE)) {
   perror("commit pointer boundary");exit(77);
  }
  mappings[i].mapping=mapping; mappings[i].size=reservation;
  mappings[i].value=(void *)(aligned-before_boundary); wide_hits++;
  return mappings[i].value;
 }
 abort();
}
static char *wide_copy(const char *text, size_t before_boundary) {
 size_t size=strlen(text)+1;
 return memcpy(wide_allocate(size,before_boundary),text,size);
}
void test_free(void *value) {
 for(size_t i=0;i<48;i++)if(value && value==mappings[i].value) {
  if(munmap(mappings[i].mapping,mappings[i].size))abort();
  mappings[i].mapping=NULL; mappings[i].value=NULL; return;
 }
 free(value);
}
char *test_getenv(const char *name) {
 char *value=getenv(name);
 return value && !strcmp(scenario,"wide-environment")?wide_copy(value,0):value;
}
char *test_strdup(const char *path) {
 if(!strcmp(scenario,"wide-copy"))return wide_copy(path,0);
 if(!strcmp(scenario,"wide-slash")) {
  const char *slash=strrchr(path,'/'); if(!slash)abort();
  return wide_copy(path,(size_t)(slash-path));
 }
 if (!strcmp(scenario, "copy-failed")) { errno=ENOMEM; return NULL; }
 return strdup(path);
}
void *test_malloc(size_t size) {
 if(!strcmp(scenario,"wide-allocation"))return wide_allocate(size,0);
 if (!strcmp(scenario, "allocate-failed")) { errno=ENOMEM; return NULL; }
 return malloc(size);
}
char *test_realpath(const char *path, char *buffer) {
 resolutions++;
 if (!strcmp(scenario, "resolve-denied")) { errno=EACCES; return NULL; }
 if(!strcmp(scenario,"wide-realpath")) {
  char *resolved=realpath(path,buffer); if(!resolved)return NULL;
  char *copy=wide_copy(resolved,0); free(resolved); return copy;
 }
 return realpath(path,buffer);
}
int test_access(const char *path, int mode) {
 reads++;
 if (!strcmp(scenario,"access-io")) { errno=EIO; return -1; }
 return access(path,mode);
}
int main(int argc,char **argv) {
 if(argc!=4)return 64;
 scenario=argv[2];
 void *slots[48]={0}; void *tail=NULL;
#define RUN(name) if(!strcmp(argv[1],#name)) { \
 trbn_##name##_link_required=strcmp(scenario,"unused")!=0; \
 tail=trbn_##name##_link_arguments(slots,argv[3]); \
 if(tail)trbn_##name##_link_cleanup(slots); }
 RUN(network) RUN(compression) RUN(crypto) RUN(oidc) RUN(sqlite)
 if(!strncmp(scenario,"wide-",5) && !wide_hits)abort();
 /* getenv results are borrowed, while allocations must have been released. */
 for(size_t i=0;i<48;i++)if(mappings[i].mapping) {
  if(strcmp(scenario,"wide-environment"))abort();
  test_free(mappings[i].value);
 }
 printf("%d %d\n",resolutions,reads);
 return tail?0:78;
}
'''
    with tempfile.TemporaryDirectory(prefix='native link paths ') as temporary:
        work = Path(temporary)
        (work / 'test.ssa').write_text(il)
        (work / 'helper.c').write_text(helper)
        subprocess.run([str(args.qbe.resolve()), '-o', str(work / 'test.s'), str(work / 'test.ssa')], check=True)
        subprocess.run(['/usr/bin/cc', '-Wall', '-Wextra', '-Werror', str(work / 'test.s'), str(work / 'helper.c'), '-o', str(work / 'test')], check=True)
        count = 0
        for family, files in FAMILIES.items():
            directory = work / family
            directory.mkdir()
            for name in files:
                (directory / name).write_bytes(b'fixture\n')
            env = dict(os.environ, LC_ALL='C')
            for other in FAMILIES:
                env.pop('TRBN_' + other.upper() + '_LIB_DIR', None)
            variable = 'TRBN_' + family.upper() + '_LIB_DIR'

            def check(scenario, expected_code=0, environment=env, error_path=None, error_number=None,
                      error_text=None, qbe_path=None):
                nonlocal count
                result = subprocess.run([str(work / 'test'), family, scenario, str(qbe_path or work / 'qbe')], env=environment, capture_output=True, text=True, timeout=10)
                assert result.returncode == expected_code, (family, scenario, result)
                if error_text is not None:
                    assert error_text in result.stderr, (family, scenario, result)
                    assert 'compiler: pinned static ' in result.stderr, (family, scenario, result)
                elif error_path is None:
                    assert result.stderr == '', (family, scenario, result)
                else:
                    assert str(error_path) + ': ' + os.strerror(error_number) in result.stderr, (family, scenario, result)
                    assert 'compiler: pinned static ' in result.stderr, (family, scenario, result)
                if scenario == 'unused':
                    assert result.stdout == '0 0\n', result
                count += 1

            check('adjacent')
            for scenario in ('wide-copy', 'wide-slash', 'wide-allocation', 'wide-realpath'):
                check(scenario)
            check('wide-environment', environment=dict(env, **{variable: str(directory)}))
            check('empty', environment=dict(env, **{variable: ''}))
            check('explicit', environment=dict(env, **{variable: str(directory)}))
            link = work / (family + ' alias')
            link.symlink_to(directory, target_is_directory=True)
            check('explicit-link', environment=dict(env, **{variable: str(link)}))
            check('no-parent', 78, qbe_path='qbe',
                  error_text='compiler: QBE path has no parent directory: qbe\n')
            check('copy-failed', 78,
                  error_text='compiler: copy QBE path: ' + os.strerror(errno.ENOMEM))
            check('allocate-failed', 78,
                  error_text='compiler: allocate static-library path: ' + os.strerror(errno.ENOMEM))
            check('allocate-failed', 78, environment=dict(env, **{variable: str(directory)}),
                  error_text='compiler: allocate static-library path: ' + os.strerror(errno.ENOMEM))
            missing = work / 'missing'
            check('unused', environment=dict(env, **{variable: str(missing)}))
            check('missing', 78, dict(env, **{variable: str(missing)}), missing, errno.ENOENT)
            check('resolve-denied', 78, error_path=directory, error_number=errno.EACCES)
            check('access-io', 78, error_path=directory / files[0], error_number=errno.EIO)
            for name in files:
                path = directory / name
                path.unlink()
                check('missing-file', 78, error_path=path, error_number=errno.ENOENT)
                path.write_bytes(b'fixture\n')
        print(f'{count} compiler link-path and syscall-failure checks passed')


if __name__ == '__main__':
    main()
