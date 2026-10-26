# Third-party notices

The detector's source code and original software documentation are licensed
under the [MIT License](LICENSE).

The executable includes the Go runtime under its BSD-style license. Release
packages reproduce it as `GO-LICENSE`; the source copy is
[release/GO-LICENSE](https://github.com/dlubom/survex-duplicates/blob/main/release/GO-LICENSE),
copied unchanged from [Go 1.26.3](https://github.com/golang/go/blob/go1.26.3/LICENSE).

## Survey data

The complete registry example and survey-derived evidence are separately
licensed under **Creative Commons Attribution-ShareAlike 4.0 International**:

- `examples/kataster/`: the full spatial model, its text dump and derived reports;
- `experiments/tolerance-2026-10-25/`: registry dump, geometric results and survey excerpts;
- `experiments/go-2026-10-26/`: new registry verification summaries and performance measurements;
- `experiments/go-release-2026-10-26/`: new registry parity and package-verification records;
- `benchmarks/kataster-results/`: historical reports derived from the registry.

Attribution: **Tatra Cave Registry contributors**,
[Jaskiniowy-Kataster-Tatr-Zachodnich](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich).
The upstream [license notice](https://github.com/dlubom/Jaskiniowy-Kataster-Tatr-Zachodnich/blob/a41faea66c3c3170a96010ef7314b375875cfced/LICENCE)
is reproduced in [examples/kataster/LICENCE](examples/kataster/LICENCE).
License: <https://creativecommons.org/licenses/by-sa/4.0/>.

The binary model and saved dump are unchanged copies. Reports are generated
analyses, not upstream assertions that the matched surveys are erroneous.
Published historical metadata has had local machine paths replaced with
portable paths; its numerical measurements are unchanged. See the example's
provenance and the experiment's publication provenance record for hashes and details.

Station names and cave names retain their original spelling. Attribution does
not imply that the registry contributors endorse this detector.
