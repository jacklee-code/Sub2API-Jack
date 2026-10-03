import { baseCompile } from '@intlify/message-compiler'
import { describe, expect, it } from 'vitest'

import zh from '@/i18n/locales/zh'
import { availableLocales } from '@/i18n'
import zhHant, { applyOverrides } from '@/jack/i18n/zhHant'
import overrides from '@/jack/i18n/zhHant.overrides'
import { toTraditional } from '../../../jack/zh-hant-convert.js'

type Tree = Record<string, unknown>

function leaves(value: unknown, prefix = ''): Map<string, unknown> {
  const out = new Map<string, unknown>()
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    out.set(prefix, value)
    return out
  }
  for (const [key, child] of Object.entries(value as Tree)) {
    for (const [path, leaf] of leaves(child, prefix ? `${prefix}.${key}` : key)) out.set(path, leaf)
  }
  return out
}

const zhLeaves = leaves(zh)
const hantLeaves = leaves(zhHant)

describe('generated zh-Hant locale', () => {
  it('has exactly the upstream zh keys', () => {
    expect([...hantLeaves.keys()].sort()).toEqual([...zhLeaves.keys()].sort())
  })

  it('only overrides keys that exist upstream', () => {
    const stale = [...leaves(overrides).keys()].filter((key) => key && !zhLeaves.has(key))
    expect(stale).toEqual([])
  })

  it('converts every string with OpenCC and keeps ASCII intact', () => {
    const overridden = new Set(leaves(overrides).keys())
    const mismatched: string[] = []
    for (const [key, value] of zhLeaves) {
      if (overridden.has(key) || typeof value !== 'string') continue
      if (hantLeaves.get(key) !== toTraditional(value)) mismatched.push(key)
    }
    expect(mismatched).toEqual([])
  })

  it('produces Traditional characters', () => {
    expect(toTraditional('为了启用账号后台 {name}')).toBe('為了啟用帳號後台 {name}')
    const converted = [...zhLeaves].filter(([key, value]) => value !== hantLeaves.get(key))
    expect(converted.length).toBeGreaterThan(zhLeaves.size / 2)
  })

  it('compiles every message', () => {
    const errors: string[] = []
    for (const [key, value] of hantLeaves) {
      if (typeof value !== 'string') continue
      baseCompile(value, { onError: (err) => errors.push(`${key}: ${err.message}`) })
    }
    expect(errors).toEqual([])
  })

  it('applies overrides deeply without dropping siblings', () => {
    expect(applyOverrides({ a: { b: '1', c: '2' } }, { a: { c: '3' } })).toEqual({ a: { b: '1', c: '3' } })
  })

  it('is offered in the locale switcher', () => {
    expect(availableLocales.map((l) => [l.code, l.name])).toEqual([
      ['en', 'English'],
      ['zh', '简中'],
      ['zh-Hant', '繁中']
    ])
  })
})
