// Traditional Chinese messages: the upstream `zh` tree converted by
// jack/vite-plugin-zh-hant.js, with Jack wording overrides on top.
import converted from '@/i18n/locales/zh?jack-zh-hant'
import overrides from './zhHant.overrides'

type MessageTree = Record<string, unknown>

function isTree(value: unknown): value is MessageTree {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

export function applyOverrides(base: MessageTree, patch: MessageTree): MessageTree {
  const result: MessageTree = { ...base }
  for (const [key, value] of Object.entries(patch)) {
    const current = result[key]
    result[key] = isTree(current) && isTree(value) ? applyOverrides(current, value) : value
  }
  return result
}

export default applyOverrides(converted, overrides)
