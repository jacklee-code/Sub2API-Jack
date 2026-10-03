// @vitest-environment node
import { describe, expect, it } from 'vitest'
import {
  JACK_CHART_FAMILIES,
  JACK_HUES,
  JACK_SHADES,
  JACK_UI_FAMILIES,
  jackChartColor,
  jackScale,
  oklchToHex
} from '../../../jack/palette.js'
import { TAILWIND_DEFAULTS } from '../../../jack/tailwind-defaults.js'

const HEX = /^#[0-9a-f]{6}$/

/** WCAG relative luminance, enough to check that shades get darker in order. */
function luminance(hex: string) {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((c) =>
    c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  )
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

describe('Jack 文房 palette', () => {
  it('builds eleven shades per hue, light to dark', () => {
    for (const hue of Object.keys(JACK_HUES) as Array<keyof typeof JACK_HUES>) {
      const scale = jackScale(hue)
      const values = JACK_SHADES.map((shade) => scale[shade])
      expect(values.every((value) => HEX.test(value)), hue).toBe(true)
      const lum = values.map(luminance)
      expect(lum, hue).toEqual([...lum].sort((a, b) => b - a))
    }
  })

  it('keeps 700-on-100 badge text readable for every interface hue', () => {
    for (const hue of new Set(Object.values(JACK_UI_FAMILIES))) {
      const scale = jackScale(hue)
      const [hi, lo] = [luminance(scale[100]), luminance(scale[700])]
      expect((hi + 0.05) / (lo + 0.05), hue).toBeGreaterThan(4.5)
    }
  })

  it('maps every Tailwind accent family for templates and charts', () => {
    const accents = Object.keys(TAILWIND_DEFAULTS).filter((f) => !['gray', 'slate', 'zinc', 'neutral', 'stone'].includes(f))
    expect(Object.keys(JACK_UI_FAMILIES).sort()).toEqual([...accents].sort())
    expect(Object.keys(JACK_CHART_FAMILIES).sort()).toEqual([...accents].sort())
  })

  it('gives the chart series upstream uses distinct hues', () => {
    const series = ['blue', 'emerald', 'amber', 'red', 'violet', 'pink', 'teal', 'orange', 'indigo', 'lime', 'cyan']
    expect(new Set(series.map((f) => JACK_CHART_FAMILIES[f])).size).toBe(series.length)
  })

  it('clamps out-of-gamut colours instead of producing invalid hex', () => {
    expect(oklchToHex(0.9, 0.4, 140)).toMatch(HEX)
  })
})

describe('jackChartColor', () => {
  const blue500 = jackScale('blueblack')[500]

  it('maps a Tailwind default to the Jack chart colour of the same shade', () => {
    expect(jackChartColor('#3b82f6')).toBe(blue500)
    expect(jackChartColor('#3B82F6')).toBe(blue500)
    expect(jackChartColor('rgb(59, 130, 246)')).toBe(blue500)
  })

  it('keeps alpha', () => {
    const [r, g, b] = [1, 3, 5].map((i) => parseInt(blue500.slice(i, i + 2), 16))
    expect(jackChartColor('rgba(59, 130, 246, 0.1)')).toBe(`rgba(${r}, ${g}, ${b}, 0.1)`)
  })

  it('sends neutrals through the grey resolver', () => {
    expect(jackChartColor('#374151', (shade) => (shade === '700' ? '62 64 69' : undefined))).toBe('#3e4045')
    expect(jackChartColor('#374151')).toBe('#374151')
  })

  it('leaves other colours alone', () => {
    for (const value of ['#123456', 'transparent', 'currentColor', 'hsl(0 100% 50%)', '#ffffff']) {
      expect(jackChartColor(value)).toBe(value)
    }
  })
})
