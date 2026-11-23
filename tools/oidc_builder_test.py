#!/usr/bin/env python3
"""Exercise pinned source acquisition and actual compiler header selection."""
import hashlib
import importlib.util
import io
import os
from pathlib import Path
import socket
import ssl
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import call, patch

spec = importlib.util.spec_from_file_location('oidc_builder', Path(__file__).with_name('build-oidc-dependencies.py'))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class OidcBuilderTest(unittest.TestCase):
    def source_fixture(self):
        contents = io.BytesIO()
        with tarfile.open(fileobj=contents, mode='w:gz') as archive:
            member = tarfile.TarInfo('source/data.txt')
            member.size = 6
            archive.addfile(member, io.BytesIO(b'pinned'))
        data = contents.getvalue()
        pin = {'url': 'https://example.invalid/source.tar.gz',
               'sha256': hashlib.sha256(data).hexdigest(), 'root': 'source'}
        return pin, data

    def test_transient_failure_recovers_and_cache_needs_no_network(self):
        pin, data = self.source_fixture()
        failures = [builder.urllib.error.URLError(socket.gaierror(socket.EAI_AGAIN, 'DNS unavailable')),
                    builder.urllib.error.URLError(socket.gaierror(socket.EAI_NONAME, 'name not known')),
                    builder.urllib.error.URLError(TimeoutError('connect timed out')),
                    TimeoutError('read timed out'), ConnectionResetError('connection reset')]
        failures += [builder.urllib.error.HTTPError(pin['url'], code, 'temporary response', {}, None)
                     for code in (408, 429, 500, 502, 503, 504)]
        for failure in failures:
            if isinstance(failure, builder.urllib.error.HTTPError):
                self.addCleanup(failure.close)
            with self.subTest(failure=str(failure)), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                errors = io.StringIO()
                with patch.object(builder.urllib.request, 'urlopen',
                                  side_effect=[failure, io.BytesIO(data)]) as download, \
                        patch.object(builder.time, 'sleep') as sleep, patch.object(builder.sys, 'stderr', errors):
                    source = builder.extract(pin, root / 'unpacked', root)
                self.assertEqual((source / 'data.txt').read_bytes(), b'pinned')
                self.assertEqual((root / 'source.tar.gz').read_bytes(), data)
                self.assertEqual(download.call_args_list, [call(pin['url'], timeout=60)] * 2)
                sleep.assert_called_once_with(1)
                self.assertIn('attempt 1/3 failed', errors.getvalue())
                self.assertIn(pin['url'], errors.getvalue())
                if isinstance(failure, builder.urllib.error.HTTPError):
                    self.assertTrue(failure.closed)
                with patch.object(builder.urllib.request, 'urlopen') as cached_download:
                    cached = builder.extract(pin, root / 'cached', root)
                cached_download.assert_not_called()
                self.assertEqual((cached / 'data.txt').read_bytes(), b'pinned')

    def test_exhausted_dns_failures_remain_failed_without_cache(self):
        pin, _ = self.source_fixture()
        failure = builder.urllib.error.URLError(socket.gaierror(socket.EAI_AGAIN, 'DNS unavailable'))
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            errors = io.StringIO()
            with patch.object(builder.urllib.request, 'urlopen', side_effect=failure) as download, \
                    patch.object(builder.time, 'sleep') as sleep, patch.object(builder.sys, 'stderr', errors):
                with self.assertRaises(builder.urllib.error.URLError) as raised:
                    builder.extract(pin, root / 'unpacked', root)
            self.assertIs(raised.exception, failure)
            self.assertEqual(download.call_count, 3)
            self.assertEqual(sleep.call_args_list, [call(1), call(2)])
            self.assertEqual(errors.getvalue().count('failed:'), 3)
            self.assertIn('attempt 3/3 failed', errors.getvalue())
            self.assertEqual(list(root.iterdir()), [])

    def test_permanent_failures_are_not_retried(self):
        pin, _ = self.source_fixture()
        certificate = ssl.SSLCertVerificationError(1, 'certificate verify failed')
        failures = [certificate, builder.urllib.error.URLError(certificate),
                    builder.urllib.error.URLError('unclassified failure')]
        failures += [builder.urllib.error.HTTPError(pin['url'], code, 'permanent response', {}, None)
                     for code in (400, 401, 403, 404, 410, 501)]
        for failure in failures:
            if isinstance(failure, builder.urllib.error.HTTPError):
                self.addCleanup(failure.close)
            with self.subTest(failure=str(failure)), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                with patch.object(builder.urllib.request, 'urlopen', side_effect=failure) as download, \
                        patch.object(builder.time, 'sleep') as sleep, patch.object(builder.sys, 'stderr', io.StringIO()):
                    with self.assertRaises(OSError) as raised:
                        builder.extract(pin, root / 'unpacked', root)
                self.assertIs(raised.exception, failure)
                download.assert_called_once_with(pin['url'], timeout=60)
                sleep.assert_not_called()
                self.assertEqual(list(root.iterdir()), [])

    def test_partial_read_restarts_without_caching_partial_bytes(self):
        class InterruptedBody(io.BytesIO):
            def read(self, size):
                super().read(min(size, 4))
                raise ConnectionResetError('body interrupted')

        pin, data = self.source_fixture()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            partial = InterruptedBody(data)
            with patch.object(builder.urllib.request, 'urlopen',
                              side_effect=[partial, io.BytesIO(data)]) as download, \
                    patch.object(builder.time, 'sleep'), patch.object(builder.sys, 'stderr', io.StringIO()):
                builder.extract(pin, root / 'unpacked', root)
            self.assertTrue(partial.closed)
            self.assertEqual(download.call_count, 2)
            self.assertEqual((root / 'source.tar.gz').read_bytes(), data)

    def test_oversized_download_is_not_retried_or_cached_even_with_matching_hash(self):
        pin, data = self.source_fixture()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            with patch.object(builder, 'SOURCE_MAX_BYTES', len(data) - 1), \
                    patch.object(builder.urllib.request, 'urlopen', return_value=io.BytesIO(data)) as download, \
                    patch.object(builder.time, 'sleep') as sleep:
                with self.assertRaisesRegex(ValueError, 'checksum or size differs'):
                    builder.extract(pin, root / 'unpacked', root)
            download.assert_called_once_with(pin['url'], timeout=60)
            sleep.assert_not_called()
            self.assertEqual(list(root.iterdir()), [])

    def test_invalid_download_reports_identity_without_caching_it(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            rejected = b'upstream response is not the pinned archive\n'
            pin = {'url': 'https://example.invalid/source.tar.gz', 'sha256': '0' * 64}
            with patch.object(builder.urllib.request, 'urlopen', return_value=io.BytesIO(rejected)) as download, \
                    patch.object(builder.time, 'sleep') as sleep:
                with self.assertRaises(ValueError) as error:
                    builder.extract(pin, root / 'unpacked', root)
            download.assert_called_once_with(pin['url'], timeout=60)
            sleep.assert_not_called()
            message = str(error.exception)
            for expected in (pin['url'], 'download', str(len(rejected)) + ' bytes',
                             hashlib.sha256(rejected).hexdigest(), pin['sha256']):
                self.assertIn(expected, message)
            self.assertEqual(list(root.iterdir()), [])

    def test_invalid_cache_is_preserved_and_identified(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / 'source.tar.gz'
            rejected = b'invalid cached archive\n'
            archive.write_bytes(rejected)
            pin = {'url': 'https://example.invalid/source.tar.gz', 'sha256': '0' * 64}
            with patch.object(builder.urllib.request, 'urlopen') as download:
                with self.assertRaises(ValueError) as error:
                    builder.extract(pin, root / 'unpacked', root)
            self.assertIn('cache', str(error.exception))
            self.assertIn(hashlib.sha256(rejected).hexdigest(), str(error.exception))
            download.assert_not_called()
            self.assertEqual(archive.read_bytes(), rejected)

    def test_host_header_cannot_replace_qualified_headers(self):
        with tempfile.TemporaryDirectory(prefix='oidc header fixture ') as temporary:
            root = Path(temporary)
            prefix = root / 'qualified'
            ambient = root / 'ambient'
            for directory in (prefix / 'include', ambient):
                (directory / 'openssl').mkdir(parents=True)
                (directory / 'openssl/probe.h').write_text('#define OIDC_FIXTURE 1\n')
            (root / 'vtls').mkdir()
            (root / 'vtls/openssl.c').write_text('#include <openssl/probe.h>\nint fixture = OIDC_FIXTURE;\n')
            (root / 'CMakeLists.txt').write_text('''cmake_minimum_required(VERSION 3.16)
project(header_fixture C)
add_library(pinned INTERFACE IMPORTED)
set_target_properties(pinned PROPERTIES INTERFACE_INCLUDE_DIRECTORIES "${CMAKE_CURRENT_SOURCE_DIR}/qualified/include")
add_library(fixture OBJECT vtls/openssl.c)
target_link_libraries(fixture PRIVATE pinned)
''')
            environment = dict(os.environ, CPATH=str(ambient))
            for system in ('ON', 'OFF'):
                build = root / system
                subprocess.run(['cmake', '-S', str(root), '-B', str(build),
                                '-DCMAKE_EXPORT_COMPILE_COMMANDS=ON',
                                '-DCMAKE_NO_SYSTEM_FROM_IMPORTED=' + system], env=environment,
                               check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)
                if system == 'ON':
                    self.assertEqual(builder.verify_openssl_headers(build, prefix, environment), 1)
                else:
                    with self.assertRaisesRegex(ValueError, 'unqualified OpenSSL header'):
                        builder.verify_openssl_headers(build, prefix, environment)


if __name__ == '__main__':
    unittest.main()
