import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('@/utils/ipGeoLookup', () => ({ getEntry: vi.fn(() => ({ status: 'idle' })), fetchOne: vi.fn(), fetchBatch: vi.fn() }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => (key === 'chat.usageGroupSuffix' ? '聊天模式' : key) }),
}))

import UsageTable from '../UsageTable.vue'

const DataTableStub = {
  props: ['data'],
  template: '<div><div v-for="row in data" :key="row.request_id" :data-row="row.request_id"><slot name="cell-group" :row="row" /></div></div>',
}

describe('UsageTable chat mode group', () => {
  it('labels chat-mode usage with the group name and the chat suffix', () => {
    const wrapper = mount(UsageTable, {
      props: {
        data: [
          { request_id: 'chat', chat_mode: true, group: { id: 1, name: 'Pro' } },
          { request_id: 'api', group: { id: 1, name: 'Pro' } },
        ],
        loading: false,
        columns: [],
      },
      global: { stubs: { DataTable: DataTableStub, EmptyState: true, Icon: true, Teleport: true } },
    })
    expect(wrapper.get('[data-row="chat"]').text()).toBe('Pro 聊天模式')
    expect(wrapper.get('[data-row="api"]').text()).toBe('Pro')
  })
})
