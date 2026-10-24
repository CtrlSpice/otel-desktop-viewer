// Colour-space conversions. Oklab triples are [L, a, b]; linear sRGB in [0, 1].

export type Vec3 = [number, number, number];

const SRGB_TO_LINEAR = new Float32Array(256);
for (let i = 0; i < 256; i++) {
  const c = i / 255;
  SRGB_TO_LINEAR[i] = c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

export function srgb8ToLinear(v: number): number {
  return SRGB_TO_LINEAR[v];
}

export function linearToSrgb(c: number): number {
  return c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055;
}

export function linearToOklab(r: number, g: number, b: number, out: Vec3 = [0, 0, 0]): Vec3 {
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  out[0] = 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s;
  out[1] = 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s;
  out[2] = 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s;
  return out;
}

export function oklabToLinear(L: number, a: number, b: number, out: Vec3 = [0, 0, 0]): Vec3 {
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  out[0] = 4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s;
  out[1] = -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s;
  out[2] = -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s;
  return out;
}

export function oklabToOklch([L, a, b]: Vec3): Vec3 {
  const C = Math.hypot(a, b);
  let h = (Math.atan2(b, a) * 180) / Math.PI;
  if (h < 0) h += 360;
  return [L, C, h];
}

function inGamut(c: Vec3, eps = 1e-4): boolean {
  return c[0] >= -eps && c[0] <= 1 + eps && c[1] >= -eps && c[1] <= 1 + eps && c[2] >= -eps && c[2] <= 1 + eps;
}

/** Map oklab into sRGB by reducing chroma at constant L and hue. Returns gamma-encoded sRGB in [0,1]. */
/**
 * The colour with its chroma reduced, at the same lightness and hue, just far enough to be
 * displayable in sRGB. In-gamut colours come back unchanged.
 */
export function toSrgbGamut(lab: Vec3): Vec3 {
  const L = Math.min(1, Math.max(0, lab[0]));
  const lin: Vec3 = [0, 0, 0];
  oklabToLinear(L, lab[1], lab[2], lin);
  if (inGamut(lin)) return [L, lab[1], lab[2]];
  let lo = 0;
  let hi = 1;
  for (let i = 0; i < 16; i++) {
    const mid = (lo + hi) / 2;
    oklabToLinear(L, lab[1] * mid, lab[2] * mid, lin);
    if (inGamut(lin)) lo = mid;
    else hi = mid;
  }
  return [L, lab[1] * lo, lab[2] * lo];
}

export function oklabToSrgbGamut(lab: Vec3, out: Vec3 = [0, 0, 0]): { rgb: Vec3; clipped: boolean } {
  const L = Math.min(1, Math.max(0, lab[0]));
  const lin: Vec3 = [0, 0, 0];
  oklabToLinear(L, lab[1], lab[2], lin);
  let clipped = false;
  if (!inGamut(lin)) {
    clipped = true;
    let lo = 0;
    let hi = 1;
    for (let i = 0; i < 14; i++) {
      const mid = (lo + hi) / 2;
      oklabToLinear(L, lab[1] * mid, lab[2] * mid, lin);
      if (inGamut(lin)) lo = mid;
      else hi = mid;
    }
    oklabToLinear(L, lab[1] * lo, lab[2] * lo, lin);
  }
  for (let i = 0; i < 3; i++) out[i] = linearToSrgb(Math.min(1, Math.max(0, lin[i])));
  return { rgb: out, clipped };
}

export function oklabToHex(lab: Vec3): string {
  const { rgb } = oklabToSrgbGamut(lab);
  return "#" + rgb.map((c) => Math.round(c * 255).toString(16).padStart(2, "0")).join("");
}

export function oklabToCss(lab: Vec3): string {
  const [L, C, h] = oklabToOklch(lab);
  return `oklch(${(L * 100).toFixed(1)}% ${C.toFixed(4)} ${h.toFixed(1)})`;
}

/** WCAG 2 relative luminance of the colour as displayed (gamut-mapped to sRGB). */
export function luminance(lab: Vec3): number {
  const { rgb } = oklabToSrgbGamut(lab);
  const lin = rgb.map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2];
}

/** WCAG 2 contrast ratio between two colours, 1–21. */
export function contrast(a: Vec3, b: Vec3): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

/** The most chroma sRGB can show at this oklab lightness and hue. */
export function maxChroma(L: number, h: number): number {
  const r = (h * Math.PI) / 180;
  const [, a, b] = toSrgbGamut([L, Math.cos(r), Math.sin(r)]);
  return Math.hypot(a, b);
}

/** CIELAB (D65), via linear sRGB: L* 0–100. */
export function oklabToCielab(lab: Vec3): Vec3 {
  const [r, g, b] = oklabToLinear(lab[0], lab[1], lab[2]);
  const x = (0.4124564 * r + 0.3575761 * g + 0.1804375 * b) / 0.95047;
  const y = 0.2126729 * r + 0.7151522 * g + 0.072175 * b;
  const z = (0.0193339 * r + 0.119192 * g + 0.9503041 * b) / 1.08883;
  const f = (t: number) => (t > 216 / 24389 ? Math.cbrt(t) : (24389 / 27 * t + 16) / 116);
  return [116 * f(y) - 16, 500 * (f(x) - f(y)), 200 * (f(y) - f(z))];
}

/**
 * How light a colour looks once its colourfulness is counted (the Helmholtz–Kohlrausch effect):
 * Fairchild & Pirrotta's L**, as a fraction. Vivid colours look lighter than their L*, blues and
 * purples the most, yellows the least.
 */
export function apparentLightness(lab: Vec3): number {
  const [L, a, b] = oklabToCielab(lab);
  const C = Math.hypot(a, b);
  const h = Math.atan2(b, a);
  return (L + (2.5 - 0.025 * L) * (0.116 * Math.abs(Math.sin((h - Math.PI / 2) / 2)) + 0.085) * C) / 100;
}
