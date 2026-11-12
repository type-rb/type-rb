# OIDC hostname providers

OIDC transports use non-transitional UTS #46 with Unicode 17.0.0 mapping
tables. STD3 ASCII, hyphen, bidi, joiner, label and DNS length checks are
enabled; invalid Punycode is rejected. The transport wrapper also accepts a
single terminal DNS root dot. IPv6 literals bypass IDNA. Numeric hosts must
be canonical dotted-decimal IPv4, avoiding backend-dependent shorthand or
octal/hexadecimal URL interpretations.

The providers are:

- Go: `golang.org/x/net/idna v0.62.0`, with `golang.org/x/text v0.43.0`.
- TypeScript: `tr46 6.0.0`, with `punycode 2.3.1`.
- Ruby: the MIT-licensed `uri-idna 0.3.1`, vendored with Unicode 17.0.0
  tables and the two explicit processing corrections below. It is emitted
  only for OIDC applications, under the private `TrbOidcIdna` namespace.
  The generated application needs neither an IDNA gem nor a C library.

`providers.json` fixes the upstream source commit and archive digest.
`unicode-inputs.json` fixes the public Unicode source files by URL, size and
SHA-256. Ruby and Unicode licenses are retained beside the source and included
in generated Ruby. Updates must review upstream changes, regenerate the data,
and rerun the official corpus and actual transport tests; changing a package
version alone is insufficient.

## Ruby changes from upstream

The upstream algorithm and Punycode implementation are retained. Generated
tables replace the upstream Unicode 15.1.0 tables. In `uts46/mapping.rb`, the
post-Unicode-16 STD3 rule is checked after mapping and NFC normalization,
instead of treating `disallowed_STD3_*` as mapping statuses. In
`base_processing.rb`, strict DNS length validation rejects the final empty
label; the transport wrapper handles the explicit root-dot exception.
`Ruby()` orders and embeds the existing relative dependencies and isolates
their namespace; it does not rewrite TypeRB source.

The retained upstream generator reads verified local files rather than
unversioned network data. With a Ruby toolchain using Unicode 17.0.0:

```sh
python3 tools/regenerate-oidc-idna.py
python3 tools/regenerate-oidc-idna.py --write
```

The first command checks byte equality. Temporary Unicode inputs are deleted
on success and failure. The generator's IDNA2008 tables are also retained so
the vendored provider remains internally consistent, although OIDC calls only
non-transitional UTS #46 ToASCII.

`internal/testsupport/idnacorpus` contains Unicode 17's 6,391 official cases
and its license. The generated Go profile, embedded Ruby provider, and
TypeScript provider on Node/Bun must agree on successful outputs and rejection.
Transport tests separately cover root dots, numeric hosts, paths/queries,
redirects, proxies, TLS and unchanged issuer identity. These tests qualify
their stated profile, not an unrelated public URL API or all HTTP clients.

References: [UTS #46](https://www.unicode.org/reports/tr46/),
[uri-idna](https://github.com/skryukov/uri-idna),
[tr46](https://github.com/jsdom/tr46), and
[Go IDNA](https://pkg.go.dev/golang.org/x/net/idna).
