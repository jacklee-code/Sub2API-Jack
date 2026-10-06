<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="p-3">
      <button type="button" class="btn btn-primary w-full justify-center" data-testid="chat-new" @click="emit('create')">
        <Icon name="plus" size="sm" />
        <span>{{ t('chat.newChat') }}</span>
      </button>
    </div>
    <div class="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
      <p v-if="!conversations.length" class="px-3 py-6 text-center text-sm text-gray-400">{{ t('chat.noConversations') }}</p>
      <ul class="space-y-0.5">
        <li v-for="conv in conversations" :key="conv.id">
          <div
            v-if="editingId === conv.id"
            class="flex items-center gap-1 rounded-lg bg-gray-100 px-2 py-1.5 dark:bg-dark-700"
          >
            <input
              ref="renameInput"
              v-model="draft"
              class="input h-8 flex-1 !py-1 text-sm"
              maxlength="200"
              :aria-label="t('chat.rename')"
              @keydown.enter.prevent="commit(conv)"
              @keydown.esc.prevent="cancel"
              @blur="commit(conv)"
            />
          </div>
          <div
            v-else
            role="button"
            tabindex="0"
            :class="[
              'group flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm',
              conv.id === activeId
                ? 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-white'
                : 'text-gray-600 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-800'
            ]"
            @click="emit('select', conv)"
            @keydown.enter="emit('select', conv)"
          >
            <svg v-if="conv.mode === 'image'" class="h-4 w-4 shrink-0 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" d="m2.25 15.75 5.159-5.159a2.25 2.25 0 0 1 3.182 0l5.159 5.159m-1.5-1.5 1.409-1.409a2.25 2.25 0 0 1 3.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 0 0 1.5-1.5V6a1.5 1.5 0 0 0-1.5-1.5H3.75A1.5 1.5 0 0 0 2.25 6v12a1.5 1.5 0 0 0 1.5 1.5Zm10.5-11.25h.008v.008h-.008V8.25Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Z" />
            </svg>
            <Icon v-else name="chatBubble" size="sm" class="shrink-0 text-gray-400" />
            <span class="min-w-0 flex-1 truncate">{{ conv.title || t('chat.untitled') }}</span>
            <span class="hidden shrink-0 items-center gap-0.5 group-hover:flex group-focus-within:flex" :class="{ '!flex': conv.id === activeId }">
              <button
                type="button"
                class="rounded p-1 text-gray-400 hover:bg-gray-200 hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-100"
                :title="t('chat.rename')"
                :aria-label="t('chat.rename')"
                @click.stop="start(conv)"
              >
                <Icon name="edit" size="xs" />
              </button>
              <button
                type="button"
                class="rounded p-1 text-gray-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/30 dark:hover:text-red-400"
                :title="t('chat.delete')"
                :aria-label="t('chat.delete')"
                @click.stop="emit('delete', conv)"
              >
                <Icon name="trash" size="xs" />
              </button>
            </span>
          </div>
        </li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ChatConversation } from '@/api/chat'

defineProps<{ conversations: ChatConversation[]; activeId: number | null }>()
const emit = defineEmits<{
  select: [conv: ChatConversation]
  create: []
  rename: [conv: ChatConversation, title: string]
  delete: [conv: ChatConversation]
}>()
const { t } = useI18n()

const editingId = ref<number | null>(null)
const draft = ref('')
const renameInput = ref<HTMLInputElement[] | null>(null)

async function start(conv: ChatConversation) {
  editingId.value = conv.id
  draft.value = conv.title
  await nextTick()
  const el = renameInput.value?.[0]
  el?.focus()
  el?.select()
}

function commit(conv: ChatConversation) {
  if (editingId.value !== conv.id) return
  editingId.value = null
  const title = draft.value.trim()
  if (title !== conv.title) emit('rename', conv, title)
}

function cancel() {
  editingId.value = null
}
</script>
