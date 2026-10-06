import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const source = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue'), 'utf8')

describe('AppSidebar chat unread badge', () => {
  it('marks the chat entry when replies are unread', () => {
    expect(source).toContain("import { useChatUnread } from '@/composables/useChatUnread'")
    expect(source).toContain("{ path: '/chat', label: t('chat.nav'), icon: ChatBubbleIcon, hideInSimpleMode: true, badge: () => chatUnread.value > 0 }")
  })

  it('renders the badge in both the user menu and the admin personal menu', () => {
    expect(source.match(/v-if="item\.badge\?\.\(\)"/g)?.length).toBe(2)
  })
})
