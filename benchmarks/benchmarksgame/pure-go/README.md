# Daily Pure Go comparison sources

These three unmodified files are the Go entries already registered in
`../context-sources.tsv`, from Benchmarks Game revision
`40296663ed350d5fe4a6ab5e367bab61cb77c219`.

Archive: https://salsa.debian.org/benchmarksgame-team/benchmarksgame/-/raw/40296663ed350d5fe4a6ab5e367bab61cb77c219/public/download/benchmarksgame-sourcecode.zip

Archive SHA-256: `aabcf6726cdc14f0f45b99e5daba48584f94bbb48883fd3711a1d040474d1cb4`.
Individual SHA-256 values are verified against the existing manifest by the daily
tooling tests. Keep upstream bytes and contributor comments unchanged.

The revised BSD notices are retained in `../licenses/fannkuch-nbody.txt` and
`../licenses/spectral-norm.txt`. Daily evidence includes these notices, sources,
commands and outputs. The programs compile individually, not as one Go package.

Daily measurements use smaller inputs than the formal comparison and one logical
CPU. These hand-written implementations provide an external reference; differences
from TypeRB also reflect program structure and optimization choices.
