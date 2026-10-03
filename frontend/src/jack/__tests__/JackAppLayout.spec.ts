import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import JackAppLayout from '../layouts/JackAppLayout.vue'

const { appStore, authStore, onboardingStore, route, tour } = vi.hoisted(() => ({
  appStore: { sidebarCollapsed: false },
  authStore: { user: { role: 'admin' } as { role: string } | null },
  onboardingStore: { setReplayCallback: vi.fn() },
  route: { path: '/admin/dashboard' },
  tour: { replayTour: vi.fn(), options: [] as unknown[] }
}))

vi.mock('@/styles/onboarding.css', () => ({}))
vi.mock('@/stores', () => ({ useAppStore: () => appStore }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('@/stores/onboarding', () => ({ useOnboardingStore: () => onboardingStore }))
vi.mock('vue-router', () => ({ useRoute: () => route }))
vi.mock('@/composables/useOnboardingTour', () => ({
  useOnboardingTour: (options: unknown) => {
    tour.options.push(options)
    return { replayTour: tour.replayTour }
  }
}))
vi.mock('@/components/layout/AppSidebar.vue', () => ({
  default: { name: 'AppSidebar', template: '<aside data-testid="sidebar" class="sidebar" />' }
}))
vi.mock('@/components/layout/AppHeader.vue', () => ({
  default: { name: 'AppHeader', template: '<header data-testid="header" />' }
}))

function mountLayout() {
  return mount(JackAppLayout, { slots: { default: '<p data-testid="page">Page</p>' } })
}

describe('JackAppLayout', () => {
  beforeEach(() => {
    appStore.sidebarCollapsed = false
    authStore.user = { role: 'admin' }
    route.path = '/admin/dashboard'
    onboardingStore.setReplayCallback.mockClear()
    tour.options.length = 0
  })

  it('composes the upstream sidebar, header and page inside the Jack shell', () => {
    const wrapper = mountLayout()

    expect(wrapper.classes()).toContain('jack-shell')
    // The sidebar sits on the graphite canvas, so it always renders in dark mode.
    expect(wrapper.find('.jack-rail.dark [data-testid="sidebar"]').exists()).toBe(true)
    expect(wrapper.find('.jack-sheet [data-testid="header"]').classes()).toContain('jack-toolbar')
    expect(wrapper.find('.jack-sheet .jack-content [data-testid="page"]').exists()).toBe(true)
  })

  it('follows the collapsed sidebar state', () => {
    expect(mountLayout().classes()).not.toContain('jack-shell--collapsed')

    appStore.sidebarCollapsed = true
    expect(mountLayout().classes()).toContain('jack-shell--collapsed')
  })

  it.each([
    ['/admin/dashboard', "'admin · dashboard'"],
    ['/admin/channels/pricing', "'admin · channels · pricing'"],
    ['/available-channels', "'available channels'"],
    ['/custom/42', "'custom'"],
    ['/admin/orders/9f8e-77', "'admin · orders'"],
    ['/', 'none']
  ])('shows the route %s as the small-caps path %s', (path, expected) => {
    route.path = path
    const wrapper = mountLayout()

    expect((wrapper.element as HTMLElement).style.getPropertyValue('--jack-crumb')).toBe(expected)
  })

  it('keeps the upstream onboarding wiring', () => {
    const wrapper = mountLayout()

    expect(tour.options).toEqual([{ storageKey: 'admin_guide', autoStart: true }])
    expect(onboardingStore.setReplayCallback).toHaveBeenCalledWith(tour.replayTour)
    expect((wrapper.vm as unknown as { replayTour: unknown }).replayTour).toBe(tour.replayTour)
  })

  it('uses the user tour for non-administrators', () => {
    authStore.user = { role: 'user' }
    mountLayout()

    expect(tour.options).toEqual([{ storageKey: 'user_guide', autoStart: true }])
  })
})
