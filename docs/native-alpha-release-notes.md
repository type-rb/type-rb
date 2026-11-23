TypeRB Native is the default implementation in this v0.5 prerelease. The
compiler is available as `trb` for macOS and Linux on Arm64. Keep QBE and the
library sidecars alongside the binary. Native application builds use the
system C toolchain.

The same-version Go companion is available from
[type-rb/type-rb-go](https://github.com/type-rb/type-rb-go/releases) as
`trbg`. Use it for ordinary Go, Ruby, and TypeScript source-output workflows,
including their test runners and Go source debugging. Existing v0.4 releases
remain available for hosts without a Native alpha package.

See the [v0.5 transition](https://github.com/type-rb/type-rb/blob/main/docs/native-transition.md)
for the alpha scope and the final release requirements.
