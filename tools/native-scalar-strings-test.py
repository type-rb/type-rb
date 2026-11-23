#!/usr/bin/env python3
"""Verify binary64 conversion boundaries and retained scalar conversion calls."""
import decimal
import json
import math
import os
from pathlib import Path
import random
import struct
import subprocess
import sys
import tempfile

binary = Path(sys.argv[1]).resolve()


def canonical(value):
    if math.isinf(value):
        return '-Infinity' if value < 0 else 'Infinity'
    if value == 0:
        return '0.0'
    # Python's shortest-roundtrip oracle is independent of the runtime's
    # decimal algorithm. Decimal expands its digits without rounding the value.
    text = format(decimal.Decimal(repr(value)), 'f')
    return text if '.' in text else text + '.0'


with tempfile.TemporaryDirectory(prefix='native-scalar-strings-') as temporary:
    root = Path(temporary)
    env = dict(os.environ, NO_COLOR='1', TERM='dumb', TRBN_HISTORY=str(root / 'history'))

    def run(*arguments, text=None):
        result = subprocess.run([str(binary), *map(str, arguments)], input=text,
                                cwd=root, env=env, capture_output=True, text=True,
                                timeout=180)
        assert result.returncode == 0 and not result.stderr, (result.returncode, result.stderr)
        return result.stdout

    # All binades, including subnormals and asymmetric intervals at powers of
    # two. Both neighboring directions expose asymmetric-interval mistakes.
    (root / 'main.trb').write_text('''def main()
mut value := 1.0
mut index := 0
while index < 1074
value = value / 2.0
index += 1
end
index = 0
while index < 2098
values := [value, -value, value * 1.0000000000000002, value * 0.9999999999999999]
values.each do |item|
puts(item.to_s())
end
value = value * 2.0
index += 1
end
end
''')
    expected = []
    for exponent in range(-1074, 1024):
        value = math.ldexp(1, exponent)
        expected.extend(map(canonical, [value, -value, value * 1.0000000000000002,
                                       value * 0.9999999999999999]))
    actual = run('run', root / 'main.trb').splitlines()
    assert len(actual) == len(expected), (len(actual), len(expected))
    for index, (got, wanted) in enumerate(zip(actual, expected)):
        assert got == wanted, (index, got, wanted)
    (root / 'main.trb').unlink()

    # The shared decimal writer must preserve interior 00..99 pairs, leading
    # one- and two-digit groups, signs, and the portable Integer boundaries.
    integers = {0, 9007199254740991, -9007199254740991}
    for exponent in range(1, 16):
        for offset in (-1, 0, 1):
            value = 10 ** exponent + offset
            integers.update((value, -value))
    for pair in range(100):
        for value in (pair, 10001 + pair * 100, 1000001 + pair * 10000):
            integers.update((value, -value))
    integers = sorted(integers)
    (root / 'main.trb').write_text('def main()\nvalues := [' +
                                 ', '.join(map(str, integers)) +
                                 ']\nvalues.each do |value|\nputs(value.to_s())\n'
                                 'puts(value.to_f().to_s())\nend\nend\n')
    actual = run('run', root / 'main.trb').splitlines()
    expected = [text for value in integers for text in (str(value), canonical(float(value)))]
    assert actual == expected, [(i, got, wanted) for i, (got, wanted)
                                in enumerate(zip(actual, expected)) if got != wanted][:8]
    (root / 'main.trb').unlink()

    # Cover the decimal precision transitions and significands beyond powers of
    # two. A fixed seed keeps failures reproducible across the supported hosts.
    values = [float('1.' + '2345678901234567'[:digits]) for digits in range(1, 17)]
    generator = random.Random(0x545242)
    while len(values) < 272:
        value = struct.unpack('>d', generator.getrandbits(64).to_bytes(8, 'big'))[0]
        if math.isfinite(value):
            values.append(value)
    literals = ', '.join(json.dumps(repr(value)) for value in values)
    (root / 'main.trb').write_text('def main()\nvalues := [' + literals +
                                 ']\nvalues.each do |text|\nputs(text.to_f().to_s())\nend\nend\n')
    actual = run('run', root / 'main.trb').splitlines()
    expected = list(map(canonical, values))
    assert len(actual) == len(expected), (len(actual), len(expected))
    for index, (got, wanted) in enumerate(zip(actual, expected)):
        assert got == wanted, (index, repr(values[index]), got, wanted)
    (root / 'main.trb').unlink()

    retained = run('repl', text='''convert := fn(value: Float): String; return value.to_s(); end
saved := convert(0.1)
flag := true.to_s()
record Later
value: Integer
end
puts(saved)
puts(flag)
puts(convert(1.25))
absent: Float? := nil
puts(absent&.to_s() == nil)
:reload
puts(saved)
puts(convert(-2.5))
:quit
''')
    assert retained.count('0.1\n') == 3, retained
    assert retained.count('true\n') == 4, retained
    assert retained.endswith('0.1\n-2.5\n'), retained
    assert 'reloaded\n' in retained, retained

print(f'PASS scalar String conversion: 8,392 binary64 boundaries, 272 significands, '
      f'{len(integers)} paired Integer/Float values, retained callbacks and replay')
