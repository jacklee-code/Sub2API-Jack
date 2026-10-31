<template>
  <section class="card space-y-3 p-4" data-testid="reserve-energy-settings">
    <label class="flex items-center gap-2 font-medium">
      <input v-model="enabled" type="checkbox" @change="changed" />
      {{ reserveText('title') }}
    </label>
    <p class="input-hint">{{ reserveText('description') }}</p>
    <template v-if="enabled">
      <ol class="space-y-2">
        <li v-for="(id, index) in ids" :key="id" class="flex items-center gap-2">
          <span class="text-sm tabular-nums">{{ index + 1 }}.</span>
          <span class="min-w-0 flex-1 truncate">{{ accountName(id) }}</span>
          <button type="button" class="btn btn-secondary px-2" :disabled="index === 0" :aria-label="reserveText('up')" @click="move(index, -1)">↑</button>
          <button type="button" class="btn btn-secondary px-2" :disabled="index === ids.length - 1" :aria-label="reserveText('down')" @click="move(index, 1)">↓</button>
          <button type="button" class="btn btn-secondary" :aria-label="`${reserveText('remove')} ${accountName(id)}`" @click="remove(index)">{{ reserveText('remove') }}</button>
        </li>
      </ol>
      <label class="input-label" for="jack-reserve-add">{{ reserveText('add') }}</label>
      <select id="jack-reserve-add" class="input" :required="ids.length === 0" :disabled="loading || ids.length >= 16" value="" @change="add">
        <option value="">{{ loading ? reserveText('loading') : reserveText('choose') }}</option>
        <option v-for="item in available" :key="item.id" :value="item.id">{{ item.name }} · #{{ item.id }} · {{ item.type === 'apikey' ? 'API Key' : 'OAuth' }}</option>
      </select>
      <p class="input-hint">{{ reserveText('proxy') }}</p>
      <p v-if="loadError" role="alert" class="text-sm text-red-600">{{ reserveText('loadError') }} <button type="button" class="underline" @click="loadAccounts">{{ reserveText('retry') }}</button></p>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { list } from '@/api/admin/accounts'
import type { AccountListItem } from '@/types'

const props = defineProps<{ accountId: number; extra?: Record<string, unknown> }>()
const emit = defineEmits<{ change: [extra: Record<string, unknown>] }>()
const enabled = ref(false)
const ids = ref<number[]>([])
const accounts = ref<AccountListItem[]>([])
const loading = ref(false)
const loadError = ref(false)
let loadVersion = 0

const messages = {
  en: {
    title: 'Reserve energy', description: 'Use the primary’s subscription quota and available Credit first, then reserves in this order. Always return to the primary when its quota recovers.',
    proxy: 'Each reserve uses its own credentials and proxy (direct if none). Fill first: wait for the first healthy reserve; use the next on quota exhaustion or a retryable failure. Plans do not recurse. Up to 16 accounts.',
    add: 'Add a reserve account', choose: 'Choose an OpenAI account', loading: 'Loading accounts…', up: 'Move up', down: 'Move down', remove: 'Remove', loadError: 'Could not load accounts.', retry: 'Retry'
  },
  zh: {
    title: '后备能源', description: '优先使用主账号订阅额度及可用 Credit，用尽后按以下顺序使用后备账号；主账号额度恢复后自动切回。',
    proxy: '使用后备账号自己的凭证和代理（未设代理则直连）。优先用第一个可用账号，并发满时排队；额度耗尽或可重试错误时才换下一个。不递归使用后备计划，最多 16 个账号。',
    add: '添加后备账号', choose: '选择 OpenAI 账号', loading: '正在加载账号…', up: '上移', down: '下移', remove: '移除', loadError: '无法加载账号。', retry: '重试'
  },
  'zh-Hant': {
    title: '後備能源', description: '優先使用主帳號訂閱額度及可用 Credit，用盡後按以下順序使用後備帳號；主帳號額度恢復後自動切回。',
    proxy: '使用後備帳號自己的憑證和代理（未設代理則直連）。優先用第一個可用帳號，併發滿時排隊；額度耗盡或可重試錯誤時才換下一個。不遞迴使用後備計劃，最多 16 個帳號。',
    add: '新增後備帳號', choose: '選擇 OpenAI 帳號', loading: '正在載入帳號…', up: '上移', down: '下移', remove: '移除', loadError: '無法載入帳號。', retry: '重試'
  }
}
const { locale } = useI18n()
function reserveText(key: keyof typeof messages.en) {
  return (messages[(locale?.value ?? 'en') as keyof typeof messages] ?? messages.en)[key]
}

const available = computed(() => accounts.value.filter(a => a.id !== props.accountId && !a.parent_account_id && !ids.value.includes(a.id)))
function accountName(id: number) { return accounts.value.find(a => a.id === id)?.name ?? `#${id}` }
function changed() { emit('change', { jack_reserve_enabled: enabled.value, jack_reserve_account_ids: [...ids.value] }) }
function add(event: Event) {
  const select = event.target as HTMLSelectElement
  const id = Number(select.value)
  if (Number.isSafeInteger(id) && id > 0 && id !== props.accountId && !ids.value.includes(id) && ids.value.length < 16) {
    ids.value.push(id)
    changed()
  }
  select.value = ''
}
function remove(index: number) { ids.value.splice(index, 1); changed() }
function move(index: number, direction: number) {
  const next = index + direction
  if (next < 0 || next >= ids.value.length) return
  const reordered = [...ids.value]
  const [id] = reordered.splice(index, 1)
  if (id === undefined) return
  reordered.splice(next, 0, id)
  ids.value = reordered
  changed()
}
async function loadAccounts() {
  const version = ++loadVersion
  loading.value = true
  loadError.value = false
  try {
    const found: AccountListItem[] = []
    for (let page = 1; ; page++) {
      const response = await list(page, 100, { platform: 'openai', lite: 'true' })
      if (version !== loadVersion) return
      found.push(...response.items)
      if (found.length >= response.total || response.items.length === 0) break
    }
    accounts.value = found
  } catch {
    if (version === loadVersion) loadError.value = true
  } finally {
    if (version === loadVersion) loading.value = false
  }
}
watch(() => props.accountId, () => {
  enabled.value = props.extra?.jack_reserve_enabled === true
  const raw = props.extra?.jack_reserve_account_ids
  ids.value = Array.isArray(raw) ? raw.filter((id): id is number => Number.isSafeInteger(id) && id > 0) : []
}, { immediate: true })
watch(enabled, value => { if (value) void loadAccounts() }, { immediate: true })
</script>
