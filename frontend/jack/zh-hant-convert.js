/**
 * Simplified → generic Traditional Chinese conversion for the zh-Hant locale.
 *
 * OpenCC `cn` → `tw` changes characters to the common Traditional forms (為, 啟,
 * 裡) without regional vocabulary (软件 becomes 軟件, not 軟體). The character
 * table then replaces forms that read as unusual in both Taiwan and Hong Kong.
 * Kept free of Vite imports so tests under jsdom can load it.
 */
import * as OpenCC from 'opencc-js/cn2t'

/** @type {ReadonlyArray<readonly [string, string]>} */
export const JACK_ZH_HANT_CHARACTERS = [
  ['臺', '台'],
  ['賬', '帳']
]

/** @type {((text: string) => string) | undefined} */
let converter

/**
 * @param {string} text
 * @returns {string}
 */
export function toTraditional(text) {
  converter ??= OpenCC.Converter({ from: 'cn', to: 'tw' })
  let result = converter(text)
  for (const [from, to] of JACK_ZH_HANT_CHARACTERS) result = result.replaceAll(from, to)
  return result
}
