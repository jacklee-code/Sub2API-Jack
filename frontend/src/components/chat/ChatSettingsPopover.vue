<template>
  <div ref="root" class="relative">
    <button
      type="button"
      class="flex max-w-[16rem] items-center gap-1.5 rounded-full bg-gray-100 px-3 py-1.5 text-sm text-gray-700 transition-colors hover:bg-gray-200 disabled:opacity-50 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600"
      :disabled="disabled"
      :aria-expanded="open"
      aria-haspopup="dialog"
      data-testid="chat-settings"
      @click="toggle"
    >
      <span class="truncate font-medium">{{ model || t('chat.chooseModel') }}</span>
      <span class="shrink-0 text-gray-400">·</span>
      <span class="shrink-0 text-gray-500 dark:text-gray-400">{{ summary }}</span>
      <Icon name="chevronDown" size="xs" class="shrink-0 text-gray-400" />
    </button>

    <div
      v-if="open"
      class="card absolute bottom-full right-0 z-30 mb-2 w-[min(20rem,calc(100vw-2rem))] !rounded-2xl p-4 shadow-lg"
      role="dialog"
      :aria-label="t('chat.settingsTitle')"
    >
      <!-- Model list -->
      <template v-if="view === 'models'">
        <div class="mb-3 flex items-center gap-2">
          <button type="button" class="rounded-md p-1 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-700" :aria-label="t('chat.back')" @click="view = 'main'">
            <Icon name="chevronLeft" size="sm" />
          </button>
          <span class="text-sm font-medium text-gray-900 dark:text-white">{{ t('chat.model') }}</span>
        </div>
        <div v-if="groups.length > 1" class="mb-3 flex flex-wrap gap-1.5">
          <button
            v-for="g in groups"
            :key="g.id"
            type="button"
            :class="[
              'rounded-full border px-2.5 py-1 text-xs transition-colors',
              g.id === groupId
                ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/30 dark:text-primary-200'
                : 'border-gray-200 text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700'
            ]"
            :data-testid="`chat-group-${g.id}`"
            @click="emit('update:groupId', g.id)"
          >
            {{ g.name }}
          </button>
        </div>
        <p v-if="loadingModels" class="py-4 text-center text-sm text-gray-400">{{ t('chat.loadingModels') }}</p>
        <p v-else-if="!models.length" class="py-4 text-center text-sm text-gray-400">{{ t('chat.noModels') }}</p>
        <ul v-else class="-mx-1 max-h-64 overflow-y-auto">
          <li v-for="m in models" :key="m">
            <button
              type="button"
              class="flex w-full items-center justify-between rounded-lg px-2 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-700"
              :class="m === model ? 'font-medium text-gray-900 dark:text-white' : 'text-gray-600 dark:text-gray-300'"
              :data-testid="`chat-model-${m}`"
              @click="pickModel(m)"
            >
              <span class="truncate">{{ m }}</span>
              <Icon v-if="m === model" name="check" size="sm" class="shrink-0 text-primary-600 dark:text-primary-400" />
            </button>
          </li>
        </ul>
      </template>

      <!-- Chat: reasoning effort -->
      <template v-else-if="mode === 'chat'">
        <div class="flex items-start justify-between">
          <Icon name="bolt" size="sm" class="mt-1 text-gray-400" />
          <div class="min-w-0 flex-1 text-center">
            <div class="text-xl font-semibold" :class="effort ? 'text-primary-600 dark:text-primary-400' : 'text-gray-500 dark:text-gray-400'" data-testid="chat-effort-label">
              {{ effortLabel(effort) }}
            </div>
            <button type="button" class="mx-auto mt-0.5 flex max-w-full items-center gap-0.5 text-sm text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100" data-testid="chat-open-models" @click="view = 'models'">
              <span class="truncate">{{ groupName }} / {{ model || t('chat.chooseModel') }}</span>
              <Icon name="chevronRight" size="xs" class="shrink-0" />
            </button>
          </div>
          <button
            type="button"
            class="rounded-md p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700 disabled:opacity-30 dark:hover:bg-dark-700 dark:hover:text-gray-200"
            :title="t('chat.efforts.default')"
            :aria-label="t('chat.resetEffort')"
            :disabled="!effort"
            data-testid="chat-effort-reset"
            @click="emit('update:effort', '')"
          >
            <Icon name="refresh" size="sm" />
          </button>
        </div>
        <div class="relative mt-5 h-6">
          <div class="absolute inset-x-3 top-1/2 h-2.5 -translate-y-1/2 rounded-full bg-gray-200 dark:bg-dark-600">
            <div class="h-full rounded-full transition-all" :class="effort ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-500'" :style="{ width: `${fillPct}%` }"></div>
          </div>
          <div class="absolute inset-x-3 top-1/2 flex -translate-y-1/2 justify-between px-[3px]">
            <span v-for="(e, i) in levels" :key="e" class="h-1.5 w-1.5 rounded-full" :class="i <= levelIndex && effort ? 'bg-white/80' : 'bg-gray-400/60 dark:bg-gray-500/60'"></span>
          </div>
          <div
            class="pointer-events-none absolute top-1/2 h-6 w-6 -translate-x-1/2 -translate-y-1/2 rounded-full border border-gray-200 bg-white shadow transition-all dark:border-dark-400"
            :style="{ left: `calc(0.75rem + (100% - 1.5rem) * ${fillPct / 100})` }"
          ></div>
          <input
            type="range"
            class="absolute inset-0 h-full w-full cursor-pointer opacity-0"
            :min="0"
            :max="levels.length - 1"
            step="1"
            :value="levelIndex"
            :aria-label="t('chat.effort')"
            :aria-valuetext="effortLabel(effort)"
            data-testid="chat-effort-range"
            @input="onRange"
          />
        </div>
        <div class="mt-1 flex justify-between px-1 text-[11px] text-gray-400">
          <span>{{ effortLabel(levels[0]) }}</span>
          <span>{{ effortLabel(levels[levels.length - 1]) }}</span>
        </div>
      </template>

      <!-- Image: aspect ratio and count -->
      <template v-else>
        <button type="button" class="mb-3 flex w-full items-center justify-between rounded-lg bg-gray-50 px-3 py-2 text-sm hover:bg-gray-100 dark:bg-dark-700 dark:hover:bg-dark-600" data-testid="chat-open-models" @click="view = 'models'">
          <span class="min-w-0 truncate text-left"><span class="text-gray-500 dark:text-gray-400">{{ groupName }} /</span> <span class="font-medium text-gray-900 dark:text-white">{{ model || t('chat.chooseModel') }}</span></span>
          <Icon name="chevronRight" size="xs" class="shrink-0 text-gray-400" />
        </button>
        <div class="mb-1.5 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('chat.aspect') }}</div>
        <div class="grid grid-cols-4 gap-1.5">
          <button
            v-for="a in aspects"
            :key="a"
            type="button"
            :class="[
              'flex h-14 flex-col items-center justify-center gap-1 rounded-lg border text-[11px] transition-colors',
              a === aspect
                ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/30 dark:text-primary-200'
                : 'border-gray-200 text-gray-500 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-400 dark:hover:bg-dark-700'
            ]"
            :aria-pressed="a === aspect"
            :data-testid="`chat-aspect-${a}`"
            @click="emit('update:aspect', a)"
          >
            <span class="rounded-[3px] border-[1.5px] border-current" :style="shapeStyle(a)"></span>
            <span>{{ a }}</span>
          </button>
        </div>
        <div class="mb-1.5 mt-3 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('chat.count') }}</div>
        <div class="grid gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-700" :style="{ gridTemplateColumns: `repeat(${maxCount}, minmax(0, 1fr))` }">
          <button
            v-for="n in maxCount"
            :key="n"
            type="button"
            :class="[
              'rounded-md py-1 text-sm transition-colors',
              n === count ? 'bg-white font-medium text-gray-900 shadow-sm dark:bg-dark-500 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'
            ]"
            :aria-pressed="n === count"
            :data-testid="`chat-count-${n}`"
            @click="emit('update:count', n)"
          >
            {{ n }}
          </button>
        </div>
        <p class="mt-3 text-xs text-gray-400">{{ t('chat.resolutionHint') }}</p>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ChatGroup, ChatMode } from '@/api/chat'
import { fitBox } from './imageSize'

const props = defineProps<{
  mode: ChatMode
  groups: ChatGroup[]
  groupId: number | null
  models: string[]
  model: string
  loadingModels: boolean
  efforts: string[]
  effort: string
  aspects: string[]
  aspect: string
  count: number
  maxCount: number
  disabled?: boolean
}>()
const emit = defineEmits<{
  'update:groupId': [value: number]
  'update:model': [value: string]
  'update:effort': [value: string]
  'update:aspect': [value: string]
  'update:count': [value: number]
}>()
const { t } = useI18n()

const root = ref<HTMLElement | null>(null)
const open = ref(false)
const view = ref<'main' | 'models'>('main')

// The slider covers the explicit levels; "default" is the reset button.
const levels = computed(() => {
  const list = props.efforts.filter((e) => e)
  return list.length ? list : ['low', 'medium', 'high']
})
const levelIndex = computed(() => {
  const i = levels.value.indexOf(props.effort)
  return i >= 0 ? i : Math.max(0, levels.value.indexOf('medium'))
})
const fillPct = computed(() => (levels.value.length > 1 ? (levelIndex.value / (levels.value.length - 1)) * 100 : 0))
const groupName = computed(() => props.groups.find((g) => g.id === props.groupId)?.name || t('chat.group'))
const summary = computed(() =>
  props.mode === 'image' ? `${props.aspect} · ${t('chat.countShort', { n: props.count })}` : effortLabel(props.effort)
)

function effortLabel(e: string) {
  return t(`chat.efforts.${e || 'default'}`)
}

function onRange(e: Event) {
  const i = Number((e.target as HTMLInputElement).value)
  const next = levels.value[i]
  if (next && next !== props.effort) emit('update:effort', next)
}

function pickModel(m: string) {
  emit('update:model', m)
  view.value = 'main'
}

function shapeStyle(aspect: string) {
  const box = fitBox(undefined, undefined, 20, aspect)
  return { width: `${box.width}px`, height: `${box.height}px` }
}

function toggle() {
  open.value = !open.value
  view.value = props.model ? 'main' : 'models'
}

function onDocClick(e: MouseEvent) {
  if (open.value && root.value && !root.value.contains(e.target as Node)) open.value = false
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('mousedown', onDocClick)
  document.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocClick)
  document.removeEventListener('keydown', onKey)
})
</script>
