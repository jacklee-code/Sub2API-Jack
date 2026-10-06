<template>
  <button
    type="button"
    class="group relative block max-w-full overflow-hidden rounded-xl border border-gray-200 bg-gray-100 dark:border-dark-600 dark:bg-dark-700"
    :style="boxStyle"
    :title="attachment.filename"
    @click="emit('open', attachment)"
  >
    <img v-if="url" :src="url" :alt="attachment.filename" class="h-full w-full object-contain" draggable="false" />
    <span v-else-if="failed" class="flex h-full w-full items-center justify-center p-2 text-xs text-gray-400">{{ attachment.filename }}</span>
    <span v-else class="block h-full w-full animate-pulse bg-gray-200 dark:bg-dark-600"></span>
  </button>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { ChatAttachment } from '@/api/chat'
import { attachmentUrl } from './attachmentUrls'
import { fitBox } from './imageSize'

const props = withDefaults(defineProps<{ attachment: ChatAttachment; maxEdge?: number; aspect?: string }>(), {
  maxEdge: 320,
  aspect: '1:1',
})
const emit = defineEmits<{ open: [attachment: ChatAttachment] }>()

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

onMounted(load)
watch(() => props.attachment.id, load)
</script>
