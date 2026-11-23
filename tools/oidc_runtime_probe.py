"""Test-only foreign-resource and GC probes for emitted OIDC adapters."""
from pathlib import Path
import subprocess

from network_artifacts import target_name


PREAMBLE = r'''
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
static void *owned[1024];
static int created, freed, live, peak;
static int fault(const char *name) {
    const char *selected = getenv("OIDC_TEST_FAULT");
    return selected && !strcmp(selected, name);
}
static void *keep(void *value) {
    if (!value) return NULL;
    for (int i = 0; i < 1024; i++) if (!owned[i]) {
        owned[i] = value; created++; live++;
        if (live > peak) peak = live;
        return value;
    }
    abort();
}
static void drop(void *value) {
    if (!value) return;
    for (int i = 0; i < 1024; i++) if (owned[i] == value) {
        owned[i] = NULL; freed++; live--; return;
    }
    fprintf(stderr, "unowned OIDC resource release\n"); abort();
}
__attribute__((destructor)) static void report(void) {
    fprintf(stderr, "oidc-probe,%d,%d,%d,%d\n", created, freed, live, peak);
    if (live) abort();
}
'''

# These declarations describe the pinned C ABI for instrumentation only.
# Production adapters and ownership policy remain in TypeRB.
ALLOCATORS = {
    'uidna_openUTS46_78': ('uint32_t options, void *status', 'options,status'),
    'malloc': ('size_t n', 'n'),
    'strdup': ('const char *s', 's'),
    'BN_bin2bn': ('const unsigned char *p, int n, void *out', 'p,n,out'),
    'OSSL_PARAM_BLD_new': ('void', ''),
    'OSSL_PARAM_BLD_to_param': ('void *b', 'b'),
    'EVP_PKEY_CTX_new_from_name': ('void *c, const char *n, const char *p', 'c,n,p'),
    'EVP_MD_CTX_new': ('void', ''),
    'curl_easy_init': ('void', ''),
    'curl_url': ('void', ''),
    'BIO_new_file': ('const char *name, const char *mode', 'name,mode'),
    'd2i_X509': ('void **out, const unsigned char **data, long n', 'out,data,n'),
    'opendir': ('const char *name', 'name'),
}
RELEASES = {
    'uidna_close_78': ('void', ''),
    'free': ('void', ''), 'BN_free': ('void', ''),
    'OSSL_PARAM_BLD_free': ('void', ''), 'OSSL_PARAM_free': ('void', ''),
    'EVP_PKEY_CTX_free': ('void', ''), 'EVP_MD_CTX_free': ('void', ''),
    'EVP_PKEY_free': ('void', ''), 'curl_easy_cleanup': ('void', ''),
    'curl_url_cleanup': ('void', ''), 'curl_free': ('void', ''),
    'X509_free': ('void', ''), 'BIO_free': ('int', 'return '),
    'closedir': ('int', 'return '),
}
EXTRA = r'''
static int clock_calls;
int oidc_probe_wall_clock(int id, struct timespec *time) {
    int status = clock_gettime(id, time);
    const char *after = getenv("OIDC_TEST_CLOCK_AFTER");
    const char *seconds = getenv("OIDC_TEST_CLOCK_SECONDS");
    if (!status && after && seconds && ++clock_calls > atoi(after)) time->tv_sec += atol(seconds);
    return status;
}
extern int OPENSSL_init_crypto(uint64_t, const void *);
extern int OPENSSL_init_ssl(uint64_t, const void *);
int oidc_probe_OPENSSL_init_crypto(uint64_t flags, const void *p) {
    if (flags != 128 || p) abort();
    return fault("crypto-init") ? 0 : OPENSSL_init_crypto(flags, p);
}
int oidc_probe_OPENSSL_init_ssl(uint64_t flags, const void *p) {
    if (flags != 128 || p) abort();
    return fault("ssl-init") ? 0 : OPENSSL_init_ssl(flags, p);
}
extern int curl_easy_perform(void *);
int oidc_probe_curl_easy_perform(void *p) {
    return fault("perform") ? 7 : curl_easy_perform(p);
}
extern int curl_url_get(void *, int, char **, unsigned int);
int oidc_probe_curl_url_get(void *p, int part, char **out, unsigned int flags) {
    if (fault("url-get")) return 1;
    int result = curl_url_get(p, part, out, flags);
    if (!result) keep(*out);
    return result;
}
extern int EVP_PKEY_fromdata(void *, void **, int, const void *);
int oidc_probe_EVP_PKEY_fromdata(void *context, void **out, int selection, const void *params) {
    int result = EVP_PKEY_fromdata(context, out, selection, params);
    keep(*out);
    return fault("fromdata") ? 0 : result;
}
extern int EVP_DigestVerify(void *, const unsigned char *, size_t, const unsigned char *, size_t);
int oidc_probe_EVP_DigestVerify(void *c, const unsigned char *s, size_t sn, const unsigned char *m, size_t mn) {
    return fault("verify") ? 0 : EVP_DigestVerify(c, s, sn, m, mn);
}
extern int PEM_read_bio(void *, char **, char **, unsigned char **, long *);
int oidc_probe_PEM_read_bio(void *b, char **n, char **h, unsigned char **d, long *length) {
    int result = PEM_read_bio(b, n, h, d, length);
    if (result) { keep(*n); keep(*h); keep(*d); }
    return result;
}
extern void CRYPTO_free(void *, const char *, int);
void oidc_probe_CRYPTO_free(void *p, const char *file, int line) {
    drop(p); CRYPTO_free(p, file, line);
}
extern int inflateInit2_(void *, int, const char *, int);
int oidc_probe_inflateInit2_(void *s, int window, const char *version, int size) {
    if (fault("inflate-init")) return -4;
    int result = inflateInit2_(s, window, version, size);
    if (!result) keep(s);
    return result;
}
extern int inflateEnd(void *);
int oidc_probe_inflateEnd(void *s) { drop(s); return inflateEnd(s); }
'''


def c_source():
    lines = [PREAMBLE]
    for name, (signature, arguments) in ALLOCATORS.items():
        result_type = 'char *' if name == 'strdup' else 'void *'
        lines.append(f'extern {result_type}{name}({signature});\n'
                     f'{result_type}oidc_probe_{name}({signature}) {{\n'
                     f'  return fault("{name}") ? NULL : keep({name}({arguments}));\n}}')
    for name, (result_type, returned) in RELEASES.items():
        lines.append(f'extern {result_type} {name}(void *);\n'
                     f'{result_type} oidc_probe_{name}(void *p) {{ drop(p); {returned}{name}(p); }}')
    return '\n'.join(lines) + EXTRA


def instrument(source):
    hooks = set(ALLOCATORS) | set(RELEASES) | {
        'OPENSSL_init_crypto', 'OPENSSL_init_ssl', 'curl_easy_perform', 'curl_url_get',
        'EVP_PKEY_fromdata', 'EVP_DigestVerify', 'PEM_read_bio', 'CRYPTO_free',
        'inflateInit2_', 'inflateEnd'}
    lines, owner, ordinary, wall, collections = [], False, False, False, 0
    for line in source.splitlines():
        if line.startswith('function '):
            owner = '$trbn_oidc_' in line or '$oidc_' in line
            ordinary = '$trbnf' in line
            wall = '$trbn_time_clock(' in line
        if line == '}':
            owner = ordinary = wall = False
        if (owner and any(call in line for call in ('call $trbn_string_alloc(', 'call $trbn_string_from_bytes('))) or (
                ordinary and any(call in line for call in ('call $trbn_array_new(', 'call $trbn_array_push_',
                                                          'call $trbn_gc_alloc(', 'call $trbn_string_concat('))):
            lines.append(' call $trbn_gc_collect(w 0)')
            collections += 1
        if wall:
            line = line.replace('call $clock_gettime(', 'call $oidc_probe_wall_clock(')
        if owner:
            for name in hooks:
                line = line.replace('call $' + name + '(', 'call $oidc_probe_' + name + '(')
        lines.append(line)
    assert collections > 0
    return '\n'.join(lines) + '\n'


def build(binary, source, core, root, environment):
    command = [str(binary), *([] if core else ['--internal-driver']), 'emit-qbe', str(source)]
    emitted = subprocess.run(command, env=environment, capture_output=True, text=True, timeout=180)
    assert emitted.returncode == 0, emitted.stderr
    il, assembly, probe = root / 'stress.ssa', root / 'stress.s', root / 'probe.c'
    il.write_text(instrument(emitted.stdout))
    probe.write_text(c_source())
    qbe = Path(environment['TRBN_QBE']) if core else binary.with_name('qbe')
    target = target_name()
    profile = {'darwin-arm64': 'arm64_apple', 'darwin-amd64': 'amd64_apple',
               'linux-arm64': 'arm64', 'linux-amd64': 'amd64_sysv'}[target]
    subprocess.run([str(qbe), '-t', profile, '-o', str(assembly), str(il)], check=True, timeout=180)
    libraries = []
    for family, names in [('NETWORK', ['libuv.a', 'libllhttp.a', 'network-identity.o']),
                          ('OIDC', ['libcurl.a', 'libssl.a', 'libz.a', 'libnghttp2.a', 'libicuuc.a', 'libicu-uts46.a', 'oidc-identity.o']),
                          ('CRYPTO', ['libcrypto.a', 'crypto-identity.o'])]:
        directory = Path(environment.get('TRBN_' + family + '_LIB_DIR', str(qbe.parent / family.lower())))
        libraries.extend(str(directory / name) for name in names)
    flags = (['-lc++', '-framework', 'Security', '-framework', 'CoreFoundation', '-framework', 'SystemConfiguration']
             if target.startswith('darwin-') else ['-fuse-ld=lld', '-lstdc++', '-pthread', '-ldl', '-lrt'])
    executable = root / 'stressed'
    subprocess.run(['/usr/bin/cc', '-std=c11', '-D_POSIX_C_SOURCE=200809L', '-Werror', str(assembly),
                    str(probe), *libraries, *flags, '-lm', '-o', str(executable)], check=True, timeout=180)
    return executable


def statistics(result):
    rows = [line for line in result.stderr.splitlines() if line.startswith('oidc-probe,')]
    assert len(rows) == 1, result.stderr
    created, freed, live, peak = map(int, rows[0].split(',')[1:])
    assert live == 0 and created == freed, rows
    return {'created': created, 'freed': freed, 'live': live, 'peak': peak}
