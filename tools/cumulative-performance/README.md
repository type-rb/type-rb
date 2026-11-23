# Cumulative performance qualification

This manual workflow answers the original-baseline comparison in
[Task #730](https://github.com/type-rb/type-rb-native/issues/730). Its registration
freezes the accepted candidate, original comparison and daily suite digest.
The `previous` role means the original comparison here. The frozen Native,
TypeRB-generated Go and Pure Go roles keep their daily definitions.

The selector rejects abbreviated/non-commit identities, unrelated history and
stale production or measurement inputs. It checks that the five registered
String/adjacent sources and expected outputs are unchanged, and records both
reference pins and the exact workflow/controller identities. The workflow uses
the candidate's unchanged ordinary compiler preparation and measurement code.
Neither daily nor weekly state is restored, written or replaced.

Dispatch **once**, after recording the final tool revision on the task. The
budget is 67 case/role rows, 268 application builds, 402 application executions,
one compiler IR observation and three self-builds: 674 timed observations.
Existing GC probes are separate. Preparation must reproduce all compiler chains
before timing. Failed, missing or malformed observations remain invalid evidence;
successful observations with an unmet target remain an unmet result. Do not
repeat an unchanged failure or change the baseline to obtain a pass. A repair
requires a recorded new revision and preserved earlier evidence. Actions reruns
are disabled; a new dispatch still requires an explicit registered budget.

The assessment recomputes all retained medians, checks output/source identities,
role coverage, sample counts, command bounds and finite metrics, then applies
the existing scanning conditions: Native/Pure Go wall <=3, current/original wall
and CPU <=0.8, and RSS <=1.1. All adjacent rows, build costs, binary identities,
GC reports and current-source compiler costs remain in the separate artifact.
Compiler self-build on a different historical host is not a causal comparison.
Passing this workflow alone does not establish all initiative completion
criteria; correctness, managed lifetimes and integration retain their own proof.

Run the focused controls with:

```sh
python3 -m unittest discover -s tools/cumulative-performance -p 'test_*.py'
```
