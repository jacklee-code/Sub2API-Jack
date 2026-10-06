<template>
  <AppLayout>
    <div class="chat-page flex overflow-hidden rounded-2xl border border-gray-200 dark:border-dark-700" data-testid="chat-page">
      <!-- Conversation list -->
      <aside
        :class="[
          'w-full shrink-0 border-gray-200 dark:border-dark-700 md:block md:w-72 md:border-r',
          listOpen ? 'block' : 'hidden'
        ]"
      >
        <ChatConversationList
          :conversations="conversations"
          :active-id="activeId"
          @select="selectConversation"
          @create="startNew()"
          @rename="renameConversation"
          @delete="deleteTarget = $event"
        />
      </aside>

      <!-- Chat area -->
      <section :class="['min-w-0 flex-1 flex-col', listOpen ? 'hidden md:flex' : 'flex']">
        <!-- Toolbar -->
        <div class="flex flex-wrap items-center gap-2 border-b border-gray-200 px-3 py-2 dark:border-dark-700">
          <button
            type="button"
            class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-700 md:hidden"
            :aria-label="t('chat.showList')"
            @click="listOpen = true"
          >
            <Icon name="menu" size="sm" />
          </button>

          <div class="inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700" role="tablist">
            <button
              v-for="m in modes"
              :key="m.value"
              type="button"
              role="tab"
              :aria-selected="settings.mode === m.value"
              :class="[
                'rounded-md px-3 py-1 text-sm font-medium transition-colors',
                settings.mode === m.value
                  ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-500 dark:text-white'
                  : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'
              ]"
              :data-testid="`chat-mode-${m.value}`"
              @click="changeMode(m.value)"
            >
              {{ m.label }}
            </button>
          </div>

          <label class="sr-only" for="chat-group">{{ t('chat.group') }}</label>
          <select id="chat-group" v-model.number="settings.group_id" class="input h-9 w-auto max-w-[11rem] !py-1 text-sm" :disabled="busy || !groups.length" @change="changeGroup">
            <option v-for="g in groups" :key="g.id" :value="g.id">{{ g.name }}</option>
          </select>

          <label class="sr-only" for="chat-model">{{ t('chat.model') }}</label>
          <select id="chat-model" v-model="settings.model" class="input h-9 w-auto max-w-[14rem] !py-1 text-sm" :disabled="busy || loadingModels || !models.length" data-testid="chat-model" @change="changeSetting({ model: settings.model })">
            <option v-if="loadingModels" value="">{{ t('chat.loadingModels') }}</option>
            <option v-else-if="!models.length" value="">{{ t('chat.noModels') }}</option>
            <option v-for="m in models" :key="m" :value="m">{{ m }}</option>
          </select>

          <template v-if="settings.mode === 'chat'">
            <label class="sr-only" for="chat-effort">{{ t('chat.effort') }}</label>
            <select id="chat-effort" v-model="settings.reasoning_effort" class="input h-9 w-auto !py-1 text-sm" :title="t('chat.effort')" :disabled="busy" @change="changeSetting({ reasoning_effort: settings.reasoning_effort })">
              <option v-for="e in efforts" :key="e" :value="e">{{ t('chat.effort') }}: {{ effortLabel(e) }}</option>
            </select>
          </template>
          <template v-else>
            <label class="sr-only" for="chat-aspect">{{ t('chat.aspect') }}</label>
            <select id="chat-aspect" v-model="settings.image_aspect" class="input h-9 w-auto !py-1 text-sm" :title="t('chat.aspect')" :disabled="busy" @change="changeSetting({ image_aspect: settings.image_aspect })">
              <option v-for="a in aspects" :key="a" :value="a">{{ a }}</option>
            </select>
            <label class="sr-only" for="chat-count">{{ t('chat.count') }}</label>
            <select id="chat-count" v-model.number="settings.image_count" class="input h-9 w-auto !py-1 text-sm" :title="t('chat.count')" :disabled="busy" @change="changeSetting({ image_count: settings.image_count })">
              <option v-for="n in maxImageCount" :key="n" :value="n">{{ n }} × {{ t('chat.count') }}</option>
            </select>
            <span class="badge badge-gray">{{ t('chat.resolution') }}</span>
          </template>
        </div>

        <!-- Messages -->
        <div ref="scroller" class="min-h-0 flex-1 overflow-y-auto px-4 py-6" @scroll="onScroll">
          <div v-if="loadingConfig" class="flex h-full items-center justify-center">
            <span class="h-8 w-8 animate-spin rounded-full border-2 border-gray-300 border-t-primary-500"></span>
          </div>
          <div v-else-if="pageError" class="mx-auto max-w-md pt-16 text-center text-sm text-gray-500 dark:text-gray-400" role="alert">{{ pageError }}</div>
          <div v-else-if="!messages.length && !loadingMessages" class="mx-auto flex h-full max-w-xl flex-col items-center justify-center text-center">
            <h2 class="font-display text-2xl text-gray-900 dark:text-white">{{ settings.mode === 'image' ? t('chat.welcomeImage') : t('chat.welcome') }}</h2>
            <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('chat.welcomeHint') }}</p>
          </div>
          <div v-else class="mx-auto max-w-3xl space-y-6">
            <ChatMessageItem
              v-for="(m, i) in messages"
              :key="m.id"
              :message="m"
              :can-regenerate="!busy && i === messages.length - 1 && m.role === 'assistant'"
              @regenerate="regenerate"
              @open="lightbox = $event"
            />
          </div>
        </div>

        <!-- Composer -->
        <div class="px-4 pb-4">
          <div class="mx-auto max-w-3xl">
            <ChatComposer
              ref="composer"
              v-model="draft"
              v-model:web-search="webSearch"
              :mode="settings.mode"
              :attachments="draftAttachments"
              :placeholder="settings.mode === 'image' ? t('chat.imagePlaceholder') : t('chat.placeholder')"
              :busy="busy"
              :disabled="!ready"
              :can-attach="ready && canAttach"
              :can-send="canSend"
              :accept="accept"
              @send="send"
              @stop="stop"
              @files="addFiles"
              @remove="removeDraftAttachment"
              @update:web-search="saveWebSearch"
            />
            <p v-if="ready && !storageAvailable && settings.mode === 'image'" class="mt-2 text-xs text-amber-600 dark:text-amber-400">{{ t('chat.storageMissing') }}</p>
          </div>
        </div>
      </section>
    </div>

    <ConfirmDialog
      :show="!!deleteTarget"
      :title="t('chat.deleteTitle')"
      :message="t('chat.deleteHint')"
      :confirm-text="t('chat.delete')"
      danger
      @confirm="confirmDelete"
      @cancel="deleteTarget = null"
    />
    <ChatLightbox :attachment="lightbox" :can-reference="lightboxCanReference" @close="lightbox = null" @reference="useAsReference" />
  </AppLayout>
</template>

<script setup lang="ts">
import '@/styles/chat.css'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import ChatComposer from '@/components/chat/ChatComposer.vue'
import ChatConversationList from '@/components/chat/ChatConversationList.vue'
import ChatLightbox from '@/components/chat/ChatLightbox.vue'
import ChatMessageItem from '@/components/chat/ChatMessageItem.vue'
import { attachmentUrl, releaseAttachmentUrls } from '@/components/chat/attachmentUrls'
import type { DraftAttachment, UiMessage } from '@/components/chat/types'
import * as chatAPI from '@/api/chat'
import type { ChatAttachment, ChatConfig, ChatConversation, ChatMode, ChatStreamEvent, ConversationPatch } from '@/api/chat'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

const config = ref<ChatConfig | null>(null)
const loadingConfig = ref(true)
const pageError = ref('')
const conversations = ref<ChatConversation[]>([])
const activeId = ref<number | null>(null)
const messages = ref<UiMessage[]>([])
const loadingMessages = ref(false)
const listOpen = ref(false)
const deleteTarget = ref<ChatConversation | null>(null)
const lightbox = ref<ChatAttachment | null>(null)
const draft = ref('')
const draftAttachments = ref<DraftAttachment[]>([])
const webSearch = ref(false)
const busy = ref(false)
const models = ref<string[]>([])
const loadingModels = ref(false)
const scroller = ref<HTMLElement | null>(null)
const composer = ref<InstanceType<typeof ChatComposer> | null>(null)
let abort: AbortController | null = null
let stickToBottom = true
let draftSeq = 0
const modelCache = new Map<string, string[]>()

// Settings of the active conversation, or of the next one when none is open.
const settings = reactive({
  mode: 'chat' as ChatMode,
  group_id: null as number | null,
  model: '',
  reasoning_effort: '',
  image_aspect: '1:1',
  image_count: 1,
})

const modes = computed(() => [
  { value: 'chat' as ChatMode, label: t('chat.modeChat') },
  { value: 'image' as ChatMode, label: t('chat.modeImage') },
])
const groups = computed(() => config.value?.groups || [])
const efforts = computed(() => config.value?.reasoning_efforts || [''])
const aspects = computed(() => config.value?.image_aspects || ['1:1'])
const maxImageCount = computed(() => config.value?.max_image_count || 4)
const storageAvailable = computed(() => !!config.value?.storage_available)
const ready = computed(() => !loadingConfig.value && !pageError.value)
const activeConversation = computed(() => conversations.value.find((c) => c.id === activeId.value) || null)
const uploading = computed(() => draftAttachments.value.some((a) => !a.attachment && !a.error))
const canAttach = computed(() => (settings.mode === 'image' ? storageAvailable.value : true))
const accept = computed(() => {
  if (settings.mode === 'image') return 'image/png,image/jpeg,image/webp,image/gif'
  const text = '.txt,.md,.csv,.tsv,.json,.jsonl,.xml,.yaml,.yml,.toml,.log,.html,.css,.js,.ts,.tsx,.jsx,.vue,.py,.go,.java,.c,.h,.cpp,.cs,.rs,.rb,.php,.sh,.sql'
  return storageAvailable.value ? `image/png,image/jpeg,image/webp,image/gif,application/pdf,${text}` : text
})
const canSend = computed(
  () =>
    ready.value &&
    !busy.value &&
    !uploading.value &&
    !!settings.group_id &&
    !!settings.model &&
    !draftAttachments.value.some((a) => a.error) &&
    (draft.value.trim().length > 0 || (settings.mode === 'chat' && draftAttachments.value.length > 0))
)
const lightboxCanReference = computed(() => storageAvailable.value && !!lightbox.value && ['image', 'generated'].includes(lightbox.value.kind))

function errorText(e: unknown): string {
  if (e && typeof e === 'object' && 'message' in e) return String((e as { message: unknown }).message)
  return String(e)
}

function effortLabel(e: string) {
  return t(`chat.efforts.${e || 'default'}`)
}

function applyConversation(conv: ChatConversation | null) {
  const pref = config.value?.preference
  if (conv) {
    settings.mode = conv.mode
    settings.group_id = conv.group_id ?? pref?.group_id ?? groups.value[0]?.id ?? null
    settings.model = conv.model
    settings.reasoning_effort = conv.reasoning_effort
    settings.image_aspect = conv.image_aspect || '1:1'
    settings.image_count = conv.image_count || 1
  } else if (pref) {
    settings.mode = pref.mode === 'image' ? 'image' : 'chat'
    settings.group_id = pref.group_id ?? groups.value[0]?.id ?? null
    settings.model = settings.mode === 'image' ? pref.image_model : pref.chat_model
    settings.reasoning_effort = pref.reasoning_effort
    settings.image_aspect = pref.image_aspect || '1:1'
    settings.image_count = pref.image_count || 1
  }
}

async function loadModels() {
  const group = settings.group_id
  const mode = settings.mode
  if (!group) {
    models.value = []
    return
  }
  const key = `${group}:${mode}`
  loadingModels.value = true
  try {
    let list = modelCache.get(key)
    if (!list) {
      list = await chatAPI.getModels(group, mode)
      modelCache.set(key, list)
    }
    if (settings.group_id !== group || settings.mode !== mode) return
    models.value = list
    if (!list.includes(settings.model)) {
      const pref = config.value?.preference
      const preferred = mode === 'image' ? pref?.image_model : pref?.chat_model
      const next = preferred && list.includes(preferred) ? preferred : list[0] || ''
      if (next !== settings.model) {
        settings.model = next
        if (next) await changeSetting({ model: next })
      }
    }
  } catch (e) {
    models.value = []
    appStore.showError(errorText(e))
  } finally {
    loadingModels.value = false
  }
}

async function changeSetting(patch: ConversationPatch) {
  const pref = config.value?.preference
  if (pref) {
    if (patch.model !== undefined) {
      if (settings.mode === 'image') pref.image_model = patch.model
      else pref.chat_model = patch.model
    }
    if (patch.reasoning_effort !== undefined) pref.reasoning_effort = patch.reasoning_effort
    if (patch.image_aspect !== undefined) pref.image_aspect = patch.image_aspect
    if (patch.image_count !== undefined) pref.image_count = patch.image_count
    if (patch.group_id !== undefined) pref.group_id = patch.group_id
    if (patch.mode !== undefined) pref.mode = patch.mode
  }
  const conv = activeConversation.value
  if (!conv) return
  try {
    replaceConversation(await chatAPI.updateConversation(conv.id, patch))
  } catch (e) {
    appStore.showError(errorText(e))
  }
}

async function changeGroup() {
  await changeSetting({ group_id: settings.group_id })
  await loadModels()
}

async function changeMode(mode: ChatMode) {
  if (busy.value || mode === settings.mode) return
  clearDraftAttachments()
  if (activeConversation.value && messages.value.length) {
    // A conversation keeps one mode; switching starts a new one.
    activeId.value = null
    messages.value = []
    if (config.value) config.value.preference.mode = mode
    applyConversation(null)
  } else {
    settings.mode = mode
    settings.model = ''
    await changeSetting({ mode })
  }
  await loadModels()
}

function replaceConversation(conv: ChatConversation) {
  const i = conversations.value.findIndex((c) => c.id === conv.id)
  if (i >= 0) conversations.value.splice(i, 1, conv)
  else conversations.value.unshift(conv)
}

function bumpConversation(conv: ChatConversation) {
  conversations.value = [conv, ...conversations.value.filter((c) => c.id !== conv.id)]
}

async function selectConversation(conv: ChatConversation) {
  listOpen.value = false
  if (busy.value || conv.id === activeId.value) return
  clearDraftAttachments()
  activeId.value = conv.id
  applyConversation(conv)
  messages.value = []
  loadingMessages.value = true
  void loadModels()
  try {
    const list = await chatAPI.listMessages(conv.id)
    if (activeId.value !== conv.id) return
    messages.value = list.map((m) => ({ ...m, aspect: conv.image_aspect }))
    stickToBottom = true
    await scrollToBottom()
  } catch (e) {
    appStore.showError(errorText(e))
  } finally {
    loadingMessages.value = false
  }
}

function startNew() {
  if (busy.value) return
  listOpen.value = false
  activeId.value = null
  messages.value = []
  clearDraftAttachments()
  applyConversation(null)
  void loadModels()
  void nextTick(() => composer.value?.focus())
}

async function renameConversation(conv: ChatConversation, title: string) {
  try {
    replaceConversation(await chatAPI.updateConversation(conv.id, { title }))
  } catch (e) {
    appStore.showError(errorText(e))
  }
}

async function confirmDelete() {
  const conv = deleteTarget.value
  deleteTarget.value = null
  if (!conv) return
  try {
    await chatAPI.deleteConversation(conv.id)
    conversations.value = conversations.value.filter((c) => c.id !== conv.id)
    if (activeId.value === conv.id) startNew()
  } catch (e) {
    appStore.showError(errorText(e))
  }
}

// ---------- Attachments ----------

function limitFor(file: File): number {
  const limits = config.value?.limits
  if (!limits) return Infinity
  if (file.type.startsWith('image/')) return limits.max_image_bytes
  if (file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf')) return limits.max_pdf_bytes
  return limits.max_text_bytes
}

function addFiles(files: File[]) {
  const max = config.value?.limits.max_attachments || 10
  for (const file of files) {
    if (draftAttachments.value.length >= max) {
      appStore.showError(t('chat.tooMany', { n: max }))
      break
    }
    if (settings.mode === 'image' && !file.type.startsWith('image/')) {
      appStore.showError(t('chat.imageOnly'))
      continue
    }
    const limit = limitFor(file)
    if (file.size > limit) {
      appStore.showError(t('chat.tooLarge', { name: file.name, size: Math.round(limit / (1 << 20)) }))
      continue
    }
    const item = reactive<DraftAttachment>({
      key: `d${++draftSeq}`,
      name: file.name,
      progress: 0,
      previewUrl: file.type.startsWith('image/') ? URL.createObjectURL(file) : undefined,
    })
    draftAttachments.value.push(item)
    chatAPI
      .uploadAttachment(file, (p) => (item.progress = p))
      .then((att) => {
        if (!draftAttachments.value.includes(item)) {
          void chatAPI.deleteAttachment(att.id).catch(() => undefined)
          return
        }
        item.attachment = att
      })
      .catch((e) => {
        item.error = t('chat.uploadFailed', { error: errorText(e) })
      })
  }
}

function removeDraftAttachment(key: string) {
  const i = draftAttachments.value.findIndex((a) => a.key === key)
  if (i < 0) return
  const [item] = draftAttachments.value.splice(i, 1)
  if (item.previewUrl && !item.reused) URL.revokeObjectURL(item.previewUrl)
  if (item.attachment && !item.reused) void chatAPI.deleteAttachment(item.attachment.id).catch(() => undefined)
}

function clearDraftAttachments(deleteUploads = true) {
  for (const item of draftAttachments.value) {
    if (item.previewUrl && !item.reused) URL.revokeObjectURL(item.previewUrl)
    if (deleteUploads && item.attachment && !item.reused) void chatAPI.deleteAttachment(item.attachment.id).catch(() => undefined)
  }
  draftAttachments.value = []
}

async function useAsReference(att: ChatAttachment) {
  lightbox.value = null
  if (settings.mode !== 'image') await changeMode('image')
  if (draftAttachments.value.some((a) => a.attachment?.id === att.id)) return
  let previewUrl: string | undefined
  try {
    previewUrl = await attachmentUrl(att.id)
  } catch {
    previewUrl = undefined
  }
  draftAttachments.value.push({ key: `d${++draftSeq}`, name: att.filename, progress: 1, attachment: att, previewUrl, reused: true })
  void nextTick(() => composer.value?.focus())
}

// ---------- Sending ----------

function onScroll() {
  const el = scroller.value
  if (!el) return
  stickToBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80
}

async function scrollToBottom(force = false) {
  await nextTick()
  const el = scroller.value
  if (el && (force || stickToBottom)) el.scrollTop = el.scrollHeight
}

async function ensureConversation(): Promise<ChatConversation> {
  if (activeConversation.value) return activeConversation.value
  const conv = await chatAPI.createConversation({
    mode: settings.mode,
    group_id: settings.group_id,
    model: settings.model,
    reasoning_effort: settings.reasoning_effort,
    image_aspect: settings.image_aspect,
    image_count: settings.image_count,
  })
  conversations.value.unshift(conv)
  activeId.value = conv.id
  return conv
}

function handleEvent(ev: ChatStreamEvent, tempUserId: number | null) {
  const list = messages.value
  const assistant = list[list.length - 1]
  switch (ev.type) {
    case 'start': {
      bumpConversation(ev.conversation)
      if (!ev.regenerate && tempUserId !== null) {
        const i = list.findIndex((m) => m.id === tempUserId)
        if (i >= 0) list.splice(i, 1, { ...ev.user_message, aspect: ev.aspect })
      }
      list.push({ ...ev.assistant_message, attachments: [], citations: [], aspect: ev.aspect || settings.image_aspect, pendingImages: ev.count || 0, imageErrors: [] })
      break
    }
    case 'delta':
      if (assistant?.role === 'assistant') {
        assistant.content += ev.text
        assistant.searching = false
      }
      break
    case 'reasoning':
      if (assistant?.role === 'assistant') assistant.reasoning = (assistant.reasoning || '') + ev.text
      break
    case 'search':
      if (assistant?.role === 'assistant') {
        if (ev.status === 'searching') assistant.searching = true
        else {
          assistant.searching = false
          assistant.searchQueries = [...(assistant.searchQueries || []), ev.query || '']
        }
      }
      break
    case 'citations':
      if (assistant?.role === 'assistant') assistant.citations = ev.citations
      break
    case 'image':
      if (assistant?.role === 'assistant') {
        assistant.attachments = [...assistant.attachments, ev.attachment]
        assistant.pendingImages = Math.max(0, (assistant.pendingImages || 0) - 1)
      }
      break
    case 'image_error':
      if (assistant?.role === 'assistant') {
        assistant.imageErrors = [...(assistant.imageErrors || []), { index: ev.index, error: ev.error }]
        assistant.pendingImages = Math.max(0, (assistant.pendingImages || 0) - 1)
      }
      break
    case 'done':
      if (assistant?.role === 'assistant') {
        Object.assign(assistant, ev.message, {
          reasoning: ev.message.reasoning || assistant.reasoning,
          searching: false,
          pendingImages: 0,
          attachments: ev.message.attachments?.length ? ev.message.attachments : assistant.attachments,
        })
      }
      break
    case 'error':
      if (assistant?.role === 'assistant') {
        assistant.status = 'error'
        assistant.error = ev.message
        assistant.pendingImages = 0
        assistant.searching = false
      } else {
        appStore.showError(ev.message)
      }
      break
  }
  void scrollToBottom()
}

async function run(conv: ChatConversation, regenerate: boolean) {
  const text = draft.value.trim()
  const attachments = draftAttachments.value.filter((a) => a.attachment).map((a) => a.attachment as ChatAttachment)
  let tempUserId: number | null = null
  if (!regenerate) {
    tempUserId = -Date.now()
    messages.value.push({
      id: tempUserId,
      conversation_id: conv.id,
      role: 'user',
      content: text,
      status: 'complete',
      input_tokens: 0,
      output_tokens: 0,
      citations: [],
      created_at: new Date().toISOString(),
      attachments,
    })
    draft.value = ''
    clearDraftAttachments(false)
  }
  stickToBottom = true
  void scrollToBottom(true)
  busy.value = true
  abort = new AbortController()
  let started = false
  const onEvent = (ev: ChatStreamEvent) => {
    if (ev.type === 'start') started = true
    handleEvent(ev, tempUserId)
  }
  try {
    const ids = attachments.map((a) => a.id)
    if (conv.mode === 'image') {
      await chatAPI.sendImages(conv.id, { prompt: text, attachment_ids: ids, regenerate }, onEvent, abort.signal)
    } else {
      await chatAPI.sendMessage(conv.id, { text, attachment_ids: ids, regenerate, web_search: webSearch.value }, onEvent, abort.signal)
    }
  } catch (e) {
    const aborted = (e as { name?: string })?.name === 'AbortError'
    const last = messages.value[messages.value.length - 1]
    if (!started && !regenerate && !aborted) {
      // Nothing was stored; put the text back so it can be retried.
      messages.value = messages.value.filter((m) => m.id !== tempUserId)
      draft.value = text
      appStore.showError(errorText(e))
    } else if (last?.role === 'assistant' && last.status === 'streaming') {
      last.status = aborted ? 'aborted' : 'error'
      if (!aborted) last.error = errorText(e)
      last.pendingImages = 0
      last.searching = false
    } else if (!aborted) {
      appStore.showError(errorText(e))
    }
  } finally {
    const last = messages.value[messages.value.length - 1]
    if (last?.role === 'assistant' && last.status === 'streaming') {
      // The stream closed without a final event.
      last.status = 'error'
      last.pendingImages = 0
      last.searching = false
    }
    busy.value = false
    abort = null
  }
}

async function send() {
  if (!canSend.value) return
  try {
    const conv = await ensureConversation()
    await run(conv, false)
  } catch (e) {
    appStore.showError(errorText(e))
  }
}

async function regenerate() {
  const conv = activeConversation.value
  if (!conv || busy.value) return
  const last = messages.value[messages.value.length - 1]
  if (last?.role === 'assistant') messages.value.pop()
  await run(conv, true)
}

function stop() {
  abort?.abort()
}

async function saveWebSearch(value: boolean) {
  if (config.value) config.value.preference.web_search = value
  try {
    await chatAPI.updatePreference({ web_search: value })
  } catch {
    // the next send stores it as well
  }
}

onMounted(async () => {
  try {
    const [cfg, list] = await Promise.all([chatAPI.getConfig(), chatAPI.listConversations()])
    config.value = cfg
    conversations.value = list
    webSearch.value = cfg.preference.web_search
    if (!cfg.enabled) pageError.value = t('chat.disabled')
    else if (!cfg.groups.length) pageError.value = t('chat.noGroups')
    applyConversation(null)
  } catch (e) {
    pageError.value = errorText(e)
  } finally {
    loadingConfig.value = false
  }
  if (!pageError.value) {
    if (conversations.value.length) await selectConversation(conversations.value[0])
    else await loadModels()
  }
})

onBeforeUnmount(() => {
  abort?.abort()
  clearDraftAttachments()
  releaseAttachmentUrls()
})
</script>
