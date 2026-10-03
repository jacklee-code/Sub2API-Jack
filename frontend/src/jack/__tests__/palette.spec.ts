// @vitest-environment node
import { describe, expect, it } from 'vitest'
import { JACK_MUTE_CHROMA, jackMute, scaleChroma } from '../../../jack/palette.js'

/** Rough OKLab lightness and chroma, enough to compare before and after muting. */
function lightnessAndChroma(hex: string) {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((c) =>
    c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  )
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b)
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b)
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b)
  const L = 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s
  const A = 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s
  const B = 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s
  return { L, C: Math.hypot(A, B) }
}

describe('jackMute', () => {
  it.each(['#3b82f6', '#10b981', '#ef4444', '#f59e0b', '#8b5cf6'])('keeps lightness and halves chroma of %s', (hex) => {
    const before = lightnessAndChroma(hex)
    const after = lightnessAndChroma(jackMute(hex))

    expect(after.L).toBeCloseTo(before.L, 2)
    expect(after.C / before.C).toBeCloseTo(JACK_MUTE_CHROMA, 1)
  })

  it('leaves neutrals unchanged', () => {
    for (const hex of ['#000000', '#ffffff', '#808080']) expect(jackMute(hex)).toBe(hex)
  })

  it('keeps the notation and alpha of rgb() and rgba() colours', () => {
    expect(jackMute('rgba(59, 130, 246, 0.1)')).toMatch(/^rgba\(\d+, \d+, \d+, 0\.1\)$/)
    expect(jackMute('rgb(16, 185, 129)')).toMatch(/^rgb\(\d+, \d+, \d+\)$/)
    expect(jackMute('rgb(59 130 246 / 50%)')).toMatch(/^rgba\(\d+, \d+, \d+, 50%\)$/)
  })

  it('expands short hex colours', () => {
    expect(jackMute('#f00')).toBe(jackMute('#ff0000'))
  })

  it('returns anything it cannot parse unchanged', () => {
    for (const value of ['transparent', 'currentColor', 'hsl(0 100% 50%)', 'var(--x)', '']) {
      expect(jackMute(value)).toBe(value)
    }
  })

  it('round-trips a colour when chroma is kept', () => {
    expect(scaleChroma([59, 130, 246], 1)).toEqual([59, 130, 246])
  })
})
