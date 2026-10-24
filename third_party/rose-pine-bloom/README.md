# Rosé Pine Bloom attribution

The graph palettes in
`desktopexporter/internal/frontend/src/themes/series-palette.json` use unmodified
shade values from [Rosé Pine Bloom](https://github.com/rose-pine/rose-pine-bloom),
pinned to commit `e728487814c7ac4ccbf099913a56c2fa47bed6e3`.

Bloom is Copyright (c) 2025 Rosé Pine and licensed under MIT. The complete
upstream licence is included here and at
`desktopexporter/internal/frontend/public/licenses/rose-pine-bloom.txt`, which is
included in the embedded frontend bundle.

Each theme uses only its own Iris, Pine, Foam and Gold shades. Entire Love and
Rose families, canonical bases, and shade 50 are excluded, except that Foam 50
replaces Gold 100. The frozen 32-slot sequence retains the relative order of the
remaining colours after removing Rose from the original interleave. It does not
change Bloom's shade generation or the application's interface themes.

Updating the source, membership or order requires a new palette version so saved
identity-to-slot assignments cannot be silently interpreted against a different
table. There is no runtime download or automatic upstream update.
