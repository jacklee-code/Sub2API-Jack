<template>
  <div class="group relative max-w-full" :style="boxStyle">
    <button
      type="button"
      class="block h-full w-full overflow-hidden rounded-xl border border-gray-200 bg-gray-100 dark:border-dark-600 dark:bg-dark-700"
      :title="attachment.filename"
      @click="emit('open', attachment)"
    >
      <img v-if="url" :src="url" :alt="attachment.filename" class="h-full w-full object-contain" draggable="false" />
      <span v-else-if="failed" class="flex h-full w-full items-center justify-center p-2 text-xs text-gray-400">{{ attachment.filename }}</span>
      <span v-else class="block h-full w-full animate-pulse bg-gray-200 dark:bg-dark-600"></span>
    </button>
    <div
      v-if="actions && url"
      class="pointer-events-none absolute inset-x-0 bottom-0 flex justify-end gap-1 rounded-b-xl bg-gradient-to-t from-black/50 to-transparent p-1.5 opacity-0 transition-opacity group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100"
    >
      <button type="button" class="rounded-md bg-black/40 p-1.5 text-white hover:bg-black/60" :title="t('chat.download')" :aria-label="t('chat.download')" @click.stop="download">
        <Icon name="download" size="xs" />
      </button>
      <button v-if="canReference" type="button" class="rounded-md bg-black/40 p-1.5 text-white hover:bg-black/60" :title="t('chat.useAsReference')" :aria-label="t('chat.useAsReference')" @click.stop="emit('reference', attachment)">
        <Icon name="edit" size="xs" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ChatAttachment } from '@/api/chat'
import { attachmentUrl, downloadAttachment } from './attachmentUrls'
import { fitBox } from './imageSize'

const props = withDefaults(defineProps<{ attachment: ChatAttachment; maxEdge?: number; aspect?: string; actions?: boolean; canReference?: boolean }>(), {
  maxEdge: 320,
  aspect: '1:1',
  actions: false,
  canReference: false,
})
const emit = defineEmits<{ open: [attachment: ChatAttachment]; reference: [attachment: ChatAttachment] }>()
const { t } = useI18n()

const url = ref('')
const failed = ref(false)

const boxStyle = computed(() => {
  const box = fitBox(props.attachment.width, props.attachment.height, props.maxEdge, props.aspect)
  return { width: `${box.width}px`, aspectRatio: `${box.width} / ${box.height}` }
})

async function load() {
  failed.value = false
  try {
    url.value = await attachmentUrl(props.attachment.id)
  } catch {
    failed.value = true
  }
}

function download() {
  void downloadAttachment(props.attachment.id, props.attachment.filename)
}

onMounted(load)
watch(() => props.attachment.id, load)
</script>
