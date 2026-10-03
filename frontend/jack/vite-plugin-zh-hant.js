/**
 * Jack Traditional Chinese (zh-Hant) locale plugin.
 *
 * Upstream only maintains `en` and `zh` (Simplified). Instead of a hand-kept copy
 * that drifts on every upstream sync, the zh-Hant messages are the upstream `zh`
 * modules converted by `toTraditional` (zh-hant-convert.js) while Vite transforms
 * them:
 *
 * - `@/i18n/locales/zh?jack-zh-hant` resolves to the upstream module plus the
 *   marker query. Relative imports from a marked module inside the `zh` folder
 *   carry the marker, so the whole message tree is a separate set of modules.
 * - Marked modules have their source converted before TypeScript is stripped.
 *   Only CJK characters change; keys, `{placeholders}` and markup are ASCII and
 *   stay as they are.
 *
 * `src/jack/i18n/zhHant.ts` imports the converted tree and applies the Jack
 * wording overrides. Both `vite.config.ts` and `vitest.config.ts` register this
 * plugin. Plain JS (typed with JSDoc) for the same reason as `vite-plugin.js`.
 */
import { resolve } from 'node:path'
import { normalizePath } from 'vite'
import { toTraditional } from './zh-hant-convert.js'

export const ZH_HANT_QUERY = 'jack-zh-hant'

/** Upstream Simplified Chinese locale folder, relative to the frontend root. */
export const ZH_LOCALE_DIR = 'src/i18n/locales/zh'

const MARKER = `?${ZH_HANT_QUERY}`

/** @param {string} id */
function splitQuery(id) {
  const index = id.indexOf('?')
  return index === -1 ? [id, ''] : [id.slice(0, index), id.slice(index + 1)]
}

/** @param {string} id */
function isMarked(id) {
  return splitQuery(id)[1].split('&').includes(ZH_HANT_QUERY)
}

/**
 * @param {string} root Frontend root directory.
 * @returns {import('vite').Plugin}
 */
export function jackZhHant(root) {
  const localeDir = normalizePath(resolve(root, ZH_LOCALE_DIR)) + '/'

  /** @param {string} id */
  const inLocaleDir = (id) => normalizePath(splitQuery(id)[0]).startsWith(localeDir)

  return {
    name: 'jack-zh-hant',
    enforce: 'pre',

    async resolveId(source, importer, options) {
      let request = source
      if (source.endsWith(MARKER)) {
        request = source.slice(0, -MARKER.length)
      } else if (!(importer && isMarked(importer) && source.startsWith('.'))) {
        return null
      }
      const resolved = await this.resolve(request, importer, { ...options, skipSelf: true })
      if (!resolved || resolved.external || !inLocaleDir(resolved.id)) {
        if (request !== source) this.error(`[jack-zh-hant] ${source} is outside ${ZH_LOCALE_DIR}`)
        return null
      }
      return `${splitQuery(resolved.id)[0]}${MARKER}`
    },

    transform(code, id) {
      if (!isMarked(id) || !inLocaleDir(id)) return null
      return { code: toTraditional(code), map: null }
    }
  }
}
