#!/usr/bin/env python3
"""Keep Web.serve's reference REPL rejection and authored names distinct."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('binary', type=Path)
parser.add_argument('--reference', action='store_true')
args = parser.parse_args()
with tempfile.TemporaryDirectory(prefix='web-network-repl-') as temporary:
    root = Path(temporary)
    (root / 'src').mkdir()
    (root / 'src/main.trb').write_text('def main()\nreturn\nend\n')
    config = root / 'trbconfig.jsonc'
    config.write_text(json.dumps({'name': 'network-repl', 'mode': 'go' if args.reference else 'trb', 'sourceDir': 'src',
                                 'go': {'module': 'example.com/network-repl'}}))
    for invocation in ['Web.serve()', 'Web.serve(Web::ServerConfig.new(port: 0))']:
        result = subprocess.run([str(args.binary.resolve()), 'repl', '--config', str(config)],
                                cwd=root, text=True, capture_output=True,
                                input='import trb/web\n' + invocation + '\nputs(7)\n',
                                env=dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history')),
                                timeout=20)
        text = result.stdout + result.stderr
        assert 'trb.web.serve' in text and 'no REPL runtime adapter' in text, text
        assert '7' in result.stdout, text
    # A spelling collision in an authored module is an ordinary function.
    source = 'module Web\ndef self.serve(): Integer\nreturn 42\nend\nend\nWeb.serve()\n'
    result = subprocess.run([str(args.binary.resolve()), 'repl', '--config', str(config)],
                            cwd=root, text=True, capture_output=True, input=source,
                            env=dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history')),
                            timeout=20)
    assert '42' in result.stdout and 'no REPL runtime adapter' not in result.stderr, result
print('Web.serve REPL contract and authored namespace passed')
