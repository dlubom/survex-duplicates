# Default relative endpoint tolerance

Decision: 2026-10-25.

Use a configurable tolerance of 5% of the shorter leg by default, with no fixed
centimetre cap. The [full registry experiment](https://github.com/dlubom/survex-duplicates/blob/main/experiments/tolerance-2026-10-25/README.md)
found an acknowledged repeat survey with a 10 cm difference and a repeated
six-leg sequence with model differences up to 13.6 cm. An absolute 5 cm limit
missed these examples.

Relative tolerance permits larger positional differences for long legs and
is stricter for short legs. It is a candidate-search threshold, not proof of
survey identity. Absolute tolerance remains a separate mode; the two limits
are not combined using a minimum or maximum.

SPLAY selection uses its exact flag and preserves original record IDs.
[ADR 0003](0003-exclude-acknowledged-duplicates.md) subsequently excludes
acknowledged DUPLICATE legs by default. Use the explicit audit option when
reproducing the original experiment, including its Mietusia examples.
