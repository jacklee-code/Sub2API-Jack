// @vitest-environment node
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it, vi } from 'vitest'
import {
  JACK_COMPONENT_OVERRIDES,
  JACK_STYLE_ENTRY,
  JACK_THEME_STYLES,
  appendJackStyles,
  checkJackThemeTargets,
  jackTheme
} from '../../../jack/vite-plugin.js'
import { JACK_PALETTES, JACK_SHADES, withJackTheme } from '../../../jack/tailwind-theme.js'
import tailwindConfig from '../../../tailwind.config.js'

const frontendRoot = fileURLToPath(new URL('../../../', import.meta.url))
const fromRoot = (file: string) => resolve(frontendRoot, file).replace(/\\/g, '/')

type Hook = (...args: any[]) => any

function hook(name: 'resolveId' | 'transform'): Hook {
  const plugin = jackTheme(frontendRoot) as unknown as Record<string, Hook>
  return plugin[name]
}

function slotNames(file: string): string[] {
  const source = readFileSync(resolve(frontendRoot, file), 'utf8')
  const names = [...source.matchAll(/<slot(?:\s+name="([^"]+)")?/g)].map((match) => match[1] ?? 'default')
  return [...new Set(names)].sort()
}

describe('jack theme upstream hooks', () => {
  it('still finds every upstream file it hooks into', () => {
    expect(checkJackThemeTargets(frontendRoot)).toEqual([])
  })

  it('keeps the same slots as each overridden upstream layout', () => {
    for (const [target, replacement] of JACK_COMPONENT_OVERRIDES) {
      expect(slotNames(replacement), `${replacement} vs ${target}`).toEqual(slotNames(target))
    }
  })

  it('does not replace upstream layouts that declare props', () => {
    for (const [target] of JACK_COMPONENT_OVERRIDES) {
      const source = readFileSync(resolve(frontendRoot, target), 'utf8')
      expect(source, `${target} now declares props; mirror them in its Jack replacement`).not.toMatch(
        /defineProps|defineEmits|defineModel/
      )
    }
  })
})

describe('jack theme module overrides', () => {
  const [[authTarget, authReplacement], [homeTarget, homeReplacement]] = JACK_COMPONENT_OVERRIDES

  async function resolveFrom(source: string, importer: string, resolvedTarget: string) {
    const context = { resolve: vi.fn(async () => ({ id: fromRoot(resolvedTarget) })) }
    const result = await hook('resolveId').call(context, source, fromRoot(importer), {})
    return { result, context }
  }

  it('swaps upstream components for Jack replacements', async () => {
    const auth = await resolveFrom('./AuthLayout.vue', 'src/components/layout/index.ts', authTarget)
    expect(auth.result).toBe(fromRoot(authReplacement))

    const home = await resolveFrom('@/views/HomeView.vue', 'src/router/index.ts', homeTarget)
    expect(home.result).toBe(fromRoot(homeReplacement))
  })

  it('lets a Jack replacement import the upstream original', async () => {
    const { result, context } = await resolveFrom('@/views/HomeView.vue', homeReplacement, homeTarget)
    expect(result).toBeNull()
    expect(context.resolve).not.toHaveBeenCalled()
  })

  it('ignores unrelated imports without resolving them', async () => {
    const { result, context } = await resolveFrom('@/views/auth/LoginView.vue', 'src/router/index.ts', 'src/views/auth/LoginView.vue')
    expect(result).toBeNull()
    expect(context.resolve).not.toHaveBeenCalled()
  })
})

describe('jack theme stylesheet injection', () => {
  it('appends the theme after the upstream stylesheet', () => {
    const transform = hook('transform')
    const context = { addWatchFile: vi.fn() }
    const source = '@tailwind base;\n@tailwind components;\n@tailwind utilities;\n.upstream {}'
    const output = transform.call(context, source, fromRoot(JACK_STYLE_ENTRY)) as { code: string }

    expect(output.code.startsWith(source)).toBe(true)
    const positions = JACK_THEME_STYLES.map((file) => output.code.indexOf(`Jack theme: ${file}`))
    expect(positions.every((position) => position > source.length)).toBe(true)
    expect(positions).toEqual([...positions].sort((a, b) => a - b))
    expect(context.addWatchFile).toHaveBeenCalledTimes(JACK_THEME_STYLES.length)
  })

  it('leaves every other module untouched', () => {
    const transform = hook('transform')
    expect(transform.call({ addWatchFile: vi.fn() }, '.x {}', fromRoot('src/styles/onboarding.css'))).toBeNull()
  })

  it('keeps the upstream source verbatim', () => {
    expect(appendJackStyles('a', [{ file: 'f.css', css: 'b' }])).toBe('a\n\n/* ---- Jack theme: f.css ---- */\nb')
  })
})

describe('jack tailwind palette', () => {
  it('points every themed palette at CSS variables and keeps the rest of the config', () => {
    const config = withJackTheme({
      content: ['x'],
      theme: { extend: { colors: { brand: '#123456' }, spacing: { 18: '4.5rem' } } }
    })

    expect(config.content).toEqual(['x'])
    expect(config.theme?.extend?.spacing).toEqual({ 18: '4.5rem' })
    const colors = config.theme?.extend?.colors as Record<string, Record<number, string> | string>
    expect(colors.brand).toBe('#123456')
    for (const palette of JACK_PALETTES) {
      for (const shade of JACK_SHADES) {
        expect((colors[palette] as Record<number, string>)[shade]).toBe(
          `rgb(var(--jack-${palette}-${shade}) / <alpha-value>)`
        )
      }
    }
  })

  it('is applied by the real tailwind config', () => {
    const colors = tailwindConfig.theme?.extend?.colors as Record<string, Record<number, string>>
    expect(colors.primary[500]).toBe('rgb(var(--jack-primary-500) / <alpha-value>)')
    expect(colors.gray[50]).toBe('rgb(var(--jack-gray-50) / <alpha-value>)')
  })

  it('defines every palette channel the config refers to', () => {
    const tokens = readFileSync(resolve(frontendRoot, 'src/jack/theme/tokens.css'), 'utf8')
    for (const palette of JACK_PALETTES) {
      for (const shade of JACK_SHADES) {
        expect(tokens).toContain(`--jack-${palette}-${shade}:`)
      }
    }
  })
})
