# Unicode IDNA conformance data

`IdnaTestV2.txt` is the unchanged Unicode 17.0.0 corpus from
<https://www.unicode.org/Public/idna/17.0.0/IdnaTestV2.txt>.
Its SHA-256 is
`beb5d0be20e896189b03209a82fdc34f06351502bbd4b8e2523583fc2954d9cf`.
The adjacent Unicode license applies to the data. Preserve its bytes,
including comment whitespace, when reviewing provider changes.

The helper compares all 6,391 non-transitional ToASCII cases with strict
STD3, hyphen, bidi, joiner and DNS-length validation. Successful output and
rejection must match; partial output and error wording for rejected inputs
are not portable contracts. Transport-specific behavior, including the
permitted terminal root dot, is tested separately.
