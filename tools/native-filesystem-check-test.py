#!/usr/bin/env python3
"""Reject resource escape and unverified calls independently of runtime coverage."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile


CASES = {
    'forged annotated alias': 'def escape(value: Any)\n_handle: File := value\nend',
    'stored borrow callback': 'def accept(_file: File)\nend\ndef escape()\n_callback := accept\nend',
    'erased borrow callback': 'def accept(_file: File)\nend\ndef escape()\n_callback: Any := accept\nend',
    'interface borrow': 'interface Reader\nread(file: File): String\nend',
    'empty resource array': 'def escape()\n_handles: Array<File> := []\nend',
    'empty generic resource record': 'record Holder<T>\nvalues: Array<T>\nend\ndef escape()\n_holder := Holder<File>.new(values: [])\nend',
    'phantom resource enum': 'enum Holder<T>\nCount(value: Integer)\nend\ndef escape()\n_holder := Holder<File>::Count(1)\nend',
    'phantom resource record': 'record Holder<T>\nvalue: Integer\nend\ndef escape()\n_holder := Holder<File>.new(value: 1)\nend',
    'unused resource type argument': 'def kind<T>(): Integer\nreturn 1\nend\ndef escape(): Integer\nreturn kind<File>()\nend',
    'return through Any': 'def escape(file: File): Any\nreturn file\nend',
    'annotated Any alias': 'def escape(file: File)\nvalue: Any := file\nputs(value)\nreturn\nend',
    'mutable local alias': 'def escape(file: File)\nmut value := file\nputs(value)\nreturn\nend',
    'mutable borrowed parameter': 'def escape(mut file: File)\nputs(file)\nreturn\nend',
    'array storage': 'def escape(file: File): Array<Any>\nreturn [file]\nend',
    'nullable resource parameter': 'def escape(file: File?)\nputs(file)\nreturn\nend',
    'resource return type': 'def escape(file: File): File\nreturn file\nend',
    'resource record field': 'record Holder\nfile: File\nend',
    'transparent alias': 'alias Handle = File\ndef escape(file: Handle)\nputs(file)\nreturn\nend',
    'class field storage': 'class Holder\n@value: Any\ndef initialize(file: File)\n@value = file\nend\nend',
    'constructor minting': 'def escape(): Any\nreturn File.new()\nend',
    'generic erasure': '''def retain<T>(value: T): Any
return value
end
def escape(file: File): Any
return retain<File>(file)
end''',
    'first-class closure capture': '''def escape(file: File): () -> Any
return fn(): Any
return file
end
end''',
    'unknown callback during borrow': '''def escape(_file: File, callback: () -> String): String
return callback()
end''',
    'transitive unknown callback': '''def middle(callback: () -> String): String
return callback()
end
def escape(_file: File, callback: () -> String): String
return middle(callback)
end''',
    'unused generic unknown callback': '''def middle(callback: () -> String): String
return callback()
end
def escape<T>(_file: File, callback: () -> String): String
return middle(callback)
end''',
    'generic helper unknown callback': '''def middle<T>(callback: () -> String): String
return callback()
end
def escape(_file: File, callback: () -> String): String
return middle<Integer>(callback)
end''',
    'opaque constructor': '''class Reader
def initialize(callback: () -> String)
puts(callback())
end
end
def escape(_file: File, callback: () -> String)
_reader := Reader.new(callback)
end''',
    'opaque default': '''def middle(callback: () -> String, value: String = callback()): String
return value
end
def escape(_file: File, callback: () -> String): String
return middle(callback)
end''',
    'transitive suspension': '''def work(): Array<Integer>
return [1].concurrent_map(limit: 1) do |value|
value
end
end
def middle(): Array<Integer>
return work()
end
def escape(_file: File): Array<Integer>
return middle()
end''',
    'suspending default': '''def work(): Integer
values := [1].concurrent_map(limit: 1) do |value|
value
end
return values[0]
end
def middle(value: Integer = work()): Integer
return value
end
def escape(_file: File): Integer
return middle()
end''',
    'suspending record initialization': '''def work(): Integer
values := [1].concurrent_map(limit: 1) do |value|
value
end
return values[0]
end
record Config
value: Integer = work()
end
def escape(_file: File): Config
return Config.new()
end''',
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--reference', action='store_true')
    parser.add_argument('--report', type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    observations = []
    with tempfile.TemporaryDirectory(prefix='resource-check-') as temporary:
        root = Path(temporary)
        (root / 'src').mkdir()
        config = root / 'trbconfig.jsonc'
        config.write_text(json.dumps({'name': 'resource-check', 'mode': 'go' if args.reference else 'trb',
                                     'sourceDir': 'src', 'outDir': 'out',
                                     'go': {'module': 'example.com/resource-check'}}))
        for name, declarations in CASES.items():
            (root / 'src/main.trb').write_text('import trb/std/file\n' + declarations + '\ndef main()\nreturn\nend\n')
            result = subprocess.run([str(binary), 'check', '--config', str(config)], cwd=root,
                                    env=dict(os.environ, NO_COLOR='1'), text=True,
                                    capture_output=True, timeout=30)
            diagnostics = result.stdout + result.stderr
            expected = ('File cannot be constructed directly',) if name == 'constructor minting' else ('scoped resource', 'scoped File')
            resource_rejection = any(message in diagnostics for message in expected)
            assert result.returncode != 0 and resource_rejection, (name, result)
            observations.append({'case': name, 'code': result.returncode,
                                 'diagnostics': diagnostics.replace(str(root), '<fixture>')})
    report = {'mode': 'reference' if args.reference else 'native', 'rejections': observations,
              'not_exercised': []}
    if args.report:
        args.report.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    main()
