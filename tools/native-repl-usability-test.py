#!/usr/bin/env python3
"""Compare accepted editing, diagnostic roles and retained session behavior."""
import argparse
import importlib.util
import json
from pathlib import Path
import tempfile

spec = importlib.util.spec_from_file_location('repl_editor', Path(__file__).with_name('native-repl-editor-test.py'))
editor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(editor)
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('binary', type=lambda value: Path(value).resolve())
parser.add_argument('--reference', type=lambda value: Path(value).resolve())
args = parser.parse_args()

for binary, reference in [(args.binary, False), *([(args.reference, True)] if args.reference else [])]:
    with tempfile.TemporaryDirectory(prefix='repl usability ') as directory:
        root = Path(directory)
        (root / 'trbconfig.jsonc').write_text(json.dumps({
            'name': 'repl-usability', 'mode': 'go' if reference else 'trb',
            'sourceDir': '.', 'go': {'module': 'example.com/repl-usability'}}))
        terminal = editor.Terminal(binary, root, reference=reference)
        try:
            terminal.evaluate('s:=123', '123 : Integer')
            assert 's := 123' in terminal.display, terminal.display
            start = len(terminal.raw)
            terminal.evaluate('s + "aaa"', 'operator + does not support Integer and String')
            raw = terminal.raw[start:]
            assert ': \x1b[38;2;255;88;116merror\x1b[0m[' in raw, repr(raw)
            assert '\x1b[38;2;255;88;116m(trb):' not in raw, repr(raw)
            # The expression origin and operator origin are intentionally
            # distinct; both must be authored, never a fabricated column.
            expected = '(trb):2:1:' if reference else '(trb):2:3:'
            assert expected in editor.ANSI.sub('', raw), repr(raw)
            terminal.evaluate('s+1', '124 : Integer')
            assert 's + 1' in terminal.display, terminal.display
            terminal.evaluate('mut n:=1', '1 : Integer [mut]')
            rendered = [(row, text) for row, text in enumerate(terminal.screen.display)
                        if '1 : Integer [mut]' in text]
            assert rendered, terminal.display
            row, text = rendered[-1]
            cells = terminal.screen.buffer[row]
            assert cells[text.index('[mut]')].fg == 'c678dd'
            assert cells[text.index('[mut]')].bold
            assert cells[text.index('Integer')].fg == '7fdbca'
            assert cells[text.index(':')].fg == '6272a4'
            assert not cells[text.index(':')].italics
            terminal.evaluate('puts("a  b") # keep  spacing', 'a  b')
            terminal.evaluate('puts("literal \\#{value}")', 'literal #{value}')
            terminal.send('\x1b[200~def answer():Integer\nreturn 40+2\nend\x1b[201~\r')
            terminal.wait(lambda: 'return 40 + 2' in terminal.display, 'formatted multiline body')
            terminal.evaluate('answer()', '42 : Integer')
            terminal.finish()
        finally:
            terminal.close()

        if not reference:
            errors = root / 'diagnostics.log'
            terminal = editor.Terminal(binary, root, stderr_path=errors)
            try:
                terminal.evaluate('s:=123', '123 : Integer')
                terminal.send('s+"aaa"\r')
                terminal.wait(lambda: 'operator + does not support Integer and String' in errors.read_text(),
                              'redirected diagnostic')
                terminal.send('1/0\r')
                terminal.wait(lambda: 'division by zero' in errors.read_text(), 'redirected runtime error')
                assert '\x1b' not in errors.read_text(), errors.read_text()
                assert 'operator + does not support' not in editor.ANSI.sub('', terminal.raw)
                terminal.evaluate('s+1', '124 : Integer')
                terminal.finish()
            finally:
                terminal.close()
            for color, term in [(False, 'xterm-256color'), (True, 'dumb')]:
                terminal = editor.Terminal(binary, root, color=color, term=term)
                try:
                    terminal.evaluate('s:=123', '123 : Integer')
                    terminal.evaluate('s+"aaa"', 'operator + does not support Integer and String')
                    assert '\x1b[38;' not in terminal.raw and '\x1b[1;38;' not in terminal.raw
                    if term == 'dumb':
                        assert '\x1b' not in terminal.raw, repr(terminal.raw)
                    terminal.evaluate('s+1', '124 : Integer')
                    terminal.finish()
                finally:
                    terminal.close()
print('REPL canonical acceptance, severity roles, retained state and terminal controls passed')
