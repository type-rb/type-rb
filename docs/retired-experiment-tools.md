# Retired experiment controllers

Completed experiments are removed from the current checkout and CI. Use the
exact registered source revision when reproducing one; its old contract is not
another authority over today's compiler.

- Source-era benchmark controllers (scalar/aggregate/managed recovery, compiler
  reconstruction, file and project entry, module loading, lookup, numeric
  collections and bootstrap closure) remain in the
  [pre-retirement tools inventory](https://github.com/type-rb/type-rb-native/tree/d568d9e712452f66fc9c48d6025bf73b79659661/tools).
- The frozen one-slice runtime A/B workflows for the Array push fast path,
  dynamic Array address and temporary GC root push fast path, the historical
  portable-entry reproduction, and the Native MIR foundation measurement remain
  in the [pre-retirement workflows](https://github.com/type-rb/type-rb-native/tree/04c6ca7263c066fd13e83b3faa09c4e80d707c13/.github/workflows),
  together with the matching controller cases in
  [`tools/native-runtime-ab`](https://github.com/type-rb/type-rb-native/tree/04c6ca7263c066fd13e83b3faa09c4e80d707c13/tools/native-runtime-ab).

- The strict comparative cost contract remains in the
  [pre-retirement tree](https://github.com/type-rb/type-rb-native/tree/d1c151d02e051c6557acb479a9e9d7c713226858): the static String compactness
  workflow and its registered MIR transition tools, the MIR transition size
  markers under `compiler/native-mir-*-v1.txt` with
  `tools/native-mir-transition-policy.sh`, the `native-runtime-ab` workflow and
  controller, the manual formal runtime memory workflow, the manual bootstrap
  seed verification workflow and the generated full language case matrix.

Source, oracle, seed and measurement identities recorded by those experiments
are unchanged. Current recovery, conformance, target and memory checks stay
required.
