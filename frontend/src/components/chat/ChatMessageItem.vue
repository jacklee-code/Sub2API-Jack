<template>
  <div :class="['flex w-full', isUser ? 'justify-end' : 'justify-start']">
    <div :class="['min-w-0', isUser ? 'max-w-[85%] sm:max-w-[75%]' : 'w-full max-w-3xl']">
      <!-- User turn -->
      <template v-if="isUser">
        <div v-if="message.attachments.length" class="mb-2 flex flex-wrap justify-end gap-2">
          <template v-for="att in message.attachments" :key="att.id">
            <ChatImage v-if="isImage(att)" :attachment="att" :max-edge="140" @open="emit('open', $event)" />
            <button
              v-else
              type="button"
              class="flex max-w-[16rem] items-center gap-2 rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-left text-sm dark:border-dark-600 dark:bg-dark-700"
              @click="download(att)"
            >
              <Icon name="document" size="sm" class="shrink-0 text-gray-400" />
              <span class="min-w-0">
                <span class="block truncate text-gray-800 dark:text-gray-100">{{ att.filename }}</span>
                <span class="block text-xs text-gray-400">{{ formatSize(att.size) }}</span>
              </span>
            </button>
          </template>
        </div>
        <div
          v-if="message.content"
          class="whitespace-pre-wrap break-words rounded-2xl rounded-br-md bg-primary-50 px-4 py-2.5 text-[15px] leading-7 text-gray-900 dark:bg-primary-900/30 dark:text-gray-100"
        >{{ message.content }}</div>
      </template>

      <!-- Assistant turn -->
      <template v-else>
        <div v-if="message.searching || (message.searchQueries && message.searchQueries.length)" class="mb-2 space-y-1 text-xs text-gray-500 dark:text-gray-400">
          <div v-for="(q, i) in message.searchQueries || []" :key="i" class="flex items-center gap-1.5">
            <Icon name="globe" size="xs" />
            <span class="truncate">{{ q ? t('chat.searched', { query: q }) : t('chat.searchedPlain') }}</span>
          </div>
          <div v-if="message.searching" class="flex items-center gap-1.5">
            <Icon name="globe" size="xs" class="animate-pulse" />
            <span>{{ t('chat.searching') }}</span>
          </div>
        </div>

        <details v-if="message.reasoning" class="mb-2 rounded-xl border border-gray-200 px-3 py-2 text-sm dark:border-dark-600" :open="streaming && !message.content">
          <summary class="cursor-pointer select-none text-gray-500 dark:text-gray-400">{{ t('chat.reasoning') }}</summary>
          <div class="mt-2 whitespace-pre-wrap text-gray-600 dark:text-gray-400">{{ message.reasoning }}</div>
        </details>

        <div
          v-if="message.content || (streaming && !hasImages && !message.reasoning && !message.searching)"
          :class="['chat-markdown', streaming ? 'chat-cursor' : '']"
          @click="onMarkdownClick"
          v-html="html"
        ></div>

        <div v-if="hasImages" :class="['flex flex-wrap gap-2', message.content ? 'mt-3' : '']">
          <ChatImage
            v-for="att in message.attachments"
            :key="att.id"
            :attachment="att"
            :max-edge="imageEdge"
            :aspect="message.aspect"
            actions
            :can-reference="canReference"
            @open="emit('open', $event)"
            @reference="emit('reference', $event)"
          />
          <div
            v-for="n in pendingCount"
            :key="`pending-${n}`"
            class="chat-shimmer relative flex items-center justify-center overflow-hidden rounded-xl border border-gray-200 bg-gray-100 dark:border-dark-600 dark:bg-dark-800"
            :style="placeholderStyle"
          >
            <svg class="h-7 w-7 text-gray-300 dark:text-dark-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" d="m2.25 15.75 5.159-5.159a2.25 2.25 0 0 1 3.182 0l5.159 5.159m-1.5-1.5 1.409-1.409a2.25 2.25 0 0 1 3.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 0 0 1.5-1.5V6a1.5 1.5 0 0 0-1.5-1.5H3.75A1.5 1.5 0 0 0 2.25 6v12a1.5 1.5 0 0 0 1.5 1.5Zm10.5-11.25h.008v.008h-.008V8.25Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Z" />
            </svg>
          </div>
        </div>
        <p v-if="hasImages || (message.imageErrors && message.imageErrors.length)" class="mt-2 text-xs text-gray-500 dark:text-gray-400">
          <template v-if="streaming && pendingCount > 0">{{ t('chat.generating', { n: pendingCount }) }}</template>
          <template v-else>{{ imageCaption }}</template>
        </p>
        <ul v-if="message.imageErrors && message.imageErrors.length" class="mt-2 space-y-0.5 text-xs text-red-600 dark:text-red-400">
          <li v-for="e in message.imageErrors" :key="e.index">{{ t('chat.imageFailed', { n: e.index + 1 }) }}: {{ e.error }}</li>
        </ul>

        <p v-if="message.status === 'error'" class="mt-2 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300" role="alert">
          {{ t('chat.failed') }}<template v-if="message.error">: {{ message.error }}</template>
        </p>
        <p v-else-if="message.error && !streaming && !(message.imageErrors && message.imageErrors.length)" class="mt-2 text-xs text-amber-600 dark:text-amber-400">
          {{ t('chat.partialFailed') }}: {{ message.error }}
        </p>

        <div v-if="message.citations && message.citations.length" class="mt-3">
          <div class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('chat.sources') }}</div>
          <ol class="space-y-0.5 text-sm">
            <li v-for="(c, i) in message.citations" :key="c.url" class="flex gap-1.5">
              <span class="text-gray-400">{{ i + 1 }}.</span>
              <a :href="safeUrl(c.url)" target="_blank" rel="noopener noreferrer" class="min-w-0 truncate text-primary-600 hover:underline dark:text-primary-400">{{ c.title || hostOf(c.url) }}</a>
            </li>
          </ol>
        </div>

        <div v-if="!streaming" class="mt-2 flex flex-wrap items-center gap-1 text-xs text-gray-400">
          <span v-if="message.status === 'aborted'" class="badge badge-warning mr-1">{{ t('chat.aborted') }}</span>
          <span v-if="message.model && !hasImages" class="mr-2">{{ message.model }}<template v-if="message.input_tokens || message.output_tokens"> · {{ t('chat.tokens', { input: message.input_tokens, output: message.output_tokens }) }}</template></span>
          <button v-if="message.content" type="button" class="rounded-md px-2 py-1 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200" @click="copy">
            {{ copied ? t('chat.copied') : t('chat.copy') }}
          </button>
          <button v-if="canRegenerate" type="button" class="rounded-md px-2 py-1 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200" @click="emit('regenerate')">
            {{ t('chat.regenerate') }}
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ChatAttachment } from '@/api/chat'
import ChatImage from './ChatImage.vue'
import { downloadAttachment } from './attachmentUrls'
import { fitBox } from './imageSize'
import { handleCodeCopyClick, renderMarkdown } from './markdown'
import type { UiMessage } from './types'

const props = defineProps<{ message: UiMessage; canRegenerate?: boolean; canReference?: boolean }>()
const emit = defineEmits<{ regenerate: []; open: [attachment: ChatAttachment]; reference: [attachment: ChatAttachment] }>()
const { t } = useI18n()

const isUser = computed(() => props.message.role === 'user')
const streaming = computed(() => props.message.status === 'streaming')
const html = computed(() => renderMarkdown(props.message.content, t('chat.copy')))
const pendingCount = computed(() => Math.max(0, props.message.pendingImages || 0))
const imageTotal = computed(() => props.message.attachments.length + pendingCount.value + (props.message.imageErrors?.length || 0))
const hasImages = computed(() => props.message.attachments.length > 0 || pendingCount.value > 0)
const imageEdge = computed(() => (imageTotal.value > 1 ? 200 : 320))
const imageCaption = computed(() => {
  const parts = [props.message.model, props.message.aspect, t('chat.countShort', { n: props.message.attachments.length })]
  return parts.filter(Boolean).join(' · ')
})
const placeholderStyle = computed(() => {
  const box = fitBox(undefined, undefined, imageEdge.value, props.message.aspect)
  return { width: `${box.width}px`, aspectRatio: `${box.width} / ${box.height}`, maxWidth: '100%' }
})

const copied = ref(false)
async function copy() {
  try {
    await navigator.clipboard.writeText(props.message.content)
    copied.value = true
    window.setTimeout(() => (copied.value = false), 1500)
  } catch {
    // clipboard unavailable
  }
}

function onMarkdownClick(e: MouseEvent) {
  void handleCodeCopyClick(e, t('chat.copied'), t('chat.copy'))
}

function isImage(att: ChatAttachment) {
  return att.kind === 'image' || att.kind === 'generated'
}

function download(att: ChatAttachment) {
  void downloadAttachment(att.id, att.filename)
}

function formatSize(bytes: number) {
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)} MB`
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`
  return `${bytes} B`
}

function safeUrl(url: string) {
  return /^https?:\/\//i.test(url) ? url : '#'
}

function hostOf(url: string) {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}
</script>
