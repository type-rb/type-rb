# OIDC HTTP/2 and Unicode hostname observations

Implementation source: `e5eb67b76374817167d1585269a6229bea7c0233`; exact Go reference:
`3c92543fe5d3ac984d35d65f62b1ab85fc82cafd`.

The local Darwin arm64 candidate passed ordinary core/CLI fixed points, the
snapshot-v4 bounds, declaration/MIR controls, all 6,391 fixed Unicode 17 provider
cases, 52 actual hostname cases each in Go/Native/stressed Native, and seven
HTTP/2 cases in each path. Existing authentication, trust overrides, forced GC,
allocation failures and relocated static-archive controls also passed. The full
Native CLI suite passed in 391.616 seconds, 426 conformance sources matched, and
eleven recovery source tests passed. The unchanged Web deadline application
scenarios passed in 91.296 seconds. The original twenty-group time-service
contract also passed against both the current reference and Native, including
forced GC. Final preflight passed in 262.749 seconds, including the unchanged
historical project commands replayed against the new reference. Hosted target
checks are recorded in the implementing PR; local results alone do not qualify
immutable release artifacts or four hosts.

## Bounded dependency observation

One three-pair cohort uses the unchanged `PLAIN` and `OIDC` programs in
`tools/native-oidc-distribution-test.py --measure`. Both import Web request
values and print the same invalid-configuration message; no network request is
performed. This measures the whole selected OIDC feature relative to Web values,
not the isolated cost of ICU or nghttp2. Compiler/library files and build tools
are warm. Executable startup is process launch through exit; the first Web
startup observation is retained. No samples were dropped and no cold-cache or
statistical performance claim is made.

| Observation | Web values | OIDC | Difference |
| --- | ---: | ---: | ---: |
| Executable bytes | 375,624 | 5,771,192 | +5,395,568 |
| Median build seconds, 3 samples | 2.220450 | 4.072928 | +1.852478 |
| Median startup seconds, 3 samples | 0.059110 | 0.118650 | +0.059540 |

In this source-bound cohort, the full CLI was 7,125,816 bytes (+16 versus the retained qualified baseline);
the core compiler was 3,160,264 bytes (+16,528). These are same-host artifact size
observations, not timing comparisons against the preceding compiler. The core
and CLI do not acquire OIDC/ICU linkage. OIDC applications import system libc++,
libSystem and the documented Apple frameworks; crypto/TLS, curl, nghttp2 and ICU
are static, and there is no runtime library-directory or ICU data-file lookup.

[cost.json](cost.json) retains every timing sample, source/binary identities and
linked-library observations. The closed earlier provider/cost comparisons were
not rerun. The current dependency builder completed locally in 250.956 seconds;
that single build duration is not a benchmark or a cold-build guarantee.

## Preserved unsuccessful observations

The first source-built ICU tool preparation lacked the build output directory;
the second lacked the i18n symbols used only by upstream data-packaging tools.
Both were corrected within the registered preparation budget. The initial
Native implementation omitted its owned export declaration, then a
reference-evaluated emitter probe hit its known non-ASCII byte fallback. The
ordinary build similarly required ASCII escape spelling in embedded provider
source; the UTF-8 values and corpus remained unchanged. A diagnostic full
provider check under the reference evaluator could not use its runtime byte
adapter and was retained as a preparation failure; actual Native provider
execution supplies that acceptance. A link-check invocation used an unsupported
receipt option and was corrected without repeating already successful cases.
The first final preflight caught a reference update that had relabeled the
frozen time-service manifest. Its original bytes and source-era reference were
restored; the test report now records the current oracle separately and still
checks every unchanged expected outcome. The current-reference/Native/forced-GC
comparison passed without changing the frozen hash or expectations. A subsequent
preflight found the separate current-pin project-test replay still naming the
old reference; its pin was updated with every scenario and historical manifest
preserved. No failure was counted as a passing application or bootstrap result.

Initial Linux hosted qualification reached the final ICU ABI link but failed
because the standalone probes omitted libm. The Linux builder and probes now
link it explicitly; the application adapter also lists libm after the ICU
archives so it does not depend on an earlier, incidental math reference. The
original job failures remain retained. This link correction does not reopen the
closed Darwin cost cohort or change its source-bound observations.
