#!/bin/sh
# Bounded release observer. Compiler processes still execute QBE/CC directly.
set -eu
test "$#" -eq 7 || exit 64
mode=$1
input=$2
qbe=$3
profile=$4
runner=$5
workspace=$6
package=$7
case "$mode" in prepare|verify) ;; *) exit 64 ;; esac
case "$profile/$runner" in darwin-arm64-v0/macos-15|linux-arm64-v0/ubuntu-24.04-arm) ;; *) exit 64 ;; esac
test ! -e "$workspace" && test ! -e "$package" || exit 64
test -x "$input" && test -x "$qbe" || exit 64
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
entry=$root/compiler/src/compiler.trb
target=${profile%-v0}
asset=type-rb-native-bootstrap-$target
mkdir -p "$workspace" "$package" "$workspace/trace"
cc=/usr/bin/cc

build_step() {
    seed=$1
    step=$2
    role=$3
    mkdir -p "$workspace/$step"
    if test "$profile" = linux-arm64-v0; then
        strace -f -e trace=process -o "$workspace/trace/$step.trace" \
            "$seed" build "$entry" --output "$workspace/$step/compiler" --qbe "$qbe" --cc "$cc" --target "$profile"
        /bin/sh "$root/tools/runtime-worker-soak/verify-build-trace.sh" \
            "$workspace/trace/$step.trace" "$seed" "$qbe" "$cc" "$role"
    else
        "$seed" build "$entry" --output "$workspace/$step/compiler" --qbe "$qbe" --cc "$cc" --target "$profile"
        printf 'boundary-only (not a dynamic process trace): %s -> %s, %s\n' "$seed" "$qbe" "$cc" > "$workspace/trace/$step.txt"
    fi
}

seed=$input
input_role=ordinary
if test "$mode" = prepare; then
    build_step "$seed" setup-first setup
    # The first generated compiler still contains the predecessor's runtime.
    # Match the existing worker authority: both compatibility builds are setup;
    # the following B2/B3/B4 builds must each satisfy the ordinary LLD boundary.
    build_step "$workspace/setup-first/compiler" setup-runtime setup
    seed=$workspace/setup-runtime/compiler
    input_role=transition
fi
# Trace each ordinary generation outside measured observations, preserving the
# exact executable that seeds the next build. The existing corpus/measurement
# observer independently rebuilds these outputs below.
for step in b2 b3 b4; do
    build_step "$seed" "$step" ordinary
    seed=$workspace/$step/compiler
done
cmp "$workspace/b2/compiler" "$workspace/b3/compiler"
cmp "$workspace/b3/compiler" "$workspace/b4/compiler"
if test "$mode" = verify; then
    cmp "$input" "$workspace/b4/compiler"
fi
bootstrap_input=$input
if test "$mode" = prepare; then bootstrap_input=$workspace/setup-runtime/compiler; fi
/bin/sh "$root/tools/bootstrap-seed.sh" --mode previous --input "$bootstrap_input" \
    --input-role "$input_role" --qbe "$qbe" --cc "$cc" --profile "$profile" --runner-image "$runner" \
    --workspace "$workspace/authority" --output "$package/$asset" --evidence "$workspace/evidence" \
    --metadata "$package/$target.json" --asset-name "$asset" --repository-root "$root"
python3 "$root/tools/bootstrap-seed-release.py" observations "$workspace/evidence/measurements.csv" "$input_role"
cmp "$workspace/b4/compiler" "$package/$asset"
# Check the syntax that the next compiler-source simplification will use.
for fixture in logical-short-circuit elsif-control elsif-managed loop-transfer-recovery boolean-array-values boolean-array-effects boolean-array-recovery record-array-values record-array-effects record-array-managed record-array-recovery hash-values hash-managed hash-cycles array-iteration-live array-iteration-control array-iteration-managed range-values range-streaming range-extrema range-managed; do
    "$package/$asset" check "$root/compiler/conformance/valid/$fixture.trb"
done
# Execute the same positive and negative module contracts used by ordinary CI.
trace_flag=
if test "$profile" = linux-arm64-v0; then trace_flag=--trace; fi
python3 "$root/tools/bootstrap-module-cycles.py" --compiler "$package/$asset" \
    --qbe "$qbe" --profile "$profile" --evidence "$workspace/evidence/module-cycles" $trace_flag
if test -n "$trace_flag"; then
    for trace in "$workspace/evidence/module-cycles"/*/*.trace; do
        if grep -E 'execve\("[^"]*/(go|trb|sh|bash|dash|zsh)"|compiler-recovery' "$trace" > /dev/null; then
            printf '%s\n' 'module-cycle capability proof launched a forbidden process' >&2
            exit 1
        fi
    done
fi
find "$workspace" -name '*.trbn.*' -print > "$workspace/temporary-inventory.txt"
test ! -s "$workspace/temporary-inventory.txt"
