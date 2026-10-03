/**
 * Jack mineral palette: upstream's status and accent colours with the
 * saturation taken down to suit the Graphite look.
 *
 * `jackMute` keeps a colour's lightness and hue and scales its chroma in OKLab,
 * so red stays red and a 600 shade keeps its contrast against a 50 shade; it
 * just becomes the greyed, mineral version (brick, ochre, sage, steel, plum).
 * tailwind-theme.js applies it to Tailwind's colour families at build time and
 * src/jack/charts applies the same function to Chart.js colours at runtime.
 *
 * Plain JS (typed with JSDoc) so it runs in the Tailwind config and the browser,
 * and `vue-tsc -b` does not emit build output next to it.
 */

/** Share of the original chroma that survives. */
export const JACK_MUTE_CHROMA = 0.5

/** Tailwind colour families that templates use for status and accents. */
export const JACK_MUTED_FAMILIES = [
  'red',
  'orange',
  'amber',
  'yellow',
  'lime',
  'green',
  'emerald',
  'teal',
  'cyan',
  'sky',
  'blue',
  'indigo',
  'violet',
  'purple',
  'fuchsia',
  'pink',
  'rose'
]

/** Blue-tinted Tailwind neutrals that should follow the Jack greys instead. */
export const JACK_NEUTRAL_FAMILIES = ['slate', 'zinc', 'neutral', 'stone']

const HEX = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i
const RGB = /^rgba?\(\s*(\d+(?:\.\d+)?)[\s,]+(\d+(?:\.\d+)?)[\s,]+(\d+(?:\.\d+)?)\s*(?:[,/]\s*(\d*\.?\d+%?)\s*)?\)$/i

/** @param {number} c */
const toLinear = (c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)
/** @param {number} c */
const toSrgb = (c) => (c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055)
/** @param {number} c */
const clamp255 = (c) => Math.min(255, Math.max(0, Math.round(c * 255)))

/**
 * @param {[number, number, number]} rgb sRGB channels, 0-255
 * @param {number} factor chroma multiplier
 * @returns {[number, number, number]}
 */
export function scaleChroma([r8, g8, b8], factor) {
  const r = toLinear(r8 / 255)
  const g = toLinear(g8 / 255)
  const b = toLinear(b8 / 255)

  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b)
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b)
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b)

  const L = 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s
  const A = (1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s) * factor
  const B = (0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s) * factor

  const l2 = (L + 0.3963377774 * A + 0.2158037573 * B) ** 3
  const m2 = (L - 0.1055613458 * A - 0.0638541728 * B) ** 3
  const s2 = (L - 0.0894841775 * A - 1.291485548 * B) ** 3

  return [
    clamp255(toSrgb(4.0767416621 * l2 - 3.3077115913 * m2 + 0.2309699292 * s2)),
    clamp255(toSrgb(-1.2684380046 * l2 + 2.6097574011 * m2 - 0.3413193965 * s2)),
    clamp255(toSrgb(-0.0041960863 * l2 - 0.7034186147 * m2 + 1.707614701 * s2))
  ]
}

/** @param {number} n */
const hex2 = (n) => n.toString(16).padStart(2, '0')

/**
 * Returns the mineral version of a `#rgb`, `#rrggbb`, `rgb()` or `rgba()` colour in
 * the same notation (alpha kept). Anything else is returned unchanged.
 * @param {string} color
 * @param {number} [factor]
 * @returns {string}
 */
export function jackMute(color, factor = JACK_MUTE_CHROMA) {
  if (typeof color !== 'string') return color
  const value = color.trim()

  const hex = HEX.exec(value)
  if (hex) {
    const digits = hex[1].length === 3 ? [...hex[1]].map((d) => d + d).join('') : hex[1]
    const rgb = /** @type {[number, number, number]} */ ([0, 2, 4].map((i) => parseInt(digits.slice(i, i + 2), 16)))
    return `#${scaleChroma(rgb, factor).map(hex2).join('')}`
  }

  const rgbMatch = RGB.exec(value)
  if (rgbMatch) {
    const rgb = /** @type {[number, number, number]} */ (rgbMatch.slice(1, 4).map(Number))
    const [r, g, b] = scaleChroma(rgb, factor)
    return rgbMatch[4] === undefined ? `rgb(${r}, ${g}, ${b})` : `rgba(${r}, ${g}, ${b}, ${rgbMatch[4]})`
  }

  return color
}
