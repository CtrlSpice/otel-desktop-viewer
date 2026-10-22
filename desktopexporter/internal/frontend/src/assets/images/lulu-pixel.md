# Pixel Lulu

The telescope SVG stills use the CC0 pixel artwork from
[CtrlSpice/lulu](https://github.com/CtrlSpice/lulu), pinned at
`a160ebff72687362c9b9ad26c967e4c48893dc53`.

- `lulu-telescope-front.svg`: `stargazing/lulu-telescope.txt`.
- `lulu-telescope-side.svg`: `views/side.txt`, with
  `stargazing/telescope-placed.txt` overlaid at its original coordinates.

Each horizontal run of same-colour pixels becomes an SVG path segment. Pixel
geometry is preserved, with transparent padding trimmed to one pixel around the
artwork. No raster image is embedded.

The stills use a balanced Rosé Pine artwork palette: Pine Rose for the body,
Dawn Rose for its shading, Love for the gills, Dawn Text for purple outlines,
and the viewer's approved Pine blues for the telescope. Original Gold, lens,
and eye-highlight colours remain. These are fixed illustration colours; they
do not alter the application's theme tokens.

Pixel artwork: [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/).
See the source repository's [credits](https://github.com/CtrlSpice/lulu/blob/a160ebff72687362c9b9ad26c967e4c48893dc53/CREDITS.md).
The original painting by Chelsey Bateson remains separately credited.
