import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it } from 'vitest'
import AccountStatusIndicator from '@/components/account/AccountStatusIndicator.vue'
import type { Account } from '@/types'

const exhausted = {
  id: 3, platform: 'openai', type: 'apikey', status: 'error', schedulable: false,
  error_message: '[jack_openai_credit_exhausted] OpenAI API credit balance exhausted; use Restore status after replenishing credits.',
  // A pre-existing short cooldown must not hide the persistent label.
  rate_limit_reset_at: '2099-01-01T00:00:00Z',
} as Account

describe('official API credit hold', () => {
  it.each([
    ['zh-Hant', '額度耗盡', '需手動恢復狀態'],
    ['zh', '额度耗尽', '需手动恢复状态'],
    ['en', 'Credit exhausted', 'Restore status manually'],
  ])('shows the durable hold in %s and clears it after recovery', async (locale, label, hint) => {
    const wrapper = mount(AccountStatusIndicator, {
      props: { account: { ...exhausted } },
      global: { plugins: [createI18n({ legacy: false, locale, messages: { [locale]: { admin: { accounts: { status: { active: () => 'Normal', error: () => 'Error' } } } } } })] },
    })
    expect(wrapper.text()).toContain(label)
    expect(wrapper.text()).toContain(hint)
    expect(wrapper.text()).not.toContain('429')
    expect(wrapper.text()).not.toContain('[jack_')
    await wrapper.setProps({ account: { ...exhausted, status: 'active', error_message: '', schedulable: true, rate_limit_reset_at: null } })
    expect(wrapper.text()).not.toContain(label)
    expect(wrapper.text()).toContain('Normal')
  })
})
