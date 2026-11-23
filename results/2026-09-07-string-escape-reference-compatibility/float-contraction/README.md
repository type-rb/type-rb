# Float contraction reference compatibility

The selected reference is TypeRB `0.4.12-dev` at
`e3cdd5e26a57f2389d8b40dd4eafd32c608d6728`.
[Reference validation](https://github.com/type-rb/type-rb/actions/runs/39478996517)
passed after [PR #843](https://github.com/type-rb/type-rb/pull/843) merged.
This is an exact development revision, not a new release or a supported range.

The reference permits ordinary Float multiplication followed by addition or
subtraction to use a fused operation, including through a temporary. It preserves
evaluation order and independently observed rounded products, and does not allow
general reassociation. The shared conformance case accepts both permitted
rounding outcomes and checks overflow, signed zero, nonfinite values, separately
observed products, evaluation effects and forbidden reassociation. The Native
implementation may continue to round each operation separately.

The update also incorporates the declaration-root identity fixes from
[PR #840](https://github.com/type-rb/type-rb/pull/840), the subsequent version
updates, and rejection of incomplete expression parses from PR #843. Package
integration fixes do not imply new Native ORM or web-provider support. The
official source bundle remains tool-generated from the exact pinned Git objects.

The Native baseline is `df30f715a2bdad76662d391ec65f88618f003c1d` with successful
[Main validation](https://github.com/type-rb/type-rb-native/actions/runs/39464122658).
That run validates the preceding pin. Fresh exact-pin observations and acceptance
belong to this update's PR validation and subsequent Main validation.

Local preflight passes with the exact reference executable built through
`tools/build-reference.py`. The reference AST check and all 1,826 current language
cases pass: 1,806 unchanged cases retain their exact-reference observations, and
20 cases added by the current baseline pass a separate delta check. Together
these cover 6,669 command outcomes without changing expectations. The registry's
38 parity gaps and 26 uncovered contract groups remain recorded; passing this
oracle check does not close those gaps.

The independent Native `0.1.0-dev` identity, backend, immutable bootstrap assets
and historical measurements retain their existing identities. This update does
not claim a performance qualification or complete language coverage.
