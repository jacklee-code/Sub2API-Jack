#!/usr/bin/env node
/**
 * Reports Jack style hooks that an upstream sync broke. Always exits 0: the
 * result is a warning for the maintainer, never a reason to block a sync.
 *
 * In GitHub Actions each problem becomes a `::warning` annotation and the list is
 * added to the job summary. Run locally with `node jack/check-style-hooks.js`.
 */
import { appendFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { checkJackStyleHooks } from './style-hooks.js'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const warnings = checkJackStyleHooks(root)
const inActions = process.env.GITHUB_ACTIONS === 'true'

if (warnings.length === 0) {
  console.log('[jack-theme] all style hooks still match upstream')
} else {
  for (const { file, message } of warnings) {
    console.log(inActions ? `::warning file=frontend/${file}::Jack theme: ${message}` : `[jack-theme] ${file}: ${message}`)
  }
  if (process.env.GITHUB_STEP_SUMMARY) {
    const lines = warnings.map(({ file, message }) => `- \`frontend/${file}\`: ${message}`)
    appendFileSync(
      process.env.GITHUB_STEP_SUMMARY,
      `### Jack theme hooks need review\n\nThe app works, but part of the Jack look may have fallen back to upstream styling.\n\n${lines.join('\n')}\n`
    )
  }
}
