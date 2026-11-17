// The madoromi palettes: four seasons, each with a light (hiru, day) and a dark (yoru, night)
// flavour. Every accent is one of the inks at some dilution: a point on that ink's ramp curve,
// read at the lightness the colour needs. The neutrals are greys in the cast of the season's grey
// ink.

import { apparentLightness, contrast, maxChroma, oklabToHex, oklabToOklch, toSrgbGamut, type Vec3 } from "./color";
import type { InkView } from "./inkview";

/** All a palette needs of an ink: its ramp curve. */
type Curve = Pick<InkView, "curve">;

export type Season = "haru" | "natsu" | "aki" | "fuyu";
export type Mode = "hiru" | "yoru";

export const SEASONS: { id: Season; kanji: string; english: string; inks: string[]; grey: string }[] = [
  {
    id: "haru",
    kanji: "春",
    english: "spring",
    inks: ["hana-ikada", "tsutsuji", "syun-gyo", "tsukushi", "chiku-rin", "benzaiten", "ama-iro", "shin-ryoku"],
    grey: "kiri-same",
  },
  {
    id: "natsu",
    kanji: "夏",
    english: "summer",
    inks: ["ajisai", "asa-gao", "tsuyu-kusa", "hotaru-bi", "ama-iro", "kon-peki", "shin-kai", "ku-jaku", "sui-gyoku", "shimbashi-iro", "yu-yake"],
    grey: "fukagawa-nezu",
  },
  {
    id: "aki",
    kanji: "秋",
    english: "autumn",
    inks: ["momiji", "kosumosu", "yama-budo", "yama-guri", "ina-ho", "murasaki-shikibu", "tsuki-yo", "to-ro", "fuyu-gaki", "kiri-same"],
    grey: "yama-guri",
  },
  {
    id: "fuyu",
    kanji: "冬",
    english: "winter",
    // The seven lucky gods belong to the New Year.
    inks: ["fuyu-syogun", "rikka", "take-sumi", "fuyu-gaki", "edo-murasaki", "fukagawa-nezu", "ebisu", "daikokuten", "bishamonten", "benzaiten", "fukurokuju", "jurojin", "hoteison"],
    grey: "fuyu-syogun",
  },
];

export const MODES: { id: Mode; kanji: string; english: string }[] = [
  { id: "hiru", kanji: "昼", english: "day" },
  { id: "yoru", kanji: "夜", english: "night" },
];

export interface Accent {
  key: string;
  name: string;
  kanji: string;
  english: string;
  /** Target oklch hue, degrees. */
  hue: number;
  /** Lightness relative to the mode's accent lightness. */
  dL: Record<Mode, number>;
  minChroma: number;
  /** Prefer more chroma, or (for the pale ones) this chroma per mode. */
  vivid: boolean;
  chroma?: Record<Mode, number>;
  /** Contrast against the paper, where it differs from the mode's (gold can't be dark and gold). */
  contrast?: Partial<Record<Mode, number>>;
}

/** The accents, roughly Catppuccin's: rosewater … blue. */
export const ACCENTS: Accent[] = [
  { key: "sakura", name: "sakura", kanji: "桜", english: "cherry blossom", hue: 0, dL: { hiru: 0, yoru: 0.1 }, minChroma: 0.03, vivid: false, chroma: { hiru: 0.09, yoru: 0.09 } },
  { key: "nadeshiko", name: "nadeshiko", kanji: "撫子", english: "pink", hue: 352, dL: { hiru: 0.05, yoru: 0.04 }, minChroma: 0.08, vivid: true },
  { key: "botan", name: "botan", kanji: "牡丹", english: "peony", hue: 340, dL: { hiru: 0, yoru: 0 }, minChroma: 0.12, vivid: true },
  { key: "fuji", name: "fuji", kanji: "藤", english: "wisteria", hue: 305, dL: { hiru: 0, yoru: 0 }, minChroma: 0.1, vivid: true },
  { key: "kikyo", name: "kikyō", kanji: "桔梗", english: "bellflower", hue: 285, dL: { hiru: -0.02, yoru: 0 }, minChroma: 0.1, vivid: true },
  { key: "beni", name: "beni", kanji: "紅", english: "crimson", hue: 22, dL: { hiru: -0.02, yoru: -0.03 }, minChroma: 0.14, vivid: true },
  { key: "akane", name: "akane", kanji: "茜", english: "madder", hue: 8, dL: { hiru: 0.04, yoru: 0.04 }, minChroma: 0.1, vivid: true },
  { key: "kaki", name: "kaki", kanji: "柿", english: "persimmon", hue: 50, dL: { hiru: 0.12, yoru: 0.03 }, minChroma: 0.12, vivid: true, contrast: { hiru: 2.8 } },
  { key: "yamabuki", name: "yamabuki", kanji: "山吹", english: "kerria gold", hue: 85, dL: { hiru: 0.16, yoru: 0.04 }, minChroma: 0.15, vivid: true, contrast: { hiru: 2.5 } },
  { key: "wakaba", name: "wakaba", kanji: "若葉", english: "young leaves", hue: 135, dL: { hiru: 0.06, yoru: 0.05 }, minChroma: 0.1, vivid: true },
  { key: "tokiwa", name: "tokiwa", kanji: "常磐", english: "evergreen", hue: 175, dL: { hiru: 0.02, yoru: 0.02 }, minChroma: 0.08, vivid: true },
  { key: "sora", name: "sora", kanji: "空", english: "sky", hue: 220, dL: { hiru: 0.04, yoru: 0.04 }, minChroma: 0.08, vivid: true },
  { key: "ruri", name: "ruri", kanji: "瑠璃", english: "lapis", hue: 240, dL: { hiru: 0, yoru: 0 }, minChroma: 0.1, vivid: true },
  { key: "ai", name: "ai", kanji: "藍", english: "indigo", hue: 262, dL: { hiru: -0.02, yoru: -0.02 }, minChroma: 0.12, vivid: true },
];

export interface Neutral {
  key: string;
  name: string;
  kanji: string;
  english: string;
  /** Target lightness per mode. */
  L: Record<Mode, number>;
  /** The text tiers may carry a little more of the grey ink's colour. */
  text: boolean;
}

/** Writing on paper: the text is always the ink and the base always the paper, light or dark. */
export const NEUTRALS: Neutral[] = [
  { key: "sumi", name: "sumi", kanji: "墨", english: "ink", L: { hiru: 0.44, yoru: 0.88 }, text: true },
  { key: "usuzumi1", name: "usuzumi 1", kanji: "薄墨", english: "thinned ink", L: { hiru: 0.5, yoru: 0.82 }, text: true },
  { key: "usuzumi0", name: "usuzumi 0", kanji: "薄墨", english: "thinned ink", L: { hiru: 0.55, yoru: 0.75 }, text: true },
  { key: "kasure2", name: "kasure 2", kanji: "掠れ", english: "dry brush", L: { hiru: 0.6, yoru: 0.68 }, text: false },
  { key: "kasure1", name: "kasure 1", kanji: "掠れ", english: "dry brush", L: { hiru: 0.66, yoru: 0.61 }, text: false },
  { key: "kasure0", name: "kasure 0", kanji: "掠れ", english: "dry brush", L: { hiru: 0.71, yoru: 0.54 }, text: false },
  { key: "nijimi2", name: "nijimi 2", kanji: "滲み", english: "bleed", L: { hiru: 0.76, yoru: 0.47 }, text: false },
  { key: "nijimi1", name: "nijimi 1", kanji: "滲み", english: "bleed", L: { hiru: 0.81, yoru: 0.4 }, text: false },
  { key: "nijimi0", name: "nijimi 0", kanji: "滲み", english: "bleed", L: { hiru: 0.86, yoru: 0.32 }, text: false },
  { key: "kami", name: "kami", kanji: "紙", english: "paper", L: { hiru: 0.96, yoru: 0.24 }, text: false },
  { key: "hyoshi", name: "hyōshi", kanji: "表紙", english: "cover", L: { hiru: 0.93, yoru: 0.21 }, text: false },
  { key: "chitsu", name: "chitsu", kanji: "帙", english: "book case", L: { hiru: 0.9, yoru: 0.18 }, text: false },
];

/** Accents' lightness before contrast, and the contrast they must reach against the paper. */
const ACCENT_L: Record<Mode, number> = { hiru: 0.54, yoru: 0.76 };
export const ACCENT_CONTRAST: Record<Mode, number> = { hiru: 3.5, yoru: 6 };

/**
 * What makes a flavour's accents sit together, per mode; the knobs to turn for a calmer or a
 * richer family. Each accent is scored by how far it strays from them:
 * - apparent: how light it looks with its colourfulness counted (Helmholtz–Kohlrausch L**, see
 *   apparentLightness), so no accent glows past the others; each accent's `dL` offsets it.
 * - chroma: the flavour's chroma level, so no accent is much louder or quieter than the rest.
 * - glow: how much lighter its colourfulness makes it look than a grey of its lightness (L**
 *   less L*) before it costs; past it the accent is loud.
 * - gamut: the share of sRGB's chroma at that lightness and hue an accent may use before it
 *   reads as neon (colours on the gamut's edge do).
 */
export const HARMONY: Record<Mode, { apparent: number; chroma: number; glow: number; gamut: number }> = {
  hiru: { apparent: 0.6, chroma: 0.14, glow: 0.15, gamut: 0.85 },
  yoru: { apparent: 0.78, chroma: 0.12, glow: 0.06, gamut: 0.85 },
};
/**
 * Each season's mood, as offsets to HARMONY (spring lighter and softer, summer clear, autumn deep,
 * winter muted), and a lean: every accent's hue is drawn up to `lean` degrees towards the season's
 * `pole` (young leaves, sky, persimmon, wisteria). The same ink read at another strength is another colour, so the seasons differ
 * even where they share inks.
 */
export const MOODS: Record<Season, { apparent: number; chroma: number; pole: number; lean: number }> = {
  haru: { apparent: 0.02, chroma: -0.01, pole: 130, lean: 15 },
  natsu: { apparent: 0.01, chroma: 0.02, pole: 220, lean: 15 },
  aki: { apparent: -0.03, chroma: 0, pole: 40, lean: 15 },
  fuyu: { apparent: -0.01, chroma: -0.03, pole: 290, lean: 15 },
};
/**
 * How much each way of straying costs, per unit (hue in degrees, the rest as fractions). Chroma
 * past the flavour's costs `loud`, short of it `chroma`: a loud accent spoils a flavour more.
 */
export const COST = { hue: 1 / 8, hueFar: 1, chroma: 50, loud: 150, shortfall: 400, apparent: 60, glow: 30, gamut: 25, reuse: 0.6, season: 4, echo: 3, contrast: 8 };
/** No accent looks lighter or darker than its target by more than this (in L**), where any ink can. */
export const APPARENT_BAND = 0.06;
/** Hue strays up to this many degrees cost only `hue`; past it, `hueFar` too. */
const HUE_EASY = 15;
/** How far from its lightness an accent may be read, either way. */
const ACCENT_REACH = 0.15;
/** Accents in one flavour are at least this far apart (ΔE in oklab). */
export const MIN_DELTA = 0.05;
/** How far an accent's hue may stray from its name's. */
export const MAX_HUE_SHIFT = 25;
const accentContrast = (a: Accent, mode: Mode) => a.contrast?.[mode] ?? ACCENT_CONTRAST[mode];
/**
 * An accent that can't keep its colour at full contrast (a gold on white) may give some up, to
 * this share of it, and pays COST.contrast for every 1:1 it gives.
 */
export const CONTRAST_GIVE = 0.85;
/**
 * The neutrals' chroma, the same in every season so that only the cast differs: the papers
 * (kami, hyōshi, chitsu), the ladder between, and the text. Each takes at most this share of
 * what sRGB can show there.
 */
const NEUTRAL_CHROMA = { paper: { hiru: 0.016, yoru: 0.022 } as Record<Mode, number>, ladder: 0.028, text: 0.034, gamut: 0.5 };
const PAPERS = ["kami", "hyoshi", "chitsu"];
/** The text tiers' minimum contrast against the paper. */
const TEXT_CONTRAST: Record<string, number> = { sumi: 7, usuzumi0: 4.5, kasure2: 3 };

export interface Swatch {
  lab: Vec3;
  hex: string;
  /** The ink it is (for a neutral, the ink whose cast it has), and its lightness. */
  ink: string;
  L: number;
}

export interface Flavour {
  id: string;
  season: Season;
  mode: Mode;
  colours: Record<string, Swatch>;
  /** The 16 terminal colours, 0–15. */
  ansi: Swatch[];
}

const swatch = (lab: Vec3, ink: string, L: number): Swatch => {
  const c = toSrgbGamut(lab);
  return { lab: c, hex: oklabToHex(c), ink, L };
};

/** An ink's colour at a lightness, read along its ramp curve; undefined outside the ramp. */
export function colourAt(v: Curve, L: number): Vec3 | undefined {
  const lab = v.curve.lab;
  const n = lab.length / 3;
  if (L < lab[0] || L > lab[(n - 1) * 3]) return;
  let i = 0;
  while (i < n - 2 && lab[(i + 1) * 3] < L) i++;
  const f = Math.min(1, Math.max(0, (L - lab[i * 3]) / (lab[(i + 1) * 3] - lab[i * 3] || 1)));
  return [L, lab[i * 3 + 1] + (lab[(i + 1) * 3 + 1] - lab[i * 3 + 1]) * f, lab[i * 3 + 2] + (lab[(i + 1) * 3 + 2] - lab[i * 3 + 2]) * f];
}

/**
 * A grey ink's cast: its hue where it's dense (the darker half of its ramp), weighted by chroma.
 * Thinned out, the greys all drift towards the same blue; dense, each has its own colour.
 */
function castOf(v: Curve): number {
  const lab = v.curve.lab;
  const n = lab.length / 3;
  const mid = (lab[0] + lab[(n - 1) * 3]) / 2;
  let a = 0;
  let b = 0;
  for (let i = 0; i < n && lab[i * 3] <= mid; i++) {
    a += lab[i * 3 + 1];
    b += lab[i * 3 + 2];
  }
  return (Math.atan2(b, a) * 180) / Math.PI;
}

const hueDistance = (a: number, b: number) => Math.abs(((a - b + 540) % 360) - 180);
const deltaE = (a: Vec3, b: Vec3) => Math.hypot(a[0] - b[0], a[1] - b[1], a[2] - b[2]);

/** The accents that are the terminal's colours 1–6: red, green, yellow, blue, magenta, cyan. */
const ANSI_HUES = ["beni", "wakaba", "yamabuki", "ai", "botan", "tokiwa"];

/** Where a ladder's steps may stand: the mode's accent lightness, give or take this much. */
const LADDER_REACH: Record<Mode, [number, number]> = { hiru: [-0.22, 0.2], yoru: [-0.18, 0.16] };
/** Steps on one ink stand this far apart (ΔE in oklab) where the ink has room. */
const LADDER_GAP = MIN_DELTA * 1.25;
/** A grey ink (less chroma than this) is this many degrees further from a colourful accent in hue. */
const GREY_MISS = { chroma: 0.06, degrees: 90 };

/**
 * Accents for a palette of a few inks (a mix): each accent is the ink nearest its hue, and the
 * accents that share an ink are told apart by lightness, as steps along its ramp. Where an ink
 * has a few accents, its steps stand close around the mode's accent lightness, so the palette
 * stays evenly bright; where it has many (one ink alone), they spread along the whole legible
 * stretch, in the order of their lightness offsets (gold and persimmon lightest, indigo darkest).
 * An ink has only so many strengths MIN_DELTA apart; past that, accents share a colour outright
 * rather than come out nearly alike.
 */
function ladder(views: Map<string, Curve>, inks: string[], mode: Mode, paper: Vec3): Record<string, Swatch> {
  const [lo, hi] = LADDER_REACH[mode].map((d) => ACCENT_L[mode] + d);
  type Point = { L: number; lab: Vec3; at: number; h: number; C: number; contrast: number };
  // Each accent's least contrast, as the madoromi flavours allow it (gold may be lighter on white).
  const floor = (a: Accent) => accentContrast(a, mode) * CONTRAST_GIVE;
  const least = Math.min(...ACCENTS.map(floor));
  // Each ink's stretch legible enough for some accent, sampled finely, with the distance along it.
  const stretches = inks.flatMap((ink) => {
    const v = views.get(ink);
    if (!v) return [];
    const points: Point[] = [];
    for (let L = lo; L <= hi + 1e-9; L += 0.005) {
      const c = colourAt(v, L);
      if (!c) continue;
      const lab = toSrgbGamut(c);
      const k = contrast(lab, paper);
      if (k < least) continue;
      const [, C, h] = oklabToOklch(lab);
      points.push({ L, lab, h, C, contrast: k, at: points.length ? points[points.length - 1].at + deltaE(points[points.length - 1].lab, lab) : 0 });
    }
    return points.length ? [{ ink, points, length: points[points.length - 1].at }] : [];
  });
  if (!stretches.length) throw new Error(`none of ${inks.join(", ")} is legible enough on this paper`);

  // How far an ink is from an accent: its hue over the stretch (weighted by chroma), further if
  // it's too grey for the accent, and out of reach if it's nowhere legible enough for it.
  const character = new Map(
    stretches.map((st) => {
      const [a, b] = st.points.reduce(([a, b], p) => [a + p.lab[1], b + p.lab[2]], [0, 0]);
      return [st, { h: (Math.atan2(b, a) * 180) / Math.PI, C: Math.max(...st.points.map((p) => p.C)) }];
    }),
  );
  const miss = (st: (typeof stretches)[number], a: Accent) => {
    const c = character.get(st)!;
    return hueDistance(c.h, a.hue) + (a.vivid && c.C < GREY_MISS.chroma ? GREY_MISS.degrees : 0) + (st.points.some((q) => q.contrast >= floor(a)) ? 0 : 1000);
  };
  // Which accents keep a step of their own first: the six terminal colours, then those drawn in
  // the most roles.
  const seen = (key: string) => (ANSI_HUES.includes(key) ? ACCENTS.length : 0) + Object.values(ROLES).filter((k) => k === key).length;
  const out: Record<string, Swatch> = {};
  for (const st of stretches) {
    const mine = ACCENTS.filter((a) => stretches.reduce((b, o) => (miss(o, a) < miss(b, a) ? o : b)) === st);
    if (!mine.length) continue;
    const steps = Math.max(1, Math.min(mine.length, Math.floor(st.length / MIN_DELTA) + 1));
    // Those get a step each, darkest-meant first; the rest share the colour of the one nearest
    // them in hue (akane beni's, ruri ai's), as a 16-colour terminal draws code.
    const own = [...mine].sort((x, y) => seen(y.key) - seen(x.key)).slice(0, steps).sort((x, y) => x.dL[mode] - y.dL[mode]);
    // Close around the accent lightness where they fit; else spread over the whole stretch.
    const gap = Math.min(LADDER_GAP, steps > 1 ? st.length / (steps - 1) : 0);
    const want = ACCENT_L[mode] + own.reduce((sum, a) => sum + a.dL[mode], 0) / own.length;
    const centre = st.points.reduce((b, q) => (Math.abs(q.L - want) < Math.abs(b.L - want) ? q : b)).at;
    let first = Math.min(Math.max(0, centre - (gap * (steps - 1)) / 2), st.length - gap * (steps - 1));
    // Slide the steps along so each accent has its full contrast where the ink allows, else its least.
    const legible = (min: number) => st.points.filter((q) => q.contrast >= min);
    for (const need of [(a: Accent) => accentContrast(a, mode), floor]) {
      let [from, to] = [-Infinity, Infinity];
      own.forEach((a, i) => {
        const fit = legible(need(a));
        from = Math.max(from, (fit.length ? fit[0].at : Infinity) - i * gap);
        to = Math.min(to, (fit.length ? fit[fit.length - 1].at : -Infinity) - i * gap);
      });
      if (from <= to) {
        first = Math.min(Math.max(first, from), to);
        break;
      }
    }
    own.forEach((a, i) => {
      const at = first + i * gap;
      // The nearest step legible enough for this accent (or, if none is, the most legible).
      const fit = legible(accentContrast(a, mode)).length ? legible(accentContrast(a, mode)) : legible(floor(a));
      const p = fit.length ? fit.reduce((b, q) => (Math.abs(q.at - at) < Math.abs(b.at - at) ? q : b)) : st.points.reduce((b, q) => (q.contrast > b.contrast ? q : b));
      out[a.key] = swatch(p.lab, st.ink, p.L);
    });
    for (const a of mine.filter((x) => !own.includes(x))) out[a.key] = out[own.reduce((b, o) => (hueDistance(o.hue, a.hue) < hueDistance(b.hue, a.hue) ? o : b)).key];
  }

  // Accents nearly alike (on two inks, or crowded at the end of one): the less seen moves to the
  // nearest strength of its ink that's apart from every other accent, or else takes the other's colour.
  const alike = (x: Swatch, y: Swatch) => x !== y && deltaE(x.lab, y.lab) < MIN_DELTA;
  for (const a of [...ACCENTS].sort((x, y) => seen(x.key) - seen(y.key))) {
    const others = ACCENTS.filter((o) => o !== a).map((o) => out[o.key]);
    const near = others.find((o) => alike(out[a.key], o));
    if (!near) continue;
    const st = stretches.find((x) => x.ink === out[a.key].ink)!;
    const here = st.points.reduce((b, q) => (Math.abs(q.L - out[a.key].L) < Math.abs(b.L - out[a.key].L) ? q : b)).at;
    const free = st.points
      .filter((q) => q.contrast >= floor(a) && others.every((o) => deltaE(o.lab, q.lab) >= MIN_DELTA))
      .sort((x, y) => (y.contrast >= accentContrast(a, mode) ? 1 : 0) - (x.contrast >= accentContrast(a, mode) ? 1 : 0) || Math.abs(x.at - here) - Math.abs(y.at - here))[0];
    out[a.key] = free ? swatch(free.lab, st.ink, free.L) : near;
  }
  return out;
}

/** How a flavour can be made other than from a whole season: a mix (see mix.ts). */
export interface FlavourOptions {
  /** Its id, if not `<season>-<mode>`. */
  id?: string;
  /** The ink whose cast tints the neutrals, if not the season's grey. */
  grey?: string;
  /** Accents from only these inks, as a ladder (see ladder), rather than the best fit from all. */
  inks?: string[];
}

/**
 * Builds one flavour from every ink's view. `echoes` holds, per accent, the inks other seasons of
 * this mode already used for it; reusing one costs a little, so the seasons differ where they can.
 */
export function buildFlavour(views: Map<string, Curve>, season: Season, mode: Mode, echoes = new Map<string, Set<string>>(), opts: FlavourOptions = {}): Flavour {
  const s0 = SEASONS.find((x) => x.id === season)!;
  const s = { ...s0, grey: opts.grey ?? s0.grey };
  const grey = views.get(s.grey)!;
  const cast = castOf(grey);
  const colours: Record<string, Swatch> = {};

  // --- Neutrals: each tier's lightness, in the season's grey ink's cast.
  const neutralAt = (n: Neutral, L: number): Vec3 => {
    const want = PAPERS.includes(n.key) ? NEUTRAL_CHROMA.paper[mode] : n.text ? NEUTRAL_CHROMA.text : NEUTRAL_CHROMA.ladder;
    const C = Math.min(want, NEUTRAL_CHROMA.gamut * maxChroma(L, cast));
    return [L, C * Math.cos((cast * Math.PI) / 180), C * Math.sin((cast * Math.PI) / 180)];
  };
  for (const n of NEUTRALS) colours[n.key] = swatch(neutralAt(n, n.L[mode]), s.grey, n.L[mode]);
  const paper = colours.kami.lab;
  // Text tiers move away from the paper until they're legible enough.
  for (const [key, min] of Object.entries(TEXT_CONTRAST)) {
    const n = NEUTRALS.find((x) => x.key === key)!;
    let L = n.L[mode];
    while (contrast(colours[key].lab, paper) < min && L > 0.02 && L < 0.98) {
      L += mode === "yoru" ? 0.01 : -0.01;
      colours[key] = swatch(neutralAt(n, L), s.grey, L);
    }
  }

  // --- Accents: from a few inks, a ladder (see ladder); from all, the best fit for each.
  if (opts.inks) Object.assign(colours, ladder(views, opts.inks, mode, paper));
  else {
    // --- Accents. Every ink is a candidate, read at every lightness near the accent's that's legible
    // enough on the paper. The best is the nearest in hue that keeps the flavour's harmony (see
    // HARMONY), preferring the season's inks and inks not used yet, and far enough from the accents
    // already chosen. The pale ones choose first, then those with the fewest inks near their hue.
    type Candidate = { lab: Vec3; ink: string; L: number; h: number; C: number; gamut: number; apparent: number; glow: number; short: number };
    const candidates = new Map<string, Candidate[]>();
    for (const a of ACCENTS) {
      const L0 = ACCENT_L[mode] + a.dL[mode];
      const min = accentContrast(a, mode);
      const list: Candidate[] = [];
      for (const [ink, v] of views)
        for (let L = L0 - ACCENT_REACH; L <= L0 + ACCENT_REACH + 1e-9; L += 0.01) {
          const c = colourAt(v, L);
          if (!c || contrast(c, paper) < min * CONTRAST_GIVE) continue;
          const lab = toSrgbGamut(c);
          const [, C, h] = oklabToOklch(lab);
          const apparent = apparentLightness(lab);
          const short = Math.max(0, min - contrast(lab, paper));
          list.push({ lab, ink, L, h, C, gamut: C / maxChroma(L, h), apparent, glow: apparent - apparentLightness([lab[0], 0, 0]), short });
        }
      candidates.set(a.key, list);
    }
    const near = (key: string) => candidates.get(key)!.filter((c) => hueDistance(c.h, ACCENTS.find((a) => a.key === key)!.hue) < 20 && c.C > 0.05).length;
    const pale = (key: string) => (ACCENTS.find((a) => a.key === key)!.vivid ? 1 : 0);
    const order = ACCENTS.map((a) => a.key).sort((x, y) => pale(x) - pale(y) || near(x) - near(y));
    const mood = MOODS[season];
    const harmony = { ...HARMONY[mode], apparent: HARMONY[mode].apparent + mood.apparent, chroma: HARMONY[mode].chroma + mood.chroma };
    const used = new Map<string, number>();
    const chosen: Vec3[] = [];
    for (const key of order) {
      const a0 = ACCENTS.find((x) => x.key === key)!;
      const a = { ...a0, hue: (a0.hue + mood.lean * Math.sin(((mood.pole - a0.hue) * Math.PI) / 180) + 360) % 360 };
      // dL is in oklab lightness; L** runs about 1.16 times as fast.
      const apparent = harmony.apparent + a.dL[mode] * 1.16;
      let best: (Candidate & { score: number }) | undefined;
      // Within the band and at full contrast if anything there is colourful enough for the accent;
      // past the band if not; and only then giving up a little contrast (see CONTRAST_GIVE).
      for (const [band, give] of [[APPARENT_BAND, false], [Infinity, false], [APPARENT_BAND, true], [Infinity, true]] as const) {
      if (best && (!a.vivid || best.C >= a.minChroma * 0.85)) break;
      best = undefined;
      for (const c of candidates.get(key)!) {
        if (chosen.some((o) => deltaE(o, c.lab) < MIN_DELTA)) continue;
        if (c.short && !give) continue;
        const hd = hueDistance(c.h, a.hue);
        // However the season leans, an accent stays near its name's hue.
        if (hueDistance(c.h, a0.hue) > MAX_HUE_SHIFT || Math.abs(c.apparent - apparent) > band) continue;
        const score =
          hd * COST.hue +
          Math.max(0, hd - HUE_EASY) * COST.hueFar +
          (a.vivid ? Math.max(0, harmony.chroma - c.C) * COST.chroma + Math.max(0, c.C - harmony.chroma) * COST.loud + Math.max(0, a.minChroma - c.C) * COST.shortfall : Math.abs(c.C - a.chroma![mode]) * COST.shortfall) +
          Math.abs(c.apparent - apparent) * COST.apparent +
          c.short * COST.contrast +
          Math.max(0, c.glow - harmony.glow) * COST.glow +
          Math.max(0, c.gamut - harmony.gamut) * COST.gamut +
          (used.get(c.ink) ?? 0) * COST.reuse +
          (echoes.get(key)?.has(c.ink) ? COST.echo : 0) -
          // The season's own inks, where they're colourful enough for the accent.
          (s.inks.includes(c.ink) && (!a.vivid || c.C >= a.minChroma) ? COST.season : 0);
        if (!best || score < best.score) best = { ...c, score };
      }
      }
      if (!best) throw new Error(`no ink fits ${key} in ${season}-${mode}`);
      colours[key] = swatch(best.lab, best.ink, best.L);
      chosen.push(colours[key].lab);
      used.set(best.ink, (used.get(best.ink) ?? 0) + 1);
    }
  }

  // --- Terminal colours. Bright ones are the same ink a step further from the paper.
  const brighter = (key: string): Swatch => {
    const sw = colours[key];
    const L = sw.L + (mode === "yoru" ? 0.06 : -0.06);
    const c = colourAt(views.get(sw.ink)!, L);
    return c ? swatch(c, sw.ink, L) : sw;
  };
  const [black, brightBlack, white, brightWhite] =
    mode === "yoru" ? ["nijimi1", "nijimi2", "usuzumi1", "usuzumi0"] : ["usuzumi1", "usuzumi0", "nijimi2", "nijimi1"];
  const ansi = [colours[black], ...ANSI_HUES.map((k) => colours[k]), colours[white], colours[brightBlack], ...ANSI_HUES.map(brighter), colours[brightWhite]];

  return { id: opts.id ?? `${season}-${mode}`, season, mode, colours, ansi };
}

export function buildFamily(views: Map<string, Curve>): Flavour[] {
  // Every day flavour, then every night one; within each, the seasons in order.
  return MODES.flatMap((m) => {
    const echoes = new Map<string, Set<string>>();
    return SEASONS.map((s) => {
      const f = buildFlavour(views, s.id, m.id, echoes);
      for (const a of ACCENTS) echoes.set(a.key, (echoes.get(a.key) ?? new Set()).add(f.colours[a.key].ink));
      return f;
    });
  });
}

export interface Check {
  flavour: string;
  what: string;
  value: number;
  min: number;
}

/** Everything a flavour must meet: accents' and text tiers' contrast, accents' distinctness. */
export function checkFlavour(f: Flavour): Check[] {
  const paper = f.colours.kami.lab;
  const out: Check[] = [];
  for (const a of ACCENTS) out.push({ flavour: f.id, what: `${a.key} contrast`, value: contrast(f.colours[a.key].lab, paper), min: accentContrast(a, f.mode) * CONTRAST_GIVE });
  for (const [key, min] of Object.entries(TEXT_CONTRAST)) out.push({ flavour: f.id, what: `${key} contrast`, value: contrast(f.colours[key].lab, paper), min });
  for (let i = 0; i < ACCENTS.length; i++)
    for (let j = i + 1; j < ACCENTS.length; j++)
      out.push({
        flavour: f.id,
        what: `${ACCENTS[i].key}/${ACCENTS[j].key} ΔE`,
        value: deltaE(f.colours[ACCENTS[i].key].lab, f.colours[ACCENTS[j].key].lab),
        min: MIN_DELTA,
      });
  return out;
}

/** What every port, and the page's previews, draws each role in. */
export const ROLES = {
  // Code.
  text: "sumi",
  comment: "kasure2",
  punctuation: "usuzumi0",
  keyword: "fuji",
  operator: "sora",
  string: "wakaba",
  escape: "botan",
  regexp: "botan",
  number: "kaki",
  constant: "kaki",
  enumMember: "tokiwa",
  character: "tokiwa",
  function: "ai",
  builtin: "beni",
  macro: "tokiwa",
  decorator: "nadeshiko",
  type: "yamabuki",
  namespace: "kikyo",
  parameter: "akane",
  property: "ruri",
  self: "beni",
  tag: "botan",
  attribute: "yamabuki",
  label: "sakura",
  // Prose.
  heading: "fuji",
  link: "sora",
  code: "wakaba",
  quote: "kasure2",
  list: "kaki",
  // Interface and state.
  accent: "fuji",
  cursor: "sakura",
  match: "kaki",
  error: "beni",
  warning: "yamabuki",
  info: "sora",
  hint: "tokiwa",
  success: "wakaba",
  added: "wakaba",
  modified: "yamabuki",
  deleted: "beni",
} as const;
export type Role = keyof typeof ROLES;

/** How evenly a flavour's accents sit: the spread of their apparent lightness and chroma, and the loudest. */
export function harmonyOf(f: Flavour) {
  const rows = ACCENTS.map((a) => {
    const lab = f.colours[a.key].lab;
    const [L, C, h] = oklabToOklch(lab);
    const apparent = apparentLightness(lab);
    return { key: a.key, apparent, glow: apparent - apparentLightness([L, 0, 0]), C, gamut: C / maxChroma(L, h) };
  });
  const mean = (xs: number[]) => xs.reduce((p, q) => p + q, 0) / xs.length;
  const sd = (xs: number[]) => Math.sqrt(mean(xs.map((x) => (x - mean(xs)) ** 2)));
  const apparent = rows.map((r) => r.apparent);
  const loudest = rows.reduce((p, q) => (q.glow > p.glow ? q : p));
  return { apparent: { mean: mean(apparent), sd: sd(apparent) }, glow: mean(rows.map((r) => r.glow)), chroma: { mean: mean(rows.map((r) => r.C)), sd: sd(rows.map((r) => r.C)) }, gamut: mean(rows.map((r) => r.gamut)), loudest, rows };
}

/** A family as `bun run palettes` stores it (src/flavours.json), and back. */
export type FamilyJSON = { id: string; season: Season; mode: Mode; colours: Record<string, Swatch>; ansi: Swatch[] }[];
const roundSwatch = (w: Swatch): Swatch => ({ lab: w.lab.map((x) => Math.round(x * 1e5) / 1e5) as Vec3, hex: w.hex, ink: w.ink, L: Math.round(w.L * 1e4) / 1e4 });
export const encodeFamily = (family: Flavour[]): FamilyJSON =>
  family.map((f) => ({
    id: f.id,
    season: f.season,
    mode: f.mode,
    colours: Object.fromEntries(Object.entries(f.colours).map(([k, w]) => [k, roundSwatch(w)])),
    ansi: f.ansi.map(roundSwatch),
  }));
export const decodeFamily = (json: FamilyJSON): Flavour[] => json as Flavour[];
