#!/bin/sh

set -eu

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
workspace=$(mktemp -d "${TMPDIR:-/tmp}/type-rb-native-rss-test.XXXXXX")
trap 'rm -rf "$workspace"' EXIT HUP INT TERM

cat > "$workspace/samples.csv" <<'EOF'
elapsed_seconds,rss_bytes,completed_phases
0,10485760,0
1,10551296,1
2,10616832,2
3,10682368,3
4,10747904,4
5,10813440,5
6,10878976,6
7,10944512,7
8,11010048,8
9,11075584,9
10,11141120,10
11,11206656,11
12,11272192,12
13,11337728,13
14,11403264,14
15,11468800,15
EOF

cat > "$workspace/expected.txt" <<'EOF'
sample_count=16
post_warmup_sample_count=10
maximum_rss_bytes=11468800
first_quartile_median_rss_bytes=10911744
last_quartile_median_rss_bytes=11436032
quartile_median_growth_bytes=524288
fitted_rss_slope_bytes_per_minute=3932160.000000
EOF

awk -f "$script_directory/analyze-rss.awk" "$workspace/samples.csv" > "$workspace/actual.txt"
cmp "$workspace/expected.txt" "$workspace/actual.txt"
printf 'runtime-memory-rss-analysis: passed\n'
