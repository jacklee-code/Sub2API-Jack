import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FleetsView from '../FleetsView.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), preview: vi.fn(), mutate: vi.fn(), groups: vi.fn(), users: vi.fn(), success: vi.fn(), info: vi.fn() }))
vi.mock('@/api/admin/fleets', () => ({ list: mocks.list, preview: mocks.preview, mutate: mocks.mutate }))
vi.mock('@/api/admin/groups', () => ({ getAll: mocks.groups }))
vi.mock('@/api/admin/users', () => ({ list: mocks.users }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showSuccess: mocks.success, showInfo: mocks.info }) }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))

function fleet(id: number) {
  return { id, name: `Fleet ${id}`, group_id: id * 10, group_name: `Group ${id}`, platform: 'openai', expires_at: '2026-11-02T15:59:59Z', version: 3, daily_limit_usd: 300, weekly_limit_usd: 300, monthly_limit_usd: 1200,
    members: id === 1 ? [{ user_id: 7, email: 'member@example.test', username: '', role: 'user', subscription_id: 21, independent_expiry: false, active: true, expires_at: '2026-11-02T15:59:59Z', status: 'active', daily_usage_usd: 5, weekly_usage_usd: 7, monthly_usage_usd: 9, key_count: 2 }] : [] }
}
function render() {
  return mount(FleetsView, { global: { stubs: {
    AppLayout: { template: '<div><slot /></div>' }, Icon: true,
    RouterLink: { template: '<a><slot /></a>' },
    BaseDialog: { props: ['show'], template: '<div v-if="show" data-dialog><slot /><slot name="footer" /></div>' }
  } } })
}
beforeEach(() => {
  vi.resetAllMocks()
  mocks.list.mockResolvedValue([fleet(1), fleet(2)])
  mocks.preview.mockResolvedValue([])
  mocks.groups.mockResolvedValue([{ id: 30, name: 'Available group', subscription_type: 'subscription', status: 'active' }])
  mocks.mutate.mockResolvedValue({ fleet_id: 1, cache_pending: false })
})

describe('Fleet management', () => {
  it('moves a member through the accessible button and submits both fleet versions', async () => {
    const wrapper = render(); await flushPromises()
    await wrapper.get('button[aria-label="fleet.transfer: member@example.test"]').trigger('click')
    await wrapper.get('[data-dialog] select').setValue(2); await flushPromises()
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(mocks.mutate).toHaveBeenCalledWith(expect.objectContaining({ action: 'transfer', fleet_id: 1, target_fleet_id: 2, user_id: 7, versions: { 1: 3, 2: 3 } }), expect.any(String))
    wrapper.unmount()
  })

  it('reuses the operation key after a lost response without extending twice', async () => {
    mocks.mutate.mockRejectedValueOnce(new Error('Connection lost'))
    const wrapper = render(); await flushPromises()
    const section = wrapper.get('[data-fleet-id="1"]')
    await section.findAll('button').find(b => b.text() === 'fleet.adjust')!.trigger('click')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.text()).toContain('Connection lost')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(mocks.mutate.mock.calls).toHaveLength(2)
    expect(mocks.mutate.mock.calls[0][1]).toBe(mocks.mutate.mock.calls[1][1])
    expect(mocks.mutate.mock.calls[1][0]).toMatchObject({ action: 'expiry', days: 30 })
    wrapper.unmount()
  })

  it('previews adoption and explicitly preserves an independent expiry', async () => {
    mocks.list.mockResolvedValue([])
    mocks.preview.mockResolvedValue([{ ...fleet(1).members[0], role: 'admin', user_id: 1, subscription_id: 99, email: 'admin@example.test', expires_at: '2026-11-13T00:00:00Z' }])
    const wrapper = render(); await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'fleet.create')!.trigger('click')
    await wrapper.get('input[maxlength="100"]').setValue('New fleet')
    await wrapper.get('select').setValue(30); await flushPromises()
    await wrapper.get('input[type="datetime-local"]').setValue('2026-11-02T23:59:59')
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.findAll('input[type="checkbox"]')[1].setValue(true)
    expect(wrapper.text()).toContain('2026-11-13')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(mocks.mutate).toHaveBeenCalledWith(expect.objectContaining({ action: 'create', expires_at: '2026-11-02T15:59:59.000Z', members: [{ user_id: 1, independent_expiry: true, adopt_existing: true }] }), expect.any(String))
    wrapper.unmount()
  })

  it('resets all fleets with an explicit selected window and reports pending cache synchronization', async () => {
    mocks.mutate.mockResolvedValue({ fleet_id: 0, cache_pending: true })
    const wrapper = render(); await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'fleet.resetAll')!.trigger('click')
    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[0].setValue(false); await checkboxes[2].setValue(false)
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(mocks.mutate).toHaveBeenCalledWith(expect.objectContaining({ action: 'reset_all', daily: false, weekly: true, monthly: false, versions: { 1: 3, 2: 3 } }), expect.any(String))
    expect(mocks.info).toHaveBeenCalledWith('fleet.pending')
    wrapper.unmount()
  })
})
