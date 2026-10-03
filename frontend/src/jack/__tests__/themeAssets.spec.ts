// @vitest-environment node
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it, vi } from 'vitest'
import tailwindColors from 'tailwindcss/colors.js'
import { JACK_MODULE_WRAPPERS, jackTheme } from '../../../jack/vite-plugin.js'
import { JACK_MUTED_FAMILIES, JACK_NEUTRAL_FAMILIES, jackMute } from '../../../jack/palette.js'
import tailwindConfig from '../../../tailwind.config.js'

const frontendRoot = fileURLToPath(new URL('../../../', import.meta.url))
const fromRoot = (file: string) => resolve(frontendRoot, file).replace(/\\/g, '/')

type Hook = (...args: any[]) => any

describe('chart.js wrapper', () => {
  const resolveId = (jackTheme(frontendRoot) as unknown as Record<string, Hook>).resolveId
  const [[specifier, wrapper]] = JACK_MODULE_WRAPPERS

  it('gives upstream code the Jack wrapper', async () => {
    const context = { resolve: vi.fn() }
    const result = await resolveId.call(context, specifier, fromRoot('src/components/charts/TokenUsageTrend.vue'), {})

    expect(result).toBe(fromRoot(wrapper))
    expect(context.resolve).not.toHaveBeenCalled()
  })

  it('lets the wrapper import the real package', async () => {
    const result = await resolveId.call({ resolve: vi.fn() }, specifier, fromRoot(wrapper), {})
    expect(result).toBeNull()
  })

  it('leaves sub-path imports alone', async () => {
    const result = await resolveId.call({ resolve: vi.fn() }, `${specifier}/helpers`, fromRoot('src/x.ts'), {})
    expect(result).toBeNull()
  })
})

describe('mineral Tailwind palette', () => {
  const colors = tailwindConfig.theme?.extend?.colors as Record<string, Record<string, string>>
  const defaults = tailwindColors as unknown as Record<string, Record<string, string>>

  it('replaces every status and accent family with its muted version', () => {
    for (const family of JACK_MUTED_FAMILIES) {
      for (const [shade, value] of Object.entries(defaults[family])) {
        expect(colors[family][shade], `${family}-${shade}`).toBe(jackMute(value))
      }
    }
  })

  it('points blue-tinted neutrals at the Jack greys', () => {
    for (const family of JACK_NEUTRAL_FAMILIES) {
      expect(colors[family][500]).toBe('rgb(var(--jack-gray-500) / <alpha-value>)')
    }
  })
})

describe('bundled Chinese fonts', () => {
  const css = readFileSync(resolve(frontendRoot, 'src/jack/theme/fonts-cjk.css'), 'utf8')
  const urls = [...css.matchAll(/url\('@\/([^']+)'\)/g)].map((match) => match[1])

  it('declares both faces', () => {
    expect(css).toContain("font-family: 'Noto Serif SC Variable'")
    expect(css).toContain("font-family: 'Noto Sans SC Variable'")
  })

  it('points every slice at a vendored file', () => {
    expect(urls.length).toBeGreaterThan(100)
    for (const url of urls) expect(existsSync(resolve(frontendRoot, 'src', url)), url).toBe(true)
  })

  it('leaves emoji to the system font', () => {
    const ranges = [...css.matchAll(/unicode-range:\s*([^;]+);/g)].flatMap((match) => match[1].split(','))
    const starts = ranges.map((token) => parseInt(token.trim().replace(/^U\+/i, '').split('-')[0], 16))
    expect(starts.some((start) => start >= 0x1f000)).toBe(false)
  })
})
