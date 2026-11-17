// Palettes mixed from the inks you choose: those inks, and the ones a harmony adds (their
// neighbours in hue, their complement, a triad or a square), built into a day and a night flavour
// by the same rules and measures as the madoromi flavours (see buildFlavour). Every accent is one
// of those inks at some lightness; the neutrals are greys in the cast of the first ink. A mix
// installs through the same ports as any flavour.

import { buildFlavour, MODES, type Flavour, type Mode } from "./palette";
import type { InkView } from "./inkview";

/** All a mix needs of an ink: its ramp curve. */
type Curve = Pick<InkView, "curve">;

export type Harmony = "mono" | "analogous" | "complementary" | "triadic" | "tetradic";

export const HARMONIES: { id: Harmony; name: string; text: string; offsets: number[] }[] = [
  { id: "mono", name: "mono-ramp", text: "Only the inks you choose, as steps from dark to light. An ink has only a few strengths that read well, so some accents share a colour.", offsets: [] },
  { id: "analogous", name: "analogous", text: "The inks you choose, and inks either side of them in hue.", offsets: [-30, 30] },
  { id: "complementary", name: "complementary", text: "The inks you choose, and inks opposite them in hue.", offsets: [180] },
  { id: "triadic", name: "triadic", text: "The inks you choose, and inks a third of the way round the colour wheel from them.", offsets: [120, 240] },
  { id: "tetradic", name: "tetradic", text: "The inks you choose, and inks a quarter, a half and three quarters round the colour wheel.", offsets: [90, 180, 270] },
];

/** An ink's character: its chroma-weighted hue along the whole curve, and its most chroma. */
export function inkCharacter(v: Curve): { hue: number; chroma: number } {
  const lab = v.curve.lab;
  let a = 0;
  let b = 0;
  let peak = 0;
  for (let i = 0; i < lab.length; i += 3) {
    a += lab[i + 1];
    b += lab[i + 2];
    peak = Math.max(peak, Math.hypot(lab[i + 1], lab[i + 2]));
  }
  return { hue: ((Math.atan2(b, a) * 180) / Math.PI + 360) % 360, chroma: peak };
}

/** An ink below this much chroma anywhere is a grey: it tints, but isn't added to fill a hue. */
const GREY = 0.06;
/** A chosen ink already covers a harmony's hue within this many degrees. */
const COVERS = 30;
/** At most this many inks in one mix. */
const MAX_INKS = 6;

const hueDistance = (a: number, b: number) => Math.abs(((a - b + 540) % 360) - 180);

/** The inks a harmony brings to the chosen ones. */
export function harmonyInks(views: Map<string, Curve>, chosen: string[], harmony: Harmony): string[] {
  const offsets = HARMONIES.find((h) => h.id === harmony)!.offsets;
  const character = new Map([...views].map(([id, v]) => [id, inkCharacter(v)]));
  const family = chosen.filter((id) => views.has(id));
  const covered = (hue: number) => family.some((id) => hueDistance(character.get(id)!.hue, hue) < COVERS);
  for (const id of [...family])
    for (const off of offsets) {
      if (family.length >= MAX_INKS) break;
      const target = (character.get(id)!.hue + off + 360) % 360;
      if (covered(target)) continue;
      // The most colourful ink nearest that hue.
      let best: string | undefined;
      let bestScore = Infinity;
      for (const [other, c] of character) {
        if (family.includes(other) || c.chroma < GREY) continue;
        const score = hueDistance(c.hue, target) - c.chroma * 50;
        if (score < bestScore) [best, bestScore] = [other, score];
      }
      if (best) family.push(best);
    }
  return family;
}

/** A mix's name: its chosen inks and harmony. */
export const mixName = (chosen: string[], harmony: Harmony) => `mix-${chosen.join("-")}${harmony === "mono" ? "" : `-${harmony}`}`;

/** The day and night flavours mixed from the chosen inks (one or more) in a harmony. */
export function mixFamily(views: Map<string, Curve>, chosen: string[], harmony: Harmony): Flavour[] {
  const inks = harmonyInks(views, chosen, harmony);
  if (!inks.length) throw new Error("choose at least one ink");
  return MODES.map((m) =>
    // The season only lends its structure (its mood shapes only a whole season's accents).
    buildFlavour(views, "fuyu", m.id, new Map(), { id: `${mixName(chosen, harmony)}-${m.id}`, inks, grey: inks[0] }),
  );
}

export type { Mode };
