#!/usr/bin/env python3
"""Black-box REPL screen checks. Optional reference runs the shared scenarios.

Install tools/repl-test-requirements.txt into a test environment first.
"""
import argparse
import codecs
import errno
import fcntl
import json
import os
from pathlib import Path
import re
import select
import signal
import struct
import sys
import tempfile
import termios
import time

import pyte

ANSI = re.compile(r'\x1b\[[0-?]*[ -/]*[@-~]')


class Terminal:
    def __init__(self, binary, root, reference=False, color=True, term='xterm-256color', stderr_path=None):
        self.master, slave = os.openpty()
        self.original = termios.tcgetattr(slave)
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 24, 90, 0, 0))
        self.screen = pyte.Screen(90, 24)
        self.screen.write_process_input = lambda data: os.write(self.master, data.encode())
        self.stream = pyte.Stream(self.screen)
        self.decoder = codecs.getincrementaldecoder('utf8')('replace')
        self.raw = ''
        self.pid = os.fork()
        if self.pid == 0:
            os.close(self.master)
            os.setsid()
            fcntl.ioctl(slave, termios.TIOCSCTTY, 0)
            for descriptor in (0, 1, 2):
                os.dup2(slave, descriptor)
            os.close(slave)
            if stderr_path is not None:
                with open(stderr_path, 'wb') as errors:
                    os.dup2(errors.fileno(), 2)
            os.chdir(root)
            env = dict(os.environ, TERM=term, LC_ALL='en_US.UTF-8' if sys.platform == 'darwin' else 'C.UTF-8',
                       TRBN_HISTORY=str(root / 'history.json'),
                       XDG_CACHE_HOME=str(root / 'cache'))
            env.pop('NO_COLOR', None)
            if not color:
                env['NO_COLOR'] = '1'
            os.execve(binary, [str(binary), 'repl'], env)
        os.close(slave)
        self.prompt = 'trb:go> ' if reference else 'trbn:trb> '
        try:
            self.wait(lambda: self.prompt in self.display, 'initial prompt')
        except BaseException:
            self.close()
            raise

    @property
    def display(self):
        return '\n'.join(self.screen.display)

    def pump(self, duration=0.1):
        deadline = time.monotonic() + duration
        while time.monotonic() < deadline:
            if not select.select([self.master], [], [], 0.02)[0]:
                continue
            try:
                chunk = os.read(self.master, 65536)
            except OSError as error:
                if error.errno == errno.EIO:
                    return
                raise
            if not chunk:
                return
            text = self.decoder.decode(chunk)
            self.raw += text
            self.stream.feed(text)

    def wait(self, predicate, description, timeout=12):
        deadline = time.monotonic() + timeout
        while not predicate():
            assert time.monotonic() < deadline, (description, self.display, self.raw[-1500:])
            self.pump()
        self.pump(0.04)

    def send(self, text):
        os.write(self.master, text.encode())
        self.pump()

    def evaluate(self, text, expected):
        offset = len(self.raw)
        self.send(text + '\r')
        self.wait(lambda: expected in ANSI.sub('', self.raw[offset:]), expected)

    def finish(self):
        self.send(':quit\r')
        self.exited(0)

    def exited(self, expected):
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            pid, status = os.waitpid(self.pid, os.WNOHANG)
            if pid:
                self.pid = 0
                assert os.waitstatus_to_exitcode(status) == expected
                assert termios.tcgetattr(self.master) == self.original, 'terminal modes not restored'
                return
            self.pump()
        raise AssertionError('REPL did not exit')

    def close(self):
        if self.pid:
            os.kill(self.pid, signal.SIGKILL)
            os.waitpid(self.pid, 0)
        os.close(self.master)


def shared_scenarios(binary, reference=False):
    with tempfile.TemporaryDirectory(prefix='native-repl-editor-') as directory:
        root = Path(directory)
        terminal = Terminal(binary, root, reference)
        try:
            terminal.send('mut apples := 4')
            terminal.wait(lambda: 'mut apples := 4' in terminal.display, 'live input')
            line = terminal.screen.buffer[terminal.screen.cursor.y]
            start = terminal.screen.cursor.x - len('mut apples := 4')
            assert line[start].fg == 'c678dd', ('keyword color', line[start])
            assert line[start + 14].fg == 'ffb86c', ('number color', line[start + 14])
            terminal.evaluate('', '4 : Integer')
            terminal.evaluate('app\t', '4 : Integer')
            terminal.evaluate('apples\x1b[D\x1b[D\x1b[D\t', '4 : Integer')
            terminal.evaluate('\x1b[A', '4 : Integer')

            terminal.send('if true\r')
            terminal.wait(lambda: terminal.screen.cursor.x >= 7, 'auto indent')
            terminal.send('apples + 1\r')
            terminal.send('end')
            # Move to the preceding line and edit it before submitting the block.
            terminal.send('\x1b[A\x05\x7f2\x1b[B\x05')
            terminal.evaluate('', '6 : Integer')

            terminal.send('\x1b[200~puts("paste once")\n1 + 2\x1b[201~')
            terminal.pump(0.2)
            # Pasting a newline cannot execute the buffer before Enter.
            assert not re.search(r'^paste once\r?$', ANSI.sub('', terminal.raw), re.M)
            terminal.evaluate('', '3 : Integer')
            assert len(re.findall(r'^paste once\r?$', ANSI.sub('', terminal.raw), re.M)) == 1, repr(terminal.raw[-2400:])

            terminal.send('apples +')
            terminal.send('\x03')
            terminal.evaluate('apples', '4 : Integer')
            terminal.finish()
        finally:
            terminal.close()

        (root / 'src').mkdir()
        config = {'name': 'sample', 'sourceDir': 'src'}
        if reference:
            config.update(mode='go', go={'module': 'example.com/sample'})
        (root / 'trbconfig.jsonc').write_text(json.dumps(config))
        (root / 'src/main.trb').write_text('def answer(): Integer\nreturn 42\nend\ndef main()\nreturn\nend\n')
        terminal = Terminal(binary, root, reference)
        try:
            terminal.send('ans\t')
            terminal.wait(lambda: 'answer()' in terminal.display, 'zero-argument function completion')
            terminal.send('\x03')  # Leave the reference completion menu, if active.
            terminal.send('\x03')  # Cancel the remaining input buffer.
            terminal.finish()
        finally:
            terminal.close()


def native_scenarios(binary):
    with tempfile.TemporaryDirectory(prefix='native-repl-detail-') as directory:
        root = Path(directory)
        terminal = Terminal(binary, root)
        try:
            terminal.send('\x1b[3~')  # Delete on an empty buffer must not mean EOF.
            terminal.evaluate('mut numbers := [1, 2]', '[1, 2] : Array<Integer>')
            terminal.evaluate('mut count:Int\t := 3', '3 : Integer')
            terminal.evaluate('numbers.size()\x1b[D\x1b[D\t', '2 : Integer')
            terminal.send('numbers.\t')
            terminal.wait(lambda: 'size(): Integer' in terminal.display, 'typed completion detail')
            terminal.send('\r')  # Accept the selected completion without evaluating it.
            terminal.evaluate('', '2 : Integer')
            terminal.send('numbers.\t\t')
            terminal.wait(lambda: 'push(value)' in terminal.display, 'cycle completion')
            terminal.send('\x03')
            terminal.send('record Point\rx: Integer\rend\r')
            terminal.pump()
            terminal.evaluate('mut point := Point.new(x: 3)', 'Point(x: 3) : Point')
            terminal.evaluate('point.x\t', '3 : Integer')

            terminal.send('\x12numbers')
            terminal.wait(lambda: 'reverse-search: numbers' in terminal.display, 'history search')
            terminal.send('\r')
            terminal.evaluate('', '2 : Integer')
            terminal.send('draft\x12absent\x07')
            terminal.wait(lambda: 'draft' in terminal.display, 'search cancellation restores draft')
            terminal.send('\x03')

            terminal.evaluate('puts("こんにちは😀")', 'こんにちは😀')
            terminal.evaluate('"Aあ😀"[1]', '"あ" : String')
            terminal.send('# 日本e\u0301')
            terminal.wait(lambda: '# 日本é' in terminal.display or '# 日本é' in terminal.display,
                          'Unicode display')
            assert terminal.screen.cursor.x == 17, ('Unicode cells', terminal.screen.cursor.x)
            terminal.send('\x7f')  # Delete base plus combining mark in one edit.
            assert terminal.screen.cursor.x == 16
            terminal.send('\x7f')  # Delete a wide glyph without deleting half of its UTF-8.
            assert terminal.screen.cursor.x == 14
            terminal.send('\x03')
            terminal.send('123\x7f\x1f')  # Undo deletion.
            terminal.evaluate('', '123 : Integer')

            terminal.send('# ' + 'x' * 130)
            terminal.wait(lambda: terminal.screen.cursor.y > 0, 'wrapped input')
            fcntl.ioctl(terminal.master, termios.TIOCSWINSZ, struct.pack('HHHH', 18, 48, 0, 0))
            terminal.screen.resize(18, 48)
            os.kill(terminal.pid, signal.SIGWINCH)
            terminal.pump(0.4)
            terminal.send('\x01\x0b7\r')
            terminal.wait(lambda: '7 : Integer' in terminal.display, 'editing after resize')
            terminal.send('apples +')
            os.kill(terminal.pid, signal.SIGINT)
            terminal.pump()
            terminal.evaluate('missing_diagnostic_name', 'error[TRBN4003]: unresolved local missing_diagnostic_name')
            assert ': \x1b[38;2;255;88;116merror\x1b[0m[TRBN4003]:' in terminal.raw
            assert '\x1b[38;2;255;88;116m(trb):' not in terminal.raw
            terminal.evaluate('1 + 2', '3 : Integer')
            terminal.send('mut unfinished := "abc')
            terminal.wait(lambda: terminal.screen.buffer[terminal.screen.cursor.y][terminal.screen.cursor.x - 1].underscore,
                          'unfinished string highlighting')
            terminal.send('\x03')
            terminal.send('\x1b[200~puts("oversized must not execute")\n# ' + 'x' * 65536 + '\x1b[201~')
            terminal.wait(lambda: 'submission discarded' in terminal.display, 'oversized paste rejection', timeout=30)
            assert not re.search(r'^oversized must not execute\r?$', ANSI.sub('', terminal.raw), re.M)
            terminal.evaluate('1 + 2', '3 : Integer')
            terminal.finish()
            history = json.loads((root / 'history.json').read_text())
            assert ':quit' not in history
            assert all('\x1b' not in entry for entry in history)
        finally:
            terminal.close()
        terminal = Terminal(binary, root, color=False)
        try:
            terminal.send('1 + 2')
            terminal.wait(lambda: '1 + 2' in terminal.display, 'uncolored input')
            assert '\x1b[38;' not in terminal.raw and '\x1b[1;38;' not in terminal.raw
            terminal.evaluate('', '3 : Integer')
            terminal.evaluate('missing_diagnostic_name', '(trb):2: error[TRBN4003]:')
            assert '\x1b[38;' not in terminal.raw and '\x1b[1;38;' not in terminal.raw
            terminal.finish()
        finally:
            terminal.close()

        terminal = Terminal(binary, root)
        try:
            terminal.send('incomplete input')
            os.kill(terminal.pid, signal.SIGTERM)
            terminal.exited(143)
        finally:
            terminal.close()


def interpolation_scenarios(binary):
    with tempfile.TemporaryDirectory(prefix='native-repl-interpolation-') as directory:
        root = Path(directory)
        for color in (True, False):
            terminal = Terminal(binary, root, color=color)
            try:
                if not color:
                    terminal.send('\x1b[A')
                    terminal.wait(lambda: r'puts("literal \#{missing}")' in terminal.display,
                                  'escaped interpolation history round trip')
                    terminal.send('\x03')
                terminal.evaluate('s := "abc"', '"abc" : String')
                source = 'puts("hello #{s}")'
                terminal.send(source)
                terminal.wait(lambda: source in terminal.display, 'interpolation input')
                line = terminal.screen.buffer[terminal.screen.cursor.y]
                start = terminal.screen.cursor.x - len(source)
                if color:
                    assert line[start + 6].fg == 'c5e478', ('literal text', line[start + 6])
                    assert line[start + 12].fg == 'c678dd', ('interpolation delimiter', line[start + 12])
                    assert line[start + 14].fg != line[start + 6].fg, ('embedded expression', line[start + 14])
                else:
                    assert '\x1b[38;' not in terminal.raw and '\x1b[1;38;' not in terminal.raw
                terminal.evaluate('', 'hello abc')
                terminal.send('puts("hello #{s')
                terminal.send('}")')
                terminal.evaluate('', 'hello abc')
                terminal.evaluate('puts("#{"nested"}")', 'nested')
                source = r'puts("literal \#{missing}")'
                terminal.send(source)
                terminal.wait(lambda: source in terminal.display, 'escaped interpolation input')
                if color:
                    line = terminal.screen.buffer[terminal.screen.cursor.y]
                    start = terminal.screen.cursor.x - len(source)
                    assert line[start + 6].fg == line[start + source.index('#')].fg
                    assert line[start + 6].fg == line[start + source.index('missing')].fg
                terminal.evaluate('', 'literal #{missing}')
                terminal.finish()
            finally:
                terminal.close()


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('binary', type=lambda path: Path(path).resolve())
    parser.add_argument('--reference', type=lambda path: Path(path).resolve())
    arguments = parser.parse_args()
    shared_scenarios(arguments.binary)
    native_scenarios(arguments.binary)
    interpolation_scenarios(arguments.binary)
    if arguments.reference:
        shared_scenarios(arguments.reference, reference=True)
    print('REPL screen, editing, completion, Unicode and terminal restoration checks passed')
