# Fonts of the Synod player

The font set is closed (decision 118): four families, each under the SIL Open Font License 1.1, each one variable woff2 file with every weight from 400 to 700. The player release ships these files as `synod-player-fonts-<tag>.tar.gz` (the files below plus the licence of each family), listed in `SHA256SUMS`. Unpacked, the folder is what `options.fontBase` points at.

| Family | File | Version | Upstream release | Commit | Weights | Licence |
| --- | --- | --- | --- | --- | --- | --- |
| Inter | `Inter.woff2` | 4.1 | https://github.com/rsms/inter/releases/tag/v4.1 | `e3a3d4c57d5ecc01453a575621882a384c1995a3` | 100-900 (and `opsz`) | SIL OFL 1.1 (`OFL-Inter.txt`) |
| JetBrains Mono | `JetBrainsMono.woff2` | 2.304 | https://github.com/JetBrains/JetBrainsMono/releases/tag/v2.304 | `cd5227bd1f61dff3bbd6c814ceaf7ffd95e947d9` | 100-800 | SIL OFL 1.1 (`OFL-JetBrainsMono.txt`) |
| Source Serif 4 | `SourceSerif4.woff2` | 4.005 | https://github.com/adobe-fonts/source-serif/releases/tag/4.005R | `2823e993c53fca27c5c8749f529b56a5a7c77b6b` | 200-900 (and `opsz`) | SIL OFL 1.1 (`OFL-SourceSerif4.txt`) |
| Shantell Sans | `ShantellSans.woff2` | 1.011 | https://github.com/arrowtype/shantell-sans/releases/tag/1.011 | `68f5eca1490a58973b2a814d3a7d0f61291fd0c9` | 300-800 (and `ital`, `BNCE`, `INFM`, `SPAC`) | SIL OFL 1.1 (`OFL-ShantellSans.txt`) |

## Where each file comes from

The licence text of every family was read from the upstream release and is the SIL Open Font License 1.1 (the `OFL-*.txt` files are copied from it unchanged).

- `Inter.woff2`: `web/InterVariable.woff2` of `Inter-4.1.zip`; `OFL-Inter.txt` is its `LICENSE.txt`.
- `JetBrainsMono.woff2`: upstream publishes the variable font only as TrueType (`fonts/variable/JetBrainsMono[wght].ttf` of `JetBrainsMono-2.304.zip`). It is converted to woff2 with fonttools and brotli (`TTFont.flavor = "woff2"`), without subsetting or other change; `OFL-JetBrainsMono.txt` is its `OFL.txt`.
- `SourceSerif4.woff2`: `source-serif-4.005_WOFF2/VAR/SourceSerif4Variable-Roman.ttf.woff2` of `source-serif-4.005_WOFF2.zip`; `OFL-SourceSerif4.txt` is `LICENSE.md` of tag `4.005R` of the repository.
- `ShantellSans.woff2`: `Web/ShantellSans[BNCE,INFM,SPAC,ital,wght].woff2` of `Shantell_Sans_1.011.zip`; `OFL-ShantellSans.txt` is its `2 - License.txt`.

## SHA-256 of the files

```
693b77d4f32ee9b8bfc995589b5fad5e99adf2832738661f5402f9978429a8e3  Inter.woff2
dd03643f16a3ee8e8e0f8d4071be01611f585c9d73d351b95778027686262877  JetBrainsMono.woff2
940a76eda1388de39d38c8e7a79bf6ea058a387faee0a9f33c8d25c6ba05e1be  SourceSerif4.woff2
1473f076cf7ab16865ba2337841776cd67db4744f7bac3b4c36508a0a083dfa7  ShantellSans.woff2
```

## Not shipped

The xkcd script font is CC BY-NC 3.0, which does not allow the redistribution a Synod host needs; the set stays closed so that a scene draws the same on every machine.
