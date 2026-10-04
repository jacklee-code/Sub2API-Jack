import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'

import JackHomeView from '../views/JackHomeView.vue'
import { buildSnippets, snippetText, summarizePlaza } from '../home/catalog'
import type { ModelPlazaResponse } from '@/api/modelPlaza'

const { appStore, authStore, getModelPlaza } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: {} as Record<string, unknown>,
    siteName: 'Jack API',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn()
  },
  authStore: {
    isAuthenticated: false,
    isAdmin: false,
    checkAuth: vi.fn()
  },
  getModelPlaza: vi.fn()
}))

vi.mock('@/stores', () => ({ useAppStore: () => appStore, useAuthStore: () => authStore }))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
vi.mock('@/api/modelPlaza', () => ({ getModelPlaza }))
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn(async () => true) })
}))
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})
vi.mock('@/views/HomeView.vue', () => ({
  default: { name: 'UpstreamHomeView', template: '<div data-testid="upstream-home" />' }
}))

const stubs = {
  RouterLink: RouterLinkStub,
  LocaleSwitcher: { template: '<div />' },
  Icon: { template: '<span />' },
  InkCanvas: { template: '<canvas />' }
}

function plaza(models: Array<[string, string]>): ModelPlazaResponse {
  return {
    description: '',
    groups: [
      {
        id: 1,
        name: 'default',
        platform: 'anthropic',
        models: models.map(([platform, name]) => ({ platform, name, pricing: null, official_pricing: null }))
      } as unknown as ModelPlazaResponse['groups'][number]
    ]
  }
}

const mountHome = () => mount(JackHomeView, { global: { stubs } })

describe('JackHomeView model catalogue', () => {
  beforeEach(() => {
    appStore.cachedPublicSettings = { site_name: 'Jack API' }
    authStore.isAuthenticated = false
    getModelPlaza.mockReset()
  })

  it('stays out of the page while the Model Plaza is off', async () => {
    const wrapper = mountHome()
    await flushPromises()

    expect(getModelPlaza).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="jack-home-models"]').exists()).toBe(false)
  })

  it('does not ask anonymous visitors for a plaza that requires sign-in', async () => {
    appStore.cachedPublicSettings = { model_plaza_enabled: true, model_plaza_require_auth: true }
    const wrapper = mountHome()
    await flushPromises()

    expect(getModelPlaza).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="jack-home-models"]').exists()).toBe(false)
  })

  it('lists the plaza models and uses them in the snippets', async () => {
    appStore.cachedPublicSettings = { model_plaza_enabled: true }
    getModelPlaza.mockResolvedValue(plaza([['anthropic', 'claude-test-sonnet'], ['openai', 'gpt-test']]))
    const wrapper = mountHome()
    await flushPromises()

    const section = wrapper.find('[data-testid="jack-home-models"]')
    expect(section.exists()).toBe(true)
    expect(section.text()).toContain('claude-test-sonnet')
    expect(section.text()).toContain('gpt-test')
  })

  it('hides the catalogue when the plaza request fails', async () => {
    appStore.cachedPublicSettings = { model_plaza_enabled: true }
    getModelPlaza.mockRejectedValue(new Error('404'))
    const wrapper = mountHome()
    await flushPromises()

    expect(getModelPlaza).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-testid="jack-home-models"]').exists()).toBe(false)
  })

  it('invites visitors to register only when registration is open', () => {
    appStore.cachedPublicSettings = { registration_enabled: true }
    const open = mountHome().findAllComponents(RouterLinkStub).map((link) => link.props('to'))
    expect(open).toContain('/register')

    appStore.cachedPublicSettings = { registration_enabled: false }
    const closed = mountHome().findAllComponents(RouterLinkStub).map((link) => link.props('to'))
    expect(closed).not.toContain('/register')
  })

  it('shows the configured API base URL', () => {
    appStore.cachedPublicSettings = { api_base_url: 'https://api.example.test/' }
    expect(mountHome().text()).toContain('https://api.example.test')
  })
})

describe('home catalogue helpers', () => {
  it('deduplicates models per platform and orders the common platforms first', () => {
    const catalog = summarizePlaza({
      description: '',
      groups: [
        plaza([['gemini', 'gemini-x'], ['anthropic', 'claude-a']]).groups[0],
        plaza([['anthropic', 'claude-a'], ['anthropic', 'claude-b'], ['anthropic', '  ']]).groups[0]
      ]
    })

    expect(catalog.platforms.map((p) => p.platform)).toEqual(['anthropic', 'gemini'])
    expect(catalog.platforms[0].models).toEqual(['claude-a', 'claude-b'])
    expect(catalog.total).toBe(3)
  })

  it('copies commands without comments or shell prompts', () => {
    const [claudeCode] = buildSnippets('https://gw.test', 'sk-x', { platforms: [], total: 0 })
    const text = snippetText(claudeCode)

    expect(text).toContain('export ANTHROPIC_BASE_URL=https://gw.test')
    expect(text).not.toContain('#')
    expect(text).not.toMatch(/^\$ /m)
  })

  it('leaves the Codex model to the CLI default without a catalogue', () => {
    const codex = buildSnippets('https://gw.test', 'sk-x', { platforms: [], total: 0 }).find((s) => s.id === 'codex')!
    expect(snippetText(codex)).not.toMatch(/^model = /m)
  })
})
