/**
 * Jack theme layer for Tailwind.
 *
 * Upstream templates use the `gray`, `dark`, `primary` and `accent` palettes
 * thousands of times. Instead of editing those templates, this wrapper points
 * every shade at a CSS variable (`--jack-<palette>-<shade>`, RGB channels), so the
 * actual colours live in `src/jack/theme/tokens.css`. Opacity modifiers such as
 * `bg-gray-300/50` keep working through `<alpha-value>`.
 *
 * Usage (frontend/tailwind.config.js): `export default withJackTheme({ ... })`.
 */

export const JACK_PALETTES = ['gray', 'dark', 'primary', 'accent']
export const JACK_SHADES = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950]

export function jackPalette(name) {
  return Object.fromEntries(
    JACK_SHADES.map((shade) => [shade, `rgb(var(--jack-${name}-${shade}) / <alpha-value>)`])
  )
}

const JACK_FONT_SANS = ['"Instrument Sans"']
const JACK_FONT_MONO = ['"Geist Mono"']
// Page titles and headline figures. CJK text uses a system serif where one exists
// and otherwise the sans stack, so Windows never falls back to a bitmap-era Song face.
export const JACK_FONT_DISPLAY = [
  '"Instrument Serif"',
  '"Songti SC"',
  '"Noto Serif CJK SC"',
  '"Noto Serif SC"',
  '"Source Han Serif SC"',
  '"PingFang SC"',
  '"Microsoft YaHei"',
  'serif'
]

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
