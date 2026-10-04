// One-time locale migration for the zh-Hant release. Before 繁中 existed, a
// visitor with a Traditional Chinese browser could only pick 简中, so a saved
// `zh` from that time is re-detected once. Saved English and later choices
// are kept.
const MIGRATION_KEY = 'jack_locale_zh_hant_migrated'

export function isTraditionalChinese(language: string): boolean {
  return /^zh-(hant|tw|hk|mo)\b/.test(language.toLowerCase())
}

export function migrateSavedLocale(localeKey: string, browserLanguage: string): string | null {
  const saved = localStorage.getItem(localeKey)
  if (localStorage.getItem(MIGRATION_KEY)) return saved
  localStorage.setItem(MIGRATION_KEY, '1')
  if (saved === 'zh' && isTraditionalChinese(browserLanguage)) {
    localStorage.setItem(localeKey, 'zh-Hant')
    return 'zh-Hant'
  }
  return saved
}
