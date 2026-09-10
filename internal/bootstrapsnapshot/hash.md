# Internal Hash snapshot subset

Version 4 can encode `Hash<String, Integer>` values in parameters, results,
record fields and closure captures. This is an internal, unstable data-only
subset of the checked language; it does not define a runtime layout or add a
language API.

The type definition is `kind: hash`, canonical `id: Hash<String, Integer>`,
`key: String`, and `element: Integer`. Instructions retain typed value identities
and source origins:

- `hash_construct` carries parallel ordered `keys` and `values` lists of equal
  length, including empty lists. Each key then value is evaluated in source
  order before construction; later duplicate keys replace earlier values.
- `hash_set` carries `hash`, `key`, and `value` operands. Receiver and key are
  evaluated once before the right-hand side; insertion uses the resulting
  current collection state.
- `hash_get` implements both required indexing and `fetch`, with a runtime
  failure for an absent key.
- `hash_contains` implements `key?` with a Boolean result.

Other key/value types and operations remain explicitly unsupported by this
snapshot format even when the ordinary language accepts them. Earlier snapshot
versions do not gain Hash support. No executable implementation is embedded in
the snapshot.
