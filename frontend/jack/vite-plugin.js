/**
 * Jack theme Vite plugin.
 *
 * Keeps the Jack look out of upstream files so upstream merges stay clean:
 *
 * 1. Appends the Jack theme CSS to `src/style.css` before Tailwind compiles it.
 *    tokens.css and components.css use `@layer base` / `@layer components`, so
 *    their rules land after upstream's component classes and before utilities,
 *    which keeps upstream's "utilities win" behaviour intact. shell.css is
 *    unlayered and scoped to the Jack shell, so it follows the utilities.
 * 2. Swaps a small, fixed set of upstream components for Jack versions at module
 *    resolution time. The Jack files may still import the upstream originals.
 * 3. Warns (without failing) when upstream markup that the look depends on has
 *    changed; see style-hooks.js.
 *
 * Only `vite.config.ts` registers this plugin; vitest keeps testing the upstream
 * components unchanged. Plain JS (typed with JSDoc) so `vue-tsc -b` does not emit
 * build output next to it.
 */
import { existsSync, readFileSync } from 'node:fs'
import { basename, resolve } from 'node:path'
import { normalizePath } from 'vite'
import { checkJackStyleHooks } from './style-hooks.js'

/** @type {ReadonlyArray<readonly [string, string]>} Upstream module → Jack replacement, relative to the frontend root. */
export const JACK_COMPONENT_OVERRIDES = [
  ['src/components/layout/AuthLayout.vue', 'src/jack/layouts/JackAuthLayout.vue'],
  ['src/views/HomeView.vue', 'src/jack/views/JackHomeView.vue'],
  ['src/components/layout/AppLayout.vue', 'src/jack/layouts/JackAppLayout.vue']
]

/** Upstream stylesheet that receives the theme. */
export const JACK_STYLE_ENTRY = 'src/style.css'

/** Theme stylesheets, appended in this order. */
export const JACK_THEME_STYLES = [
  'src/jack/theme/fonts.css',
  'src/jack/theme/tokens.css',
  'src/jack/theme/components.css',
  'src/jack/theme/shell.css'
]

const REQUIRED_STYLE_DIRECTIVES = ['@tailwind base', '@tailwind components', '@tailwind utilities']

/** @param {string} id */
function stripQuery(id) {
  const index = id.indexOf('?')
  return normalizePath(index === -1 ? id : id.slice(0, index))
}

/** @param {string} file */
function stem(file) {
  return basename(file).replace(/\.vue$/, '')
}

/**
 * Returns human-readable problems; an empty list means the hooks still fit upstream.
 * @param {string} root
 * @returns {string[]}
 */
export function checkJackThemeTargets(root) {
  const problems = []
  for (const [target, replacement] of JACK_COMPONENT_OVERRIDES) {
    if (!existsSync(resolve(root, target))) problems.push(`override target is missing: ${target}`)
    if (!existsSync(resolve(root, replacement))) problems.push(`override replacement is missing: ${replacement}`)
  }
  const entry = resolve(root, JACK_STYLE_ENTRY)
  if (!existsSync(entry)) {
    problems.push(`style entry is missing: ${JACK_STYLE_ENTRY}`)
  } else {
    const css = readFileSync(entry, 'utf8')
    for (const directive of REQUIRED_STYLE_DIRECTIVES) {
      if (!css.includes(directive)) problems.push(`${JACK_STYLE_ENTRY} no longer contains "${directive}"`)
    }
  }
  for (const style of JACK_THEME_STYLES) {
    if (!existsSync(resolve(root, style))) problems.push(`theme stylesheet is missing: ${style}`)
  }
  return problems
}

/**
 * @param {string} source
 * @param {ReadonlyArray<{ file: string, css: string }>} styles
 * @returns {string}
 */
export function appendJackStyles(source, styles) {
  const blocks = styles.map(({ file, css }) => `\n/* ---- Jack theme: ${file} ---- */\n${css}`)
  return `${source}\n${blocks.join('\n')}`
}

/**
 * @param {string} root Frontend root directory.
 * @returns {import('vite').Plugin}
 */
export function jackTheme(root) {
  const styleEntry = normalizePath(resolve(root, JACK_STYLE_ENTRY))
  const themeFiles = JACK_THEME_STYLES.map((file) => normalizePath(resolve(root, file)))
  const overrides = new Map(
    JACK_COMPONENT_OVERRIDES.map(([target, replacement]) => [
      normalizePath(resolve(root, target)),
      normalizePath(resolve(root, replacement))
    ])
  )
  const replacements = new Set(overrides.values())
  const targetStems = new Set(JACK_COMPONENT_OVERRIDES.map(([target]) => stem(target)))

  return {
    name: 'jack-theme',
    enforce: 'pre',

    buildStart() {
      const problems = checkJackThemeTargets(root)
      if (problems.length > 0) {
        this.error(
          `[jack-theme] upstream layout changed; update frontend/jack/vite-plugin.js:\n- ${problems.join('\n- ')}`
        )
      }
      for (const { file, message } of checkJackStyleHooks(root)) {
        this.warn(`[jack-theme] ${file}: ${message}`)
      }
    },

    async resolveId(source, importer, options) {
      // Only plain module imports are swapped. Requests with a query, such as the
      // `HomeView.vue?vue&type=style` part Vue splits out of an SFC, belong to the
      // original file; swapping them makes the original import its replacement.
      if (!importer || source.includes('?') || !targetStems.has(stem(source))) return null
      // Jack replacements are allowed to import the upstream original.
      if (replacements.has(stripQuery(importer))) return null
      const resolved = await this.resolve(source, importer, { ...options, skipSelf: true })
      if (!resolved || resolved.id.includes('?')) return null
      return overrides.get(normalizePath(resolved.id)) ?? null
    },

    buildEnd(error) {
      if (error) return
      // A replaced upstream module must never depend on its own replacement.
      for (const [target, replacement] of overrides) {
        const imported = this.getModuleInfo(target)?.importedIds ?? []
        if (imported.some((id) => stripQuery(id) === replacement)) {
          this.error(`[jack-theme] ${target} imports its replacement ${replacement}; check resolveId`)
        }
      }
    },

    transform(code, id) {
      if (id.includes('?') || stripQuery(id) !== styleEntry) return null
      const styles = themeFiles.map((file, index) => {
        this.addWatchFile(file)
        return { file: JACK_THEME_STYLES[index], css: readFileSync(file, 'utf8') }
      })
      return { code: appendJackStyles(code, styles), map: null }
    }
  }
}
