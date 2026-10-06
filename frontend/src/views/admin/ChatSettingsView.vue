<template>
  <AppLayout>
    <div class="mx-auto max-w-3xl space-y-6 p-4 sm:p-6">
      <div>
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('chat.settings.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500">{{ t('chat.settings.description') }}</p>
      </div>
      <p v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950">{{ error }}</p>

      <form v-if="form" class="card space-y-6 p-5" @submit.prevent="save">
        <div class="flex items-start justify-between gap-4">
          <div>
            <div class="font-medium text-gray-900 dark:text-white">{{ t('chat.settings.enabled') }}</div>
            <p class="mt-1 text-sm text-gray-500">{{ t('chat.settings.enabledHint') }}</p>
          </div>
          <Toggle v-model="form.enabled" data-testid="chat-settings-enabled" />
        </div>

        <div>
          <label class="input-label" for="chat-system-prompt">{{ t('chat.settings.systemPrompt') }}</label>
          <textarea id="chat-system-prompt" v-model="form.system_prompt" rows="4" class="input" maxlength="20000"></textarea>
          <p class="input-hint">{{ t('chat.settings.systemPromptHint') }}</p>
        </div>

        <div>
          <h2 class="mb-3 font-medium text-gray-900 dark:text-white">{{ t('chat.settings.limits') }}</h2>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="input-label" for="chat-max-image">{{ t('chat.settings.maxImage') }}</label>
              <input id="chat-max-image" v-model.number="mb.image" type="number" min="1" max="64" class="input" />
            </div>
            <div>
              <label class="input-label" for="chat-max-pdf">{{ t('chat.settings.maxPdf') }}</label>
              <input id="chat-max-pdf" v-model.number="mb.pdf" type="number" min="1" max="64" class="input" />
            </div>
            <div>
              <label class="input-label" for="chat-max-text">{{ t('chat.settings.maxText') }}</label>
              <input id="chat-max-text" v-model.number="mb.text" type="number" min="1" max="16" class="input" />
            </div>
            <div>
              <label class="input-label" for="chat-max-attachments">{{ t('chat.settings.maxAttachments') }}</label>
              <input id="chat-max-attachments" v-model.number="form.max_attachments" type="number" min="1" max="32" class="input" />
            </div>
            <div>
              <label class="input-label" for="chat-max-conversations">{{ t('chat.settings.maxConversations') }}</label>
              <input id="chat-max-conversations" v-model.number="form.max_conversations" type="number" min="1" class="input" />
            </div>
            <div class="sm:col-span-2">
              <label class="input-label" for="chat-max-context">{{ t('chat.settings.maxContextTokens') }}</label>
              <input id="chat-max-context" v-model.number="form.max_context_tokens" type="number" min="8000" max="2000000" step="1000" class="input" />
              <p class="input-hint">{{ t('chat.settings.maxContextTokensHint') }}</p>
            </div>
          </div>
        </div>

        <div>
          <h2 class="mb-2 font-medium text-gray-900 dark:text-white">{{ t('chat.settings.storage') }}</h2>
          <p :class="['rounded-xl px-3 py-2 text-sm', storageAvailable ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300' : 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300']">
            {{ storageAvailable ? t('chat.settings.storageOk') : t('chat.settings.storageMissing') }}
          </p>
        </div>

        <div class="flex justify-end">
          <button type="submit" class="btn btn-primary" :disabled="saving">{{ t('chat.settings.save') }}</button>
        </div>
      </form>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Toggle from '@/components/common/Toggle.vue'
import { getAdminSettings, updateAdminSettings, type ChatAdminSettings } from '@/api/chat'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()
const MB = 1 << 20

const form = ref<ChatAdminSettings | null>(null)
const mb = reactive({ image: 20, pdf: 32, text: 2 })
const storageAvailable = ref(false)
const saving = ref(false)
const error = ref('')

function apply(result: { settings: ChatAdminSettings; storage_available: boolean }) {
  form.value = { ...result.settings }
  mb.image = Math.round(result.settings.max_image_bytes / MB)
  mb.pdf = Math.round(result.settings.max_pdf_bytes / MB)
  mb.text = Math.max(1, Math.round(result.settings.max_text_bytes / MB))
  storageAvailable.value = result.storage_available
}

async function save() {
  if (!form.value) return
  saving.value = true
  error.value = ''
  try {
    apply(
      await updateAdminSettings({
        ...form.value,
        max_image_bytes: Math.round(mb.image * MB),
        max_pdf_bytes: Math.round(mb.pdf * MB),
        max_text_bytes: Math.round(mb.text * MB),
      })
    )
    appStore.showSuccess(t('chat.settings.saved'))
  } catch (e) {
    error.value = (e as { message?: string })?.message || String(e)
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  try {
    apply(await getAdminSettings())
  } catch (e) {
    error.value = (e as { message?: string })?.message || String(e)
  }
})
</script>
