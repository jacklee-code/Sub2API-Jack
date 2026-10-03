/**
 * Jack theme layer for Tailwind.
 *
 * Upstream templates use the `gray`, `dark`, `primary` and `accent` palettes
 * thousands of times. Instead of editing those templates, this wrapper points
 * every shade at a CSS variable (`--jack-<palette>-<shade>`, RGB channels), so the
 * actual colours live in `src/jack/theme/tokens.css`. Opacity modifiers such as
 * `bg-gray-300/50` keep working through `<alpha-value>`.
 *
 * Tailwind's status and accent families (red, amber, blue, ...) become the
 * Jack 文房 hues from palette.js (blue-black ink, verdigris, brass, madder, ink
 * grey), and the blue-tinted neutrals (slate, zinc, ...) follow the Jack greys,
 * so upstream badges and icon chips match the theme without template changes.
 *
 * Usage (frontend/tailwind.config.js): `export default withJackTheme({ ... })`.
 */
import { JACK_NEUTRAL_FAMILIES, JACK_UI_FAMILIES, jackFamilyColors } from './palette.js'

export const JACK_PALETTES = ['gray', 'dark', 'primary', 'accent']
export const JACK_SHADES = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950]

export function jackPalette(name) {
  return Object.fromEntries(
    JACK_SHADES.map((shade) => [shade, `rgb(var(--jack-${name}-${shade}) / <alpha-value>)`])
  )
}

// Latin glyphs come from the Instrument faces; Chinese from the bundled Noto SC
// faces (src/jack/theme/fonts-cjk.css), so every platform renders the same.
const JACK_FONT_SANS = ['"Instrument Sans"', '"Noto Sans SC Variable"']
const JACK_FONT_MONO = ['"Geist Mono"']
// Page titles and headline figures.
export const JACK_FONT_DISPLAY = ['"Instrument Serif"', '"Noto Serif SC Variable"', 'serif']

/**
 * Jack versions of Tailwind's colour families.
 * @returns {Record<string, Record<string, string>>}
 */
export function jackFamilyPalettes() {
  return {
    ...jackFamilyColors(JACK_UI_FAMILIES),
    ...Object.fromEntries(JACK_NEUTRAL_FAMILIES.map((family) => [family, jackPalette('gray')]))
  }
}

/**
 * @param {import('tailwindcss').Config} config
 * @returns {import('tailwindcss').Config}
 */
export function withJackTheme(config) {
  const theme = config.theme ?? {}
  const extend = theme.extend ?? {}
  const fontFamily = extend.fontFamily ?? {}

  return {
    ...config,
    theme: {
      ...theme,
      extend: {
        ...extend,
        colors: {
          ...jackFamilyPalettes(),
          ...(extend.colors ?? {}),
          ...Object.fromEntries(JACK_PALETTES.map((name) => [name, jackPalette(name)]))
        },
        fontFamily: {
          ...fontFamily,
          sans: [...JACK_FONT_SANS, ...(fontFamily.sans ?? ['system-ui', 'sans-serif'])],
          mono: [...JACK_FONT_MONO, ...(fontFamily.mono ?? ['ui-monospace', 'monospace'])],
          display: JACK_FONT_DISPLAY
        },
        boxShadow: {
          ...(extend.boxShadow ?? {}),
          glass: 'var(--jack-shadow-card)',
          'glass-sm': 'var(--jack-shadow-card)',
          card: 'var(--jack-shadow-card)',
          'card-hover': 'var(--jack-shadow-card-hover)',
          glow: 'var(--jack-shadow-glow)',
          'glow-lg': 'var(--jack-shadow-glow)'
        },
        backgroundImage: {
          ...(extend.backgroundImage ?? {}),
          'gradient-primary': 'var(--jack-btn-bg)',
          'mesh-gradient': 'var(--jack-glow)'
        },
        keyframes: {
          ...(extend.keyframes ?? {}),
          glow: {
            '0%': { boxShadow: 'var(--jack-shadow-glow)' },
            '100%': { boxShadow: 'var(--jack-shadow-glow-strong)' }
          }
        }
      }
    }
  }
}
