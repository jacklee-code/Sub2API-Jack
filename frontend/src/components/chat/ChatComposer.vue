<template>
  <div
    class="card relative !rounded-2xl transition-colors focus-within:border-primary-400 dark:focus-within:border-primary-500"
    :class="{ 'border-primary-400 ring-2 ring-primary-200 dark:ring-primary-900': dragging }"
    @dragenter.prevent="onDragEnter"
    @dragover.prevent
    @dragleave.prevent="onDragLeave"
    @drop.prevent="onDrop"
  >
    <div v-if="dragging" class="pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-2xl bg-primary-50/80 text-sm font-medium text-primary-700 dark:bg-dark-900/80 dark:text-primary-300">
      {{ t('chat.dropHint') }}
    </div>

    <div v-if="attachments.length" class="flex flex-wrap gap-2 px-3 pt-3">
      <div
        v-for="item in attachments"
        :key="item.key"
        class="group relative flex max-w-[14rem] items-center gap-2 rounded-xl border border-gray-200 bg-gray-50 py-1.5 pl-1.5 pr-7 text-xs dark:border-dark-600 dark:bg-dark-700"
        :class="{ 'border-red-300 dark:border-red-700': item.error }"
      >
        <img v-if="item.previewUrl" :src="item.previewUrl" alt="" class="h-9 w-9 shrink-0 rounded-lg object-cover" />
        <span v-else class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-gray-200 text-gray-500 dark:bg-dark-600 dark:text-gray-300">
          <Icon name="document" size="sm" />
        </span>
        <span class="min-w-0">
          <span class="block truncate text-gray-800 dark:text-gray-100">{{ item.name }}</span>
          <span v-if="item.error" class="block truncate text-red-600 dark:text-red-400">{{ item.error }}</span>
          <span v-else-if="!item.attachment" class="block text-gray-400">{{ t('chat.uploading') }} {{ Math.round(item.progress * 100) }}%</span>
        </span>
        <button
          type="button"
          class="absolute right-1 top-1 rounded-md p-0.5 text-gray-400 hover:bg-gray-200 hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-100"
          :aria-label="t('chat.delete')"
          @click="emit('remove', item.key)"
        >
          <Icon name="x" size="xs" />
        </button>
      </div>
    </div>

    <textarea
      ref="textareaRef"
      :value="modelValue"
      rows="1"
      class="block max-h-60 w-full resize-none border-0 bg-transparent px-4 py-3 text-[15px] leading-6 text-gray-900 placeholder-gray-400 focus:outline-none focus:ring-0 dark:text-gray-100 dark:placeholder-gray-500"
      :placeholder="placeholder"
      :disabled="disabled"
      @input="onInput"
      @keydown="onKeydown"
      @paste="onPaste"
    ></textarea>

    <div class="flex items-center gap-1 px-2 pb-2">
      <button
        v-if="canAttach"
        type="button"
        class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-800 disabled:opacity-40 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-100"
        :title="mode === 'image' ? t('chat.attachReference') : t('chat.attach')"
        :aria-label="mode === 'image' ? t('chat.attachReference') : t('chat.attach')"
        :disabled="disabled"
        @click="fileInput?.click()"
      >
        <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" d="m18.375 12.739-7.693 7.693a4.5 4.5 0 0 1-6.364-6.364l10.94-10.94A3 3 0 1 1 19.5 7.372L8.552 18.32m.009-.01-.01.01m5.699-9.941-7.81 7.81a1.5 1.5 0 0 0 2.112 2.13" />
        </svg>
      </button>
      <input ref="fileInput" type="file" class="hidden" multiple :accept="accept" @change="onFileChange" />

      <button
        v-if="mode === 'chat'"
        type="button"
        class="flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-sm font-medium transition-colors"
        :class="webSearch
          ? 'border-emerald-600 bg-emerald-600 text-white shadow-sm hover:bg-emerald-700 dark:border-emerald-500 dark:bg-emerald-600 dark:hover:bg-emerald-500'
          : 'border-gray-200 text-gray-500 hover:border-gray-300 hover:bg-gray-100 hover:text-gray-800 dark:border-dark-600 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-100'"
        :title="t('chat.webSearchHint')"
        :aria-pressed="webSearch"
        data-testid="chat-web-search"
        @click="emit('update:webSearch', !webSearch)"
      >
        <Icon :name="webSearch ? 'check' : 'globe'" size="sm" />
        <span class="hidden sm:inline">{{ t('chat.webSearch') }}</span>
        <span v-if="webSearch" class="hidden rounded-full bg-white/20 px-1.5 text-xs sm:inline">{{ t('chat.webSearchOn') }}</span>
      </button>

      <div class="ml-auto flex min-w-0 items-center gap-2">
        <slot name="controls" />
        <button
          v-if="busy"
          type="button"
          class="flex h-9 w-9 items-center justify-center rounded-full bg-gray-900 text-white hover:bg-gray-700 dark:bg-gray-100 dark:text-gray-900 dark:hover:bg-gray-300"
          :title="t('chat.stop')"
          :aria-label="t('chat.stop')"
          @click="emit('stop')"
        >
          <span class="h-3 w-3 rounded-sm bg-current"></span>
        </button>
        <button
          v-else
          type="button"
          class="flex h-9 w-9 items-center justify-center rounded-full bg-primary-600 text-white hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-40"
          :title="t('chat.send')"
          :aria-label="t('chat.send')"
          :disabled="!canSend"
          @click="emit('send')"
        >
          <Icon name="arrowUp" size="sm" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ChatMode } from '@/api/chat'
import type { DraftAttachment } from './types'

const props = defineProps<{
  modelValue: string
  mode: ChatMode
  attachments: DraftAttachment[]
  placeholder: string
  busy: boolean
  disabled?: boolean
  canAttach: boolean
  canSend: boolean
  webSearch: boolean
  accept: string
}>()
const emit = defineEmits<{
  'update:modelValue': [value: string]
  'update:webSearch': [value: boolean]
  send: []
  stop: []
  files: [files: File[]]
  remove: [key: string]
}>()
const { t } = useI18n()

const textareaRef = ref<HTMLTextAreaElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const dragging = ref(false)
let dragDepth = 0

function resize() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, 240)}px`
}

watch(() => props.modelValue, () => nextTick(resize))

function onInput(e: Event) {
  emit('update:modelValue', (e.target as HTMLTextAreaElement).value)
}

function onKeydown(e: KeyboardEvent) {
  // Keep IME composition (Chinese/Japanese input) from sending early.
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing && e.keyCode !== 229) {
    e.preventDefault()
    if (!props.busy && props.canSend) emit('send')
  }
}

function onPaste(e: ClipboardEvent) {
  const files = Array.from(e.clipboardData?.files || [])
  if (files.length && props.canAttach) {
    e.preventDefault()
    emit('files', files)
  }
}

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (files.length) emit('files', files)
}

function onDragEnter() {
  if (!props.canAttach) return
  dragDepth++
  dragging.value = true
}

function onDragLeave() {
  dragDepth = Math.max(0, dragDepth - 1)
  if (dragDepth === 0) dragging.value = false
}

function onDrop(e: DragEvent) {
  dragDepth = 0
  dragging.value = false
  const files = Array.from(e.dataTransfer?.files || [])
  if (files.length && props.canAttach) emit('files', files)
}

function focus() {
  textareaRef.value?.focus()
}

defineExpose({ focus })
</script>
