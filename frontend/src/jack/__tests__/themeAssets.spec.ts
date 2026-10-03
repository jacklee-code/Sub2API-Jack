// @vitest-environment node
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it, vi } from 'vitest'
import { JACK_MODULE_WRAPPERS, jackTheme } from '../../../jack/vite-plugin.js'
import { JACK_NEUTRAL_FAMILIES, JACK_UI_FAMILIES, jackScale } from '../../../jack/palette.js'
import { checkTailwindDefaults } from '../../../jack/style-hooks.js'
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

describe('Jack 文房 Tailwind palette', () => {
  const colors = tailwindConfig.theme?.extend?.colors as Record<string, Record<string, string>>

  it('replaces every status and accent family with its Jack hue', () => {
    for (const [family, hue] of Object.entries(JACK_UI_FAMILIES)) {
      expect(colors[family], family).toEqual(jackScale(hue))
    }
  })

  it('points blue-tinted neutrals at the Jack greys', () => {
    for (const family of JACK_NEUTRAL_FAMILIES.filter((name) => name !== 'gray')) {
      expect(colors[family][500]).toBe('rgb(var(--jack-gray-500) / <alpha-value>)')
    }
  })
})

describe('Tailwind default colour snapshot', () => {
  it('matches the installed Tailwind', () => {
    expect(checkTailwindDefaults(frontendRoot)).toEqual([])
  })

  it('warns when the installed Tailwind colours differ', () => {
    const root = mkdtempSync(join(tmpdir(), 'jack-tw-'))
    try {
      writeFileSync(join(root, 'package.json'), '{}')
      mkdirSync(join(root, 'node_modules/tailwindcss'), { recursive: true })
      writeFileSync(join(root, 'node_modules/tailwindcss/colors.js'), "module.exports = { blue: { 500: '#000000' } }")
      const [warning] = checkTailwindDefaults(root)
      expect(warning.file).toBe('jack/tailwind-defaults.js')
      expect(warning.message).toContain('regenerate')
    } finally {
      rmSync(root, { recursive: true, force: true })
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
