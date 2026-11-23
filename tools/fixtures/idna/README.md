# Unicode 17 UTS46 conformance corpus

`IdnaTestV2.txt` is the unchanged official Unicode 17.0.0 corpus from
https://www.unicode.org/Public/idna/17.0.0/IdnaTestV2.txt.
Its SHA-256 is `beb5d0be20e896189b03209a82fdc34f06351502bbd4b8e2523583fc2954d9cf`.
`UNICODE-LICENSE.txt` accompanies the data.

`tools/oidc_idna_corpus.py` compares all strict non-transitional ToASCII outcomes
and rejection statuses. Invalid conversions need not reproduce partial output
or provider-specific error wording. The separate endpoint tests cover terminal
root dots, numeric hostnames, percent decoding, IPv6 and real transport routing.
