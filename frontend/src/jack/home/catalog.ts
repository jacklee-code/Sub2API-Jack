// Data helpers for the Jack home page: the model catalogue read from the Model
// Plaza and the connection snippets. Pure functions, so they are unit tested
// without mounting the view.
import type { ModelPlazaResponse } from '@/api/modelPlaza'
import { platformLabel } from '@/utils/platformColors'

export interface CatalogPlatform {
  platform: string
  label: string
  models: string[]
}

export interface Catalog {
  platforms: CatalogPlatform[]
  total: number
}

/** Providers shown in the hero diagram when no catalogue is available. */
export const DEFAULT_PLATFORMS = ['anthropic', 'openai', 'gemini', 'antigravity']

const PLATFORM_ORDER = [...DEFAULT_PLATFORMS, 'grok', 'deepseek', 'kimi', 'zhipu', 'minimax']

/**
 * Unique model names per platform across every visible group. Platforms follow
 * a fixed order (the common gateways first); models keep the order the plaza
 * lists them in.
 */
export function summarizePlaza(data: ModelPlazaResponse | null | undefined): Catalog {
  const byPlatform = new Map<string, Set<string>>()
  for (const group of data?.groups ?? []) {
    for (const model of group.models ?? []) {
      const platform = (model.platform || group.platform || '').trim()
      const name = (model.name || '').trim()
      if (!platform || !name) continue
      if (!byPlatform.has(platform)) byPlatform.set(platform, new Set())
      byPlatform.get(platform)!.add(name)
    }
  }

  const rank = (p: string) => {
    const index = PLATFORM_ORDER.indexOf(p)
    return index === -1 ? PLATFORM_ORDER.length : index
  }
  const platforms = [...byPlatform.entries()]
    .sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b))
    .map(([platform, models]) => ({ platform, label: platformLabel(platform), models: [...models] }))

  return { platforms, total: platforms.reduce((sum, p) => sum + p.models.length, 0) }
}

/** Display colour for a platform, from the Jack 文房 hues (see jack/palette.js). */
export function platformInk(platform: string): string {
  switch (platform) {
    case 'anthropic':
      return 'oklch(0.62 0.11 48)' // 赭石
    case 'openai':
      return 'oklch(0.6 0.075 172)' // 銅綠
    case 'gemini':
      return 'oklch(0.6 0.09 258)' // 藍黑
    case 'antigravity':
      return 'oklch(0.58 0.065 318)' // 茄紫
    default:
      return 'oklch(0.6 0.02 300)' // 墨灰
  }
}

function firstMatching(catalog: Catalog, platform: string, patterns: RegExp[]): string | undefined {
  const models = catalog.platforms.find((p) => p.platform === platform)?.models ?? []
  for (const pattern of patterns) {
    const hit = models.find((m) => pattern.test(m))
    if (hit) return hit
  }
  return models[0]
}

export interface SnippetLine {
  /** Plain text of the line, used for copying. */
  text: string
  /** Visual role of the line. */
  kind?: 'comment' | 'prompt'
}

export interface Snippet {
  id: 'claude-code' | 'codex' | 'python' | 'curl'
  label: string
  lines: SnippetLine[]
}

/** The text a snippet copies: comments and prompts stripped of decoration. */
export function snippetText(snippet: Snippet): string {
  return snippet.lines
    .filter((line) => line.kind !== 'comment')
    .map((line) => (line.kind === 'prompt' ? line.text.replace(/^\$ /, '') : line.text))
    .join('\n')
    .trim()
}

/**
 * Connection examples for the current site. Model names come from the
 * catalogue when it is available; otherwise the snippets fall back to a common
 * Claude model, and the Codex config leaves the model to the CLI's default.
 */
export function buildSnippets(baseUrl: string, key: string, catalog: Catalog): Snippet[] {
  const claude = firstMatching(catalog, 'anthropic', [/sonnet/i, /opus/i, /claude/i]) ?? 'claude-sonnet-4-5'
  const openai = firstMatching(catalog, 'openai', [/codex/i, /^gpt/i])
  const comment = (text: string): SnippetLine => ({ text, kind: 'comment' })
  const prompt = (text: string): SnippetLine => ({ text: `$ ${text}`, kind: 'prompt' })
  const line = (text: string): SnippetLine => ({ text })

  return [
    {
      id: 'claude-code',
      label: 'Claude Code',
      lines: [
        comment('# ~/.zshrc or ~/.bashrc'),
        prompt(`export ANTHROPIC_BASE_URL=${baseUrl}`),
        prompt(`export ANTHROPIC_AUTH_TOKEN=${key}`),
        line(''),
        prompt('claude')
      ]
    },
    {
      id: 'codex',
      label: 'Codex CLI',
      lines: [
        comment('# ~/.codex/config.toml'),
        line('model_provider = "jack"'),
        ...(openai ? [line(`model = "${openai}"`)] : []),
        line(''),
        line('[model_providers.jack]'),
        line('name = "Jack"'),
        line(`base_url = "${baseUrl}/v1"`),
        line('wire_api = "responses"'),
        line('env_key = "JACK_API_KEY"'),
        line(''),
        prompt(`export JACK_API_KEY=${key} && codex`)
      ]
    },
    {
      id: 'python',
      label: 'Python',
      lines: [
        comment('# pip install anthropic'),
        line('import anthropic'),
        line(''),
        line(`client = anthropic.Anthropic(base_url="${baseUrl}", api_key="${key}")`),
        line('message = client.messages.create('),
        line(`    model="${claude}",`),
        line('    max_tokens=1024,'),
        line('    messages=[{"role": "user", "content": "Hello"}],'),
        line(')'),
        line('print(message.content[0].text)')
      ]
    },
    {
      id: 'curl',
      label: 'curl',
      lines: [
        prompt(`curl ${baseUrl}/v1/messages \\`),
        line(`  -H "x-api-key: ${key}" \\`),
        line('  -H "anthropic-version: 2023-06-01" \\'),
        line('  -H "content-type: application/json" \\'),
        line(`  -d '{"model": "${claude}", "max_tokens": 1024,`),
        line(`       "messages": [{"role": "user", "content": "Hello"}]}'`)
      ]
    }
  ]
}
