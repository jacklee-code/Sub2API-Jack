import { onBeforeUnmount, onMounted, ref } from 'vue'
import { getUnreadCount } from '@/api/chat'

/**
 * Jack chat mode: number of conversations with replies the user has not seen.
 * Shared by the sidebar badge and the chat page; polled while any component
 * that uses it is mounted.
 */
const count = ref(0)
let users = 0
let timer: number | null = null
const POLL_MS = 60_000

export async function refreshChatUnread(): Promise<void> {
  if (!localStorage.getItem('auth_token')) return
  try {
    count.value = await getUnreadCount()
  } catch {
    // keep the last value; chat may be disabled or the session expired
  }
}

export function useChatUnread() {
  onMounted(() => {
    users++
    if (users === 1) {
      void refreshChatUnread()
      timer = window.setInterval(() => {
        if (document.visibilityState === 'visible') void refreshChatUnread()
      }, POLL_MS)
    }
  })
  onBeforeUnmount(() => {
    users = Math.max(0, users - 1)
    if (users === 0 && timer !== null) {
      window.clearInterval(timer)
      timer = null
    }
  })
  return { unreadCount: count, refresh: refreshChatUnread }
}
