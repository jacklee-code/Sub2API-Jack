/**
 * Jack 文房 (Wenfang) palette for the Graphite theme.
 *
 * Colour only appears where it carries meaning. Four stationery materials do:
 * blue-black ink (information, links, primary data), verdigris (success),
 * brass (warning) and madder (danger). Upstream's purely decorative purples and
 * pinks fall back to a near-neutral ink grey. Charts need more distinct series,
 * so they get a categorical set built from the same materials.
 *
 * Every hue is one OKLCH base (hue angle, peak chroma). All eleven shades come
 * from one lightness ladder: light shades stay close to paper and mid shades sit
 * deep, like ink dried on paper. Upstream keeps using Tailwind family names
 * (`bg-blue-100`, `#3b82f6` in chart code); the two maps below decide which Jack
 * hue each family becomes. To retune the look, change a base value here.
 *
 * Plain JS (typed with JSDoc, types in palette.d.ts) so it runs in the Tailwind
 * config and in the browser, and `vue-tsc -b` does not emit output next to it.
 */
import { TAILWIND_DEFAULTS } from './tailwind-defaults.js'

export const JACK_SHADES = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950]

/** [OKLCH lightness, share of peak chroma] for each shade. */
export const JACK_LADDER = [
  [0.978, 0.07],
  [0.957, 0.14],
  [0.912, 0.27],
  [0.842, 0.47],
  [0.748, 0.74],
  [0.648, 0.95],
  [0.558, 1],
  [0.472, 0.92],
  [0.392, 0.8],
  [0.318, 0.66],
  [0.232, 0.52]
]

/** Jack hues as [OKLCH hue angle, peak chroma]. */
export const JACK_HUES = {
  // Interface: meaning only.
  blueblack: [258, 0.09], // 藍黑墨水
  verdigris: [172, 0.075], // 銅綠
  brass: [84, 0.1], // 黃銅
  madder: [24, 0.12], // 茜紅
  inkgrey: [300, 0.018], // 墨灰
  // Extra chart series.
  mist: [228, 0.06], // 霧藍
  slate: [200, 0.055], // 石青
  lichen: [128, 0.07], // 地衣
  sienna: [48, 0.105], // 赭石
  rouge: [4, 0.075], // 胭脂
  aubergine: [318, 0.065], // 茄紫
  dai: [288, 0.065] // 黛
}

/** Tailwind family → Jack hue for templates (badges, chips, buttons, text). */
export const JACK_UI_FAMILIES = {
  blue: 'blueblack',
  indigo: 'blueblack',
  sky: 'blueblack',
  cyan: 'blueblack',
  green: 'verdigris',
  emerald: 'verdigris',
  teal: 'verdigris',
  lime: 'verdigris',
  amber: 'brass',
  yellow: 'brass',
  orange: 'brass',
  red: 'madder',
  rose: 'madder',
  violet: 'inkgrey',
  purple: 'inkgrey',
  fuchsia: 'inkgrey',
  pink: 'inkgrey'
}

/** Tailwind family → Jack hue for chart series; the series families upstream uses stay distinct. */
export const JACK_CHART_FAMILIES = {
  blue: 'blueblack',
  emerald: 'verdigris',
  green: 'verdigris',
  amber: 'brass',
  yellow: 'brass',
  red: 'madder',
  rose: 'madder',
  violet: 'aubergine',
  purple: 'aubergine',
  pink: 'rouge',
  fuchsia: 'rouge',
  teal: 'slate',
  orange: 'sienna',
  indigo: 'dai',
  cyan: 'mist',
  sky: 'mist',
  lime: 'lichen'
}

/** Tailwind neutrals; they follow the Jack greys (`--jack-gray-*`). */
export const JACK_NEUTRAL_FAMILIES = ['gray', 'slate', 'zinc', 'neutral', 'stone']

/** @param {number} c */
const toSrgb = (c) => (c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055)

/**
 * @param {number} L
 * @param {number} C
 * @param {number} h
 * @returns {number[]} linear sRGB
 */
function oklchToLinear(L, C, h) {
  const a = C * Math.cos((h * Math.PI) / 180)
  const b = C * Math.sin((h * Math.PI) / 180)
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s
  ]
}

/**
 * OKLCH to `#rrggbb`, lowering chroma until the colour fits in sRGB.
 * @param {number} L
 * @param {number} C
 * @param {number} h
 */
export function oklchToHex(L, C, h) {
  let chroma = C
  let rgb = oklchToLinear(L, chroma, h)
  while (chroma > 0 && rgb.some((v) => v < -0.0005 || v > 1.0005)) {
    chroma = Math.max(0, chroma - 0.002)
    rgb = oklchToLinear(L, chroma, h)
  }
  return `#${rgb
    .map((v) => Math.round(Math.min(1, Math.max(0, toSrgb(Math.max(0, v)))) * 255).toString(16).padStart(2, '0'))
    .join('')}`
}

/**
 * The eleven shades of one Jack hue.
 * @param {keyof typeof JACK_HUES} hue
 * @returns {Record<string, string>}
 */
export function jackScale(hue) {
  const [h, c] = JACK_HUES[hue]
  return Object.fromEntries(JACK_SHADES.map((shade, i) => [shade, oklchToHex(JACK_LADDER[i][0], c * JACK_LADDER[i][1], h)]))
}

/**
 * Tailwind family → shade scale, following one of the maps above.
 * @param {Record<string, string>} map
 * @returns {Record<string, Record<string, string>>}
 */
export function jackFamilyColors(map) {
  return Object.fromEntries(
    Object.entries(map).map(([family, hue]) => [family, jackScale(/** @type {keyof typeof JACK_HUES} */ (hue))])
  )
}

/** rgb triple → [family, shade] for every Tailwind default colour. */
const TAILWIND_INDEX = new Map()
for (const [family, shades] of Object.entries(TAILWIND_DEFAULTS)) {
  for (const [shade, hex] of Object.entries(shades)) {
    const rgb = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16)).join(',')
    if (!TAILWIND_INDEX.has(rgb)) TAILWIND_INDEX.set(rgb, [family, shade])
  }
}

const HEX = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i
const RGB = /^rgba?\(\s*(\d+(?:\.\d+)?)[\s,]+(\d+(?:\.\d+)?)[\s,]+(\d+(?:\.\d+)?)\s*(?:[,/]\s*(\d*\.?\d+%?)\s*)?\)$/i

/**
 * @param {string} value
 * @returns {{ rgb: number[], alpha: string | undefined } | null}
 */
function parseColor(value) {
  const hex = HEX.exec(value)
  if (hex) {
    const digits = hex[1].length === 3 ? [...hex[1]].map((d) => d + d).join('') : hex[1]
    return { rgb: [0, 2, 4].map((i) => parseInt(digits.slice(i, i + 2), 16)), alpha: undefined }
  }
  const rgb = RGB.exec(value)
  if (rgb) return { rgb: rgb.slice(1, 4).map((n) => Math.round(Number(n))), alpha: rgb[4] }
  return null
}

const CHART_SCALES = jackFamilyColors(JACK_CHART_FAMILIES)

/**
 * Maps a Tailwind default colour used in upstream chart code to its Jack chart
 * colour (same shade, alpha kept). Neutrals go through `grey(shade)` when given,
 * which returns an `r g b` channel string such as a `--jack-gray-*` value.
 * Colours that are not Tailwind defaults are returned unchanged.
 * @param {string} color
 * @param {(shade: string) => string | undefined} [grey]
 * @returns {string}
 */
export function jackChartColor(color, grey) {
  if (typeof color !== 'string') return color
  const parsed = parseColor(color.trim())
  if (!parsed) return color
  const match = TAILWIND_INDEX.get(parsed.rgb.join(','))
  if (!match) return color
  const [family, shade] = match

  let target
  if (CHART_SCALES[family]) {
    target = CHART_SCALES[family][shade]
      .slice(1)
      .match(/../g)
      .map((h) => parseInt(h, 16))
  } else if (JACK_NEUTRAL_FAMILIES.includes(family) && grey) {
    const channels = grey(shade)?.trim().split(/[\s,]+/).map(Number)
    if (channels?.length === 3 && channels.every((n) => Number.isFinite(n))) target = channels
  }
  if (!target) return color

  if (parsed.alpha !== undefined) return `rgba(${target.join(', ')}, ${parsed.alpha})`
  return `#${target.map((n) => n.toString(16).padStart(2, '0')).join('')}`
}
