import { beforeEach, describe, expect, it } from 'vitest'

import { isTraditionalChinese, migrateSavedLocale } from '@/jack/i18n/localeMigration'

const KEY = 'sub2api_locale'

describe('zh-Hant locale migration', () => {
  beforeEach(() => localStorage.clear())

  it('detects Traditional Chinese browser languages', () => {
    for (const lang of ['zh-TW', 'zh-HK', 'zh-MO', 'zh-Hant', 'zh-Hant-TW']) {
      expect(isTraditionalChinese(lang), lang).toBe(true)
    }
    for (const lang of ['zh', 'zh-CN', 'zh-SG', 'zh-Hans-HK', 'en-US']) {
      expect(isTraditionalChinese(lang), lang).toBe(false)
    }
  })

  it('moves a saved zh to zh-Hant once for Traditional browsers', () => {
    localStorage.setItem(KEY, 'zh')
    expect(migrateSavedLocale(KEY, 'zh-TW')).toBe('zh-Hant')
    expect(localStorage.getItem(KEY)).toBe('zh-Hant')

    // A later explicit choice of 简中 is kept.
    localStorage.setItem(KEY, 'zh')
    expect(migrateSavedLocale(KEY, 'zh-TW')).toBe('zh')
  })

  it('keeps zh for Simplified browsers and keeps English', () => {
    localStorage.setItem(KEY, 'zh')
    expect(migrateSavedLocale(KEY, 'zh-CN')).toBe('zh')

    localStorage.clear()
    localStorage.setItem(KEY, 'en')
    expect(migrateSavedLocale(KEY, 'zh-HK')).toBe('en')
  })

  it('leaves first-time visitors to browser detection', () => {
    expect(migrateSavedLocale(KEY, 'zh-HK')).toBeNull()
    expect(localStorage.getItem(KEY)).toBeNull()
  })
})
