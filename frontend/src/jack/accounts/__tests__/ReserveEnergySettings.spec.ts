import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ReserveEnergySettings from '../ReserveEnergySettings.vue'
import { list } from '@/api/admin/accounts'

vi.mock('@/api/admin/accounts', () => ({ list: vi.fn() }))
const items = [
  { id: 1, name: 'Primary', platform: 'openai', type: 'oauth' },
  { id: 2, name: 'Reserve A', platform: 'openai', type: 'apikey' },
  { id: 3, name: 'Reserve B', platform: 'openai', type: 'apikey' },
  { id: 4, name: 'Shadow', platform: 'openai', type: 'oauth', parent_account_id: 1 }
]
function render(extra: Record<string, unknown> = {}, locale = 'zh-Hant') {
  return mount(ReserveEnergySettings, {
    props: { accountId: 1, extra },
    global: { plugins: [createI18n({ legacy: false, locale, fallbackLocale: 'en' })] }
  })
}
beforeEach(() => { vi.mocked(list).mockReset(); vi.mocked(list).mockResolvedValue({ items, total: 4 } as Awaited<ReturnType<typeof list>>) })

describe('reserve energy account settings', () => {
  it('loads on enable and preserves ordered additions without selecting the primary or shadows', async () => {
    const wrapper = render()
    expect(list).not.toHaveBeenCalled()
    await wrapper.get('input[type=checkbox]').setValue(true)
    await flushPromises()
    expect(wrapper.text()).toContain('Credit')
    const select = wrapper.get('select')
    expect(select.findAll('option').map(o => o.attributes('value'))).toEqual(['', '2', '3'])
    await select.setValue('3')
    await select.setValue('2')
    expect(wrapper.emitted('change')?.at(-1)).toEqual([{ jack_reserve_enabled: true, jack_reserve_account_ids: [3, 2] }])
    expect(select.findAll('option')).toHaveLength(1)
  })

  it('saves reordered and removed entries and keeps the plan when switched off', async () => {
    const wrapper = render({ jack_reserve_enabled: true, jack_reserve_account_ids: [3, 2] })
    await flushPromises()
    await wrapper.findAll('button[aria-label="上移"]')[1]!.trigger('click')
    expect(wrapper.emitted('change')?.at(-1)).toEqual([{ jack_reserve_enabled: true, jack_reserve_account_ids: [2, 3] }])
    await wrapper.findAll('li button').at(-1)!.trigger('click')
    expect(wrapper.emitted('change')?.at(-1)).toEqual([{ jack_reserve_enabled: true, jack_reserve_account_ids: [2] }])
    await wrapper.get('input[type=checkbox]').setValue(false)
    expect(wrapper.emitted('change')?.at(-1)).toEqual([{ jack_reserve_enabled: false, jack_reserve_account_ids: [2] }])
  })

  it('shows missing saved IDs so they can be removed and fetches all pages', async () => {
    vi.mocked(list).mockResolvedValueOnce({ items: items.slice(0, 2), total: 4 } as Awaited<ReturnType<typeof list>>)
      .mockResolvedValueOnce({ items: items.slice(2), total: 4 } as Awaited<ReturnType<typeof list>>)
    const wrapper = render({ jack_reserve_enabled: true, jack_reserve_account_ids: [99] })
    await flushPromises()
    expect(list).toHaveBeenCalledTimes(2)
    expect(wrapper.get('li').text()).toContain('#99')
    expect(wrapper.get('select').findAll('option')).toHaveLength(3)
  })

  it('shows a recoverable loading error without dropping the saved plan', async () => {
    vi.mocked(list).mockRejectedValueOnce(new Error('offline'))
    const wrapper = render({ jack_reserve_enabled: true, jack_reserve_account_ids: [2] }, 'en')
    await flushPromises()
    expect(wrapper.get('[role=alert]').text()).toContain('Could not load accounts')
    expect(wrapper.get('li').text()).toContain('#2')
    await wrapper.get('[role=alert] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role=alert]').exists()).toBe(false)
    expect(wrapper.get('li').text()).toContain('Reserve A')
    expect(wrapper.emitted('change')).toBeUndefined()
  })
})
