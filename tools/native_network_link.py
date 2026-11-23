"""Link arguments for Web acceptance probes that invoke cc after GC injection."""
import os
from pathlib import Path
import platform


def static_network_flags(qbe):
    directory = Path(os.environ.get('TRBN_NETWORK_LIB_DIR') or Path(qbe).resolve().parent / 'network').resolve()
    flags = [str(directory / name) for name in ('libuv.a', 'libllhttp.a', 'network-identity.o')]
    if platform.system() == 'Linux':
        flags += ['-pthread', '-ldl', '-lrt']
    return flags
