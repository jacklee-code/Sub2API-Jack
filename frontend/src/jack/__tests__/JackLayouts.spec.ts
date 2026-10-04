import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

import JackHomeView from '../views/JackHomeView.vue'
import JackAuthLayout from '../layouts/JackAuthLayout.vue'

const { appStore, authStore } = vi.hoisted(() => ({
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
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => authStore
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
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

describe('JackHomeView', () => {
  beforeEach(() => {
    appStore.cachedPublicSettings = { site_name: 'Jack API', site_subtitle: 'Subtitle' }
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    authStore.checkAuth.mockClear()
  })

  it.each([
    ['custom home content', { home_content: '<h1>Custom</h1>' }],
    ['the compact home page', { compact_home_enabled: true }]
  ])('hands %s to the upstream home view', (_, settings) => {
    appStore.cachedPublicSettings = { ...appStore.cachedPublicSettings, ...settings }
    const wrapper = mount(JackHomeView, { global: { stubs } })

    expect(wrapper.find('[data-testid="upstream-home"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="jack-home"]').exists()).toBe(false)
  })

  it('renders the Jack home page and sends visitors to login', () => {
    const wrapper = mount(JackHomeView, { global: { stubs } })

    expect(wrapper.find('[data-testid="jack-home"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Subtitle')
    const destinations = wrapper.findAllComponents(RouterLinkStub).map((link) => link.props('to'))
    expect(destinations).toContain('/login')
    expect(authStore.checkAuth).toHaveBeenCalled()
  })

  it('sends signed-in administrators to the admin dashboard', () => {
    authStore.isAuthenticated = true
    authStore.isAdmin = true
    const wrapper = mount(JackHomeView, { global: { stubs } })

    const destinations = wrapper.findAllComponents(RouterLinkStub).map((link) => link.props('to'))
    expect(destinations).toContain('/admin/dashboard')
    expect(destinations).not.toContain('/login')
  })
})

describe('JackAuthLayout', () => {
  beforeEach(() => {
    appStore.cachedPublicSettings = { site_subtitle: 'Subtitle' }
    appStore.fetchPublicSettings.mockClear()
  })

  it('renders the page content and footer slots like the upstream layout', () => {
    const wrapper = mount(JackAuthLayout, {
      slots: {
        default: '<form data-testid="form" />',
        footer: '<a data-testid="footer-link">Sign up</a>'
      }
    })

    expect(wrapper.find('[data-testid="form"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="footer-link"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Jack API')
    expect(appStore.fetchPublicSettings).toHaveBeenCalled()
  })

  it('does not render an unsafe logo URL', () => {
    appStore.siteLogo = 'javascript:alert(1)'
    const wrapper = mount(JackAuthLayout)

    for (const img of wrapper.findAll('img')) {
      expect(img.attributes('src')).toBe('/logo.svg')
    }
    appStore.siteLogo = ''
  })
})
