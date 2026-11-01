import { beforeEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { defineComponent, nextTick, ref } from 'vue'
import type { AdminUsageLog } from '@/types'
import MappingVisibilityToggle from '../MappingVisibilityToggle.vue'
import { useMappingVisibility } from '../useMappingVisibility'

const record = {
  id: 1, model: 'public-model', reasoning_effort: 'xhigh',
  upstream_model: 'private-model', upstream_reasoning_effort: 'low',
  upstream_response_model: 'private-snapshot', upstream_model_mismatch: true,
  model_mapping_chain: 'public-model→private-model',
} as AdminUsageLog

const Harness = defineComponent({
  components: { MappingVisibilityToggle },
  setup() {
    const rows = ref([record])
    return { rows, ...useMappingVisibility(rows) }
  },
  template: '<div><MappingVisibilityToggle v-model="showMappings" /><pre data-testid="rows">{{ JSON.stringify(displayRecords) }}</pre></div>',
})
const mountHarness = () => mount(Harness, {
  global: { plugins: [createI18n({ legacy: false, locale: 'zh-Hant', messages: {} })] },
})

describe('admin usage mapping visibility', () => {
  beforeEach(() => localStorage.clear())

  it('defaults to requested values, restores every mapping on selection, and retains the original admin record', async () => {
    const wrapper = mountHarness()
    const toggle = wrapper.get('[data-testid="usage-show-mappings"]')
    expect(toggle.text()).toBe('顯示映射')
    expect(toggle.attributes('aria-checked')).toBe('false')
    expect(wrapper.get('[data-testid="rows"]').text()).toContain('public-model')
    expect(wrapper.get('[data-testid="rows"]').text()).toContain('xhigh')
    expect(wrapper.get('[data-testid="rows"]').text()).not.toContain('private-')
    expect((wrapper.vm as unknown as { rows: AdminUsageLog[] }).rows[0]).toEqual(record)
    await toggle.trigger('click')
    expect(toggle.attributes('aria-checked')).toBe('true')
    expect(wrapper.get('[data-testid="rows"]').text()).toContain('private-model')
    expect(wrapper.get('[data-testid="rows"]').text()).toContain('private-snapshot')
    expect(wrapper.get('[data-testid="rows"]').text()).toContain('low')
    await toggle.trigger('click')
    expect(wrapper.get('[data-testid="rows"]').text()).not.toContain('private-')
    wrapper.unmount()
  })

  it('remembers an explicit selection across mounts without changing the upstream column preferences', async () => {
    localStorage.setItem('usage-hidden-columns', '["reasoning_effort"]')
    const first = mountHarness()
    await first.get('[data-testid="usage-show-mappings"]').trigger('click')
    await nextTick()
    first.unmount()
    const second = mountHarness()
    expect(second.get('[data-testid="usage-show-mappings"]').attributes('aria-checked')).toBe('true')
    expect(second.get('[data-testid="rows"]').text()).toContain('private-model')
    expect(localStorage.getItem('usage-hidden-columns')).toBe('["reasoning_effort"]')
    second.unmount()
  })
})
