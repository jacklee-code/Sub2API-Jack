<template>
  <Teleport to="body">
    <div
      v-if="attachment"
      class="fixed inset-0 z-[100] flex flex-col bg-black/85 p-4"
      role="dialog"
      aria-modal="true"
      @click.self="emit('close')"
    >
      <div class="flex items-center justify-between gap-3 pb-3 text-sm text-white/90">
        <span class="truncate">{{ attachment.filename }}<template v-if="attachment.width && attachment.height"> · {{ attachment.width }}×{{ attachment.height }}</template></span>
        <div class="flex shrink-0 items-center gap-2">
          <button v-if="canReference" type="button" class="rounded-lg bg-white/10 px-3 py-1.5 hover:bg-white/20" @click="emit('reference', attachment)">
            {{ t('chat.useAsReference') }}
          </button>
          <button type="button" class="rounded-lg bg-white/10 px-3 py-1.5 hover:bg-white/20" @click="download">
            {{ t('chat.download') }}
          </button>
          <button type="button" class="rounded-lg bg-white/10 px-3 py-1.5 hover:bg-white/20" :aria-label="t('chat.close')" @click="emit('close')">
            <Icon name="x" size="sm" />
          </button>
        </div>
      </div>
      <div class="flex min-h-0 flex-1 items-center justify-center" @click.self="emit('close')">
        <img v-if="url" :src="url" :alt="attachment.filename" class="max-h-full max-w-full object-contain" />
        <span v-else class="h-10 w-10 animate-spin rounded-full border-2 border-white/30 border-t-white"></span>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ChatAttachment } from '@/api/chat'
import { attachmentUrl, downloadAttachment } from './attachmentUrls'

const props = defineProps<{ attachment: ChatAttachment | null; canReference?: boolean }>()
const emit = defineEmits<{ close: []; reference: [attachment: ChatAttachment] }>()
const { t } = useI18n()
const url = ref('')

watch(
  () => props.attachment?.id,
  async (id) => {
    url.value = ''
    if (!id) return
    try {
      url.value = await attachmentUrl(id)
    } catch {
      url.value = ''
    }
  },
  { immediate: true }
)

function download() {
  if (props.attachment) void downloadAttachment(props.attachment.id, props.attachment.filename)
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.attachment) emit('close')
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>
