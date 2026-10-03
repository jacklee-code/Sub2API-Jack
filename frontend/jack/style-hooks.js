/**
 * Upstream markup that the Jack look depends on, and a non-fatal check for it.
 *
 * The Jack shell and theme restyle upstream components through their class
 * names (see src/jack/theme/shell.css and components.css). When an upstream
 * sync renames one of these, the app still works but part of the look silently
 * falls back to upstream styling. This check reports that as a warning only, so
 * an upstream release (possibly a security fix) is never blocked by styling.
 *
 * Hard requirements, such as a replaced component changing its slots or props,
 * stay in vite-plugin.js and fail the build.
 *
 * Plain JS (typed with JSDoc) so `vue-tsc -b` does not emit build output next to it.
 */
import { createHash } from 'node:crypto'
import { existsSync, readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'
import { TAILWIND_DEFAULTS } from './tailwind-defaults.js'

/**
 * Text that must still appear in each upstream file, with what Jack uses it for.
 * @type {ReadonlyArray<{ file: string, hooks: ReadonlyArray<readonly [string, string]> }>}
 */
export const JACK_STYLE_HOOKS = [
  {
    file: 'src/components/layout/AppSidebar.vue',
    hooks: [
      ['class="sidebar"', 'sidebar root sits transparent on the graphite canvas'],
      ['sidebar-header', 'brand row height and border'],
      ['sidebar-logo', 'logo mark'],
      ['sidebar-brand-title', 'serif site name'],
      ['sidebar-nav', 'menu padding'],
      ['sidebar-section-title', 'section label'],
      ['sidebar-link-active', 'selected menu item'],
      ['border-l', 'rule beside expanded group children'],
      ['mt-auto border-t', 'theme / collapse footer rule']
    ]
  },
  {
    file: 'src/components/layout/AppHeader.vue',
    hooks: [
      ['<header', 'AppHeader renders a single <header> root that receives the jack-toolbar class'],
      ['h-16', 'toolbar row height'],
      ['<h1', 'serif page title and the route path shown above it']
    ]
  },
  {
    file: 'src/components/layout/TablePageLayout.vue',
    hooks: [
      ['table-page-layout', 'table page height inside the sheet'],
      ['calc(100vh - 64px - 4rem)', 'Jack recomputes this height for the sheet; re-check shell.css if it changed']
    ]
  },
  {
    file: 'src/views/admin/DashboardView.vue',
    hooks: [
      ['text-xl font-bold', 'serif headline figures'],
      ['rounded-lg bg-', 'icon chips beside the figures']
    ]
  },
  {
    file: 'src/components/user/dashboard/UserDashboardStats.vue',
    hooks: [['text-xl font-bold', 'serif headline figures']]
  },
  {
    file: 'package.json',
    hooks: [['"chart.js"', 'charts take the mineral palette through src/jack/charts/chartjs.ts']]
  }
]

/**
 * Upstream files Jack mirrors by hand, with the SHA-256 (LF line endings) of the
 * version that was last reviewed. A changed hash means upstream changed the
 * file: review the Jack counterpart, then update the hash here.
 * @type {ReadonlyArray<{ file: string, sha256: string, review: string }>}
 */
export const JACK_REVIEWED_UPSTREAM = [
  {
    file: 'src/components/layout/AppLayout.vue',
    sha256: 'b70d197cd962465baa3b063eaba12aef45ebbe61ebf2d0f28f249f43d45487fa',
    review: 'src/jack/layouts/JackAppLayout.vue mirrors its script (onboarding tour, replayTour)'
  }
]

/** @param {string} source */
export function upstreamFingerprint(source) {
  return createHash('sha256').update(source.replace(/\r\n/g, '\n')).digest('hex')
}

/**
 * Returns `{ file, message }` warnings; an empty list means every hook still fits upstream.
 * @param {string} root Frontend root directory.
 * @returns {Array<{ file: string, message: string }>}
 */
export function checkJackStyleHooks(root) {
  const warnings = []
  for (const { file, hooks } of JACK_STYLE_HOOKS) {
    const path = resolve(root, file)
    if (!existsSync(path)) {
      warnings.push({ file, message: 'file is missing; the Jack styles that target it no longer apply' })
      continue
    }
    const source = readFileSync(path, 'utf8')
    for (const [needle, purpose] of hooks) {
      if (!source.includes(needle)) warnings.push({ file, message: `no longer contains "${needle}" (${purpose})` })
    }
  }
  for (const { file, sha256, review } of JACK_REVIEWED_UPSTREAM) {
    const path = resolve(root, file)
    if (!existsSync(path)) continue
    if (upstreamFingerprint(readFileSync(path, 'utf8')) !== sha256) {
      warnings.push({ file, message: `changed upstream; review ${review}, then update its hash in frontend/jack/style-hooks.js` })
    }
  }
  warnings.push(...checkTailwindDefaults(root))
  return warnings
}

/**
 * Chart colours are recognised through the static copy in tailwind-defaults.js.
 * Warns when the installed Tailwind's default colours no longer match it.
 * @param {string} root
 * @returns {Array<{ file: string, message: string }>}
 */
export function checkTailwindDefaults(root) {
  let installed
  try {
    installed = createRequire(resolve(root, 'package.json'))('tailwindcss/colors')
  } catch {
    return []
  }
  const changed = []
  for (const [family, shades] of Object.entries(TAILWIND_DEFAULTS)) {
    for (const [shade, hex] of Object.entries(shades)) {
      if (installed?.[family]?.[shade]?.toLowerCase() !== hex) changed.push(`${family}-${shade}`)
    }
  }
  if (changed.length === 0) return []
  return [
    {
      file: 'jack/tailwind-defaults.js',
      message: `differs from the installed Tailwind colours (${changed.slice(0, 5).join(', ')}${changed.length > 5 ? ', ...' : ''}); regenerate it so charts keep the Jack palette`
    }
  ]
}
