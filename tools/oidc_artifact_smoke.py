#!/usr/bin/env python3
"""Qualify a relocated static OIDC/crypto bundle against a local HTTPS fixture."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

from network_artifact_smoke import imports
from network_artifacts import inspect, install, pack, target_name, verify
from oidc_http2_fixture import Http2Provider
from oidc_idna_corpus import qualify as qualify_idna

PROBE = r'''
#include <assert.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
extern int OPENSSL_init_ssl(uint64_t, const void *);
extern int curl_global_init(long);
extern void curl_global_cleanup(void);
extern void *curl_easy_init(void);
extern void curl_easy_cleanup(void *);
extern int curl_easy_setopt(void *, int, ...);
extern int curl_easy_perform(void *);
extern int curl_easy_getinfo(void *, int, ...);
extern const char *curl_easy_strerror(int);
extern const unsigned char trbn_oidc_build_identity[], trbn_crypto_build_identity[];
extern void udata_setFileAccess_78(int, int32_t *);
extern void *uidna_openUTS46_78(uint32_t, int32_t *);
extern int32_t uidna_nameToASCII_UTF8_78(const void *, const char *, int32_t, char *, int32_t, void *, int32_t *);
extern void uidna_close_78(void *);
struct idna_info { int16_t size; uint8_t transitional, reserved; uint32_t errors; int32_t r1, r2; };
static size_t write_body(char *data, size_t size, size_t count, void *unused) {
    (void)unused;
    return fwrite(data, 1, size * count, stdout);
}
int main(int argc, char **argv) {
    assert(argc == 3);
    assert(trbn_oidc_build_identity[0] == 'T' && trbn_crypto_build_identity[0] == 'T');
    int32_t error = 0;
    udata_setFileAccess_78(3, &error); assert(error <= 0);
    void *idna = uidna_openUTS46_78(30, &error); assert(idna && error <= 0);
    struct idna_info info = { .size = 16 };
    char ascii[256];
    const char *unicode = "B\xc3\x9c" "CHER.example.invalid";
    int32_t length = uidna_nameToASCII_UTF8_78(idna, unicode, (int32_t)strlen(unicode), ascii, 256, &info, &error);
    assert(error <= 0 && !info.errors && length == 29 && !memcmp(ascii, "xn--bcher-kva.example.invalid", 29));
    uidna_close_78(idna);
    assert(OPENSSL_init_ssl(128, NULL));
    assert(!curl_global_init(3));
    void *h = curl_easy_init(); assert(h);
    assert(!curl_easy_setopt(h, 10002, argv[1]));
    assert(!curl_easy_setopt(h, 10065, argv[2]));
    assert(!curl_easy_setopt(h, 10004, ""));
    assert(!curl_easy_setopt(h, 20011, write_body));
    assert(!curl_easy_setopt(h, 155, 5000L));
    assert(!curl_easy_setopt(h, 99, 1L));
    assert(!curl_easy_setopt(h, 10318, "http,https"));
    assert(!curl_easy_setopt(h, 84, 4L));
    int result = curl_easy_perform(h);
    if (result) fprintf(stderr, "%s\n", curl_easy_strerror(result));
    long status = 0; assert(!curl_easy_getinfo(h, 2097154, &status));
    curl_easy_cleanup(h); curl_global_cleanup();
    return result || status != 200;
}
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('libraries', type=Path)
    parser.add_argument('--crypto', type=Path, required=True)
    parser.add_argument('--receipt', type=Path, required=True)
    args = parser.parse_args()
    target = target_name()
    oidc = verify(args.libraries, target, 'oidc')
    crypto = verify(args.crypto, target, 'crypto')
    assert oidc['configuration']['crypto_libraries'] == crypto['libraries']
    assert oidc['configuration']['crypto_configuration'] == crypto['configuration']
    assert oidc['configuration']['openssl'] == crypto['pins']
    spec = importlib.util.spec_from_file_location('oidc_fixture', Path(__file__).with_name('native-oidc-test.py'))
    fixture = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(fixture)
    with tempfile.TemporaryDirectory(prefix='oidc artifact ') as temporary:
        root = Path(temporary)
        directories = {}
        for family, source in [('oidc', args.libraries), ('crypto', args.crypto)]:
            archive = root / (family + '.tar.gz')
            installed = root / ('installed ' + family)
            install(archive, installed, pack(source, archive, family), target, family)
            directories[family] = installed
        idna = qualify_idna(directories['oidc'], root / 'idna-corpus')
        source = root / 'probe.c'
        source.write_text(PROBE)
        executable = root / 'probe'
        libraries = [directories['oidc'] / name for name in ('libcurl.a', 'libssl.a', 'libz.a', 'libnghttp2.a', 'libicuuc.a', 'libicu-uts46.a', 'oidc-identity.o')]
        libraries += [directories['crypto'] / name for name in ('libcrypto.a', 'crypto-identity.o')]
        flags = (['-lc++', '-framework', 'Security', '-framework', 'CoreFoundation', '-framework', 'SystemConfiguration']
                 if target.startswith('darwin-') else ['-lstdc++', '-pthread', '-ldl', '-lm'])
        subprocess.run(['/usr/bin/cc', '-std=c11', '-Werror', str(source), *map(str, libraries),
                        *flags, '-o', str(executable)], check=True, timeout=180)
        for directory in directories.values():
            shutil.rmtree(directory)
        configuration = root / 'configuration fifo'
        os.mkfifo(configuration)
        with fixture.Provider(root, tls=True) as provider:
            result = subprocess.run([str(executable), provider.url + '/keys', str(provider.certificate)],
                                    capture_output=True, text=True, timeout=10,
                                    env={'PATH': '', 'OPENSSL_CONF': str(configuration), 'ICU_DATA': str(configuration),
                                         'OPENSSL_MODULES': str(root / 'absent'), 'OPENSSL_ENGINES': str(root / 'absent')})
            assert result.returncode == 0 and json.loads(result.stdout) == {'keys': [provider.jwk]}, result
            with Http2Provider(root / 'http2', {'keys': [provider.jwk]}) as http2:
                for protocol, path in [('h1', '/keys'), ('h2', '/required/keys')]:
                    result = subprocess.run([str(executable), http2.endpoints[protocol] + path,
                                             http2.endpoints['certificate']], capture_output=True, text=True,
                                            timeout=10, env={'PATH': '', 'OPENSSL_CONF': str(configuration), 'ICU_DATA': str(configuration)})
                    assert result.returncode == 0 and json.loads(result.stdout) == {'keys': [provider.jwk]}, result
                records = http2.requests()
                assert [(r['protocol'], r['alpn']) for r in records] == [('HTTP/1.1', 'http/1.1'), ('HTTP/2.0', 'h2')], records
                transport = {'provider': http2.identity, 'requests': records}
        linked = imports(executable)
        assert not any(part in name for name in linked for part in ('libcurl', 'libssl', 'libcrypto', 'libz.', 'libnghttp2', 'libicu')), linked
        args.receipt.write_text(json.dumps({'target': target, 'executable_bytes': executable.stat().st_size,
                                           'imports': linked, 'identity': inspect(executable, 'oidc'),
                                           'crypto_identity': inspect(executable, 'crypto'),
                                           'https_relocated': True, 'configuration_isolation': True,
                                           'http2': transport, 'idna': idna}, indent=2) + '\n')
    print('static OIDC HTTPS, relocation and single OpenSSL configuration passed')


if __name__ == '__main__':
    main()
