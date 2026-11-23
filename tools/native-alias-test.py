#!/usr/bin/env python3
"""Retain canonical alias targets through REPL inputs and explicit replay."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()
for alias_declarations, imports, annotation in (
        ('alias Values<T> = Array<Item>\n',
         'import { Values, Entry } from helper\n', 'Values<Integer>'),
        ('alias Identity<T> = T\nalias Values<T> = Array<T>\n',
         'import { Values, Entry, Identity } from helper\n', 'Identity<Values<Entry>>')):
    with tempfile.TemporaryDirectory(prefix="native alias ") as temporary:
        root = Path(temporary)
        (root / "trbconfig.jsonc").write_text('{"name":"aliases","sourceDir":"src"}')
        (root / "src").mkdir()
        (root / "src/helper.trb").write_text(
            'record Item\ntext: String\nend\nalias Entry = Item\n' +
            alias_declarations)
        text = (imports +
                'record Item\nnumber: Integer\nend\n' +
                'mut values: ' + annotation + ' := []\n'
                'values.push(Entry.new(text: "held"))\n'
                'values[0].text\n:reload\nvalues[0].text\n:q\n')
        result = subprocess.run([str(binary), 'repl'], input=text, text=True,
                                capture_output=True, cwd=root, timeout=30,
                                env=dict(os.environ, NO_COLOR='1', TRBN_HISTORY=str(root / 'history')))
        assert result.returncode == 0 and not result.stderr, result
        assert result.stdout.count('"held" : String\n') == 2, result.stdout
        assert 'reloaded\n' in result.stdout, result.stdout
print('PASS Native transparent alias REPL retention and replay')
