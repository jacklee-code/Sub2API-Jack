// @vitest-environment node
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
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
import {
  JACK_REVIEWED_UPSTREAM,
  JACK_STYLE_HOOKS,
  checkJackStyleHooks,
  upstreamFingerprint
} from '../../../jack/style-hooks.js'
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

  it('leaves the parts Vue splits out of an upstream SFC with the original file', async () => {
    // Regression: swapping `HomeView.vue?vue&type=style` made HomeView import
    // JackHomeView, a cycle that broke the production home page.
    for (const [target] of JACK_COMPONENT_OVERRIDES) {
      const styleRequest = `${fromRoot(target)}?vue&type=style&index=0&scoped=abc123&lang.css`
      const { result, context } = await resolveFrom(styleRequest, target, `${target}?vue&type=style`)
      expect(result).toBeNull()
      expect(context.resolve).not.toHaveBeenCalled()
    }
  })

  it('does not swap an import that resolves to a query request', async () => {
    const { result } = await resolveFrom('@/views/HomeView.vue', 'src/router/index.ts', `${homeTarget}?raw`)
    expect(result).toBeNull()
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

describe('jack style hooks (warning only)', () => {
  function withFakeRoot(files: Record<string, string>, run: (root: string) => void) {
    const root = mkdtempSync(join(tmpdir(), 'jack-hooks-'))
    try {
      for (const [file, content] of Object.entries(files)) {
        mkdirSync(dirname(join(root, file)), { recursive: true })
        writeFileSync(join(root, file), content)
      }
      run(root)
    } finally {
      rmSync(root, { recursive: true, force: true })
    }
  }

  it('finds every hook in the current upstream sources', () => {
    expect(checkJackStyleHooks(frontendRoot)).toEqual([])
  })

  it('reports each missing hook with its purpose', () => {
    const [{ file, hooks }] = JACK_STYLE_HOOKS
    const reviewed = Object.fromEntries(
      JACK_REVIEWED_UPSTREAM.map((entry) => [entry.file, readFileSync(resolve(frontendRoot, entry.file), 'utf8')])
    )
    withFakeRoot({ ...reviewed, [file]: '<template><aside /></template>' }, (root) => {
      const warnings = checkJackStyleHooks(root).filter((warning) => warning.file === file)
      expect(warnings).toHaveLength(hooks.length)
      expect(warnings[0].message).toContain(hooks[0][1])
    })
  })

  it('reports a missing upstream file', () => {
    withFakeRoot({}, (root) => {
      const files = checkJackStyleHooks(root).map((warning) => warning.file)
      expect(files).toEqual(JACK_STYLE_HOOKS.map((entry) => entry.file))
    })
  })

  it('asks for a review when a mirrored upstream file changes', () => {
    const [{ file }] = JACK_REVIEWED_UPSTREAM
    const original = readFileSync(resolve(frontendRoot, file), 'utf8')
    withFakeRoot({ [file]: `${original}\n<!-- upstream change -->` }, (root) => {
      const warnings = checkJackStyleHooks(root).filter((warning) => warning.file === file)
      expect(warnings).toHaveLength(1)
      expect(warnings[0].message).toContain('changed upstream')
    })
  })

  it('ignores line-ending differences when fingerprinting', () => {
    expect(upstreamFingerprint('a\r\nb\r\n')).toBe(upstreamFingerprint('a\nb\n'))
  })

  it('warns from the build instead of failing it', () => {
    const plugin = jackTheme(frontendRoot) as unknown as Record<string, Hook>
    const context = { error: vi.fn(), warn: vi.fn() }
    plugin.buildStart.call(context)
    expect(context.error).not.toHaveBeenCalled()
    expect(context.warn).not.toHaveBeenCalled()
  })
})
