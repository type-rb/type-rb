# Active evidence only

This directory is a bounded working set, not a permanent experiment archive.
[`active.json`](active.json) lists each retained result's purpose and retirement
condition. Replace an occupied slot and remove its superseded directory in the
same PR. Do not add dated snapshots without an active purpose.

CI limits the whole directory to **512 files / 4 MiB** and ten named slots;
new results also have the per-result limits in the
[retention policy](../docs/evidence-retention.md). Review is required to change
these limits, not merely to make a new snapshot fit.

For past decisions and rejected approaches, see the
[development history](../docs/history.md). The
[pre-retirement snapshot](https://github.com/type-rb/type-rb-native/tree/bd483cfc5b51035d9fac193a74ee9d0eda2419ed/results)
contains the original historical reports and their source-era inventories.
Retiring a result changes neither its original outcome nor a running experiment's
fixed comparison baseline. Published bootstrap releases remain unchanged.
