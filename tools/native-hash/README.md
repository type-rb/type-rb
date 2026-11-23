# Hash measurement tools

These tools run the bounded Darwin arm64 diagnostics registered in
[the retained report](../../results/2026-09-10-native-hash-diagnostic/README.md).
They do not change compiler cost limits or establish acceptance.

Build the candidate and accepted control with their recorded seeds/toolchains,
and provide the frozen source and core separately:

```sh
python3 tools/native-hash/measure.py \
  --candidate /path/to/candidate \
  --control /path/to/control \
  --frozen-source /path/to/frozen-source \
  --frozen-core /path/to/frozen-core \
  --output /path/to/new-initial.json

python3 tools/native-hash/compare-repl.py \
  --control /path/to/control-trbn \
  --candidate /path/to/candidate-trbn \
  --inputs results/2026-09-10-native-hash-diagnostic/initial.json \
  --output /path/to/new-paired.json
```

Each run checks output, retains warmups and measured observations, and reports
five-run medians. Source identities and environmental conditions must accompany
new output. Do not overwrite historical evidence or infer a tuning budget from
the existence of these scripts. Maximum RSS uses Darwin byte units; other hosts
are rejected explicitly.
