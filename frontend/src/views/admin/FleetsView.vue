<template>
  <AppLayout>
    <div class="space-y-6 p-4 sm:p-6">
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div><h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('fleet.title') }}</h1><p class="mt-1 text-sm text-gray-500">{{ t('fleet.description') }}</p></div>
        <div class="flex flex-wrap gap-2">
          <button class="btn btn-secondary" :disabled="loading || busy" @click="load">{{ t('fleet.refresh') }}</button>
          <button class="btn btn-secondary" :disabled="!fleets.length || busy" @click="open('reset_all')">{{ t('fleet.resetAll') }}</button>
          <button class="btn btn-primary" :disabled="busy" @click="open('create')">{{ t('fleet.create') }}</button>
        </div>
      </div>
      <p v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950">{{ error }}</p>
      <div class="flex flex-wrap gap-6 rounded-2xl border border-gray-200 bg-white px-5 py-4 dark:border-dark-700 dark:bg-dark-800">
        <div><span class="text-2xl font-semibold">{{ fleets.length }}</span><span class="ml-2 text-sm text-gray-500">{{ t('fleet.fleets') }}</span></div>
        <div><span class="text-2xl font-semibold">{{ memberCount }}</span><span class="ml-2 text-sm text-gray-500">{{ t('fleet.members') }}</span></div>
        <p class="my-auto text-sm text-gray-500">{{ t('fleet.dragHint') }}</p>
      </div>
      <p v-if="loading && !fleets.length" class="py-10 text-center text-gray-500">{{ t('fleet.loading') }}</p>
      <div v-else-if="!fleets.length" class="rounded-2xl border border-dashed border-gray-300 p-12 text-center dark:border-dark-600">
        <h2 class="text-lg font-medium">{{ t('fleet.empty') }}</h2><p class="my-3 text-gray-500">{{ t('fleet.emptyHint') }}</p><button class="btn btn-primary" @click="open('create')">{{ t('fleet.create') }}</button>
      </div>
      <div class="grid items-start gap-5 xl:grid-cols-2">
        <section v-for="fleet in fleets" :key="fleet.id" :data-fleet-id="fleet.id" class="overflow-hidden rounded-2xl border bg-white shadow-sm transition dark:bg-dark-800" :class="dragOver === fleet.id ? 'border-primary-500 ring-2 ring-primary-200' : 'border-gray-200 dark:border-dark-700'" @dragover.prevent="dragOver = fleet.id" @dragleave="dragOver = null" @drop.prevent="drop(fleet)">
          <div class="border-b border-gray-100 p-5 dark:border-dark-700">
            <div class="flex items-start justify-between gap-3"><div><h2 class="text-lg font-semibold">{{ fleet.name }}</h2><p class="mt-1 text-sm text-gray-500">{{ fleet.group_name }}</p></div><span class="badge" :class="isExpired(fleet.expires_at) ? 'badge-warning' : 'badge-success'">{{ isExpired(fleet.expires_at) ? t('fleet.expired') : t('fleet.active') }}</span></div>
            <div class="mt-4 space-y-3" :aria-label="t('fleet.fleetQuota')">
              <p v-if="usageLoading && !accountUsage[fleet.group_id]" class="text-xs text-gray-400">{{ t('fleet.loading') }}</p>
              <p v-else-if="!accountUsage[fleet.group_id]?.length" class="text-xs text-gray-400">{{ t('fleet.noAccount') }}</p>
              <div v-for="acc in accountUsage[fleet.group_id] || []" :key="acc.id" :data-account-id="acc.id" class="rounded-xl bg-gray-50 p-4 dark:bg-dark-700/50">
                <div class="flex flex-wrap items-end justify-between gap-3">
                  <div>
                    <p class="text-xs text-gray-500">{{ t('fleet.nextReset') }}</p>
                    <p class="text-xl font-semibold tabular-nums" data-next-reset>{{ acc.resets_at ? shortDate(acc.resets_at) : '—' }}</p>
                    <p v-if="acc.resets_at" class="text-xs font-medium text-primary-600 dark:text-primary-400">{{ relative(acc.resets_at) }}</p>
                  </div>
                  <div class="text-right">
                    <p class="text-xs text-gray-500">{{ t('fleet.weeklyRemaining') }}</p>
                    <p class="text-xl font-semibold tabular-nums">{{ acc.pct === null ? '—' : `${Math.round(acc.pct)}%` }}</p>
                  </div>
                </div>
                <div v-if="acc.pct !== null" class="mt-2 h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="Math.round(acc.pct)" :aria-label="`${t('fleet.fleetQuota')} ${acc.name}`"><div class="h-full rounded-full transition-all" :class="barClass(acc.pct)" :style="{ width: `${acc.pct}%` }"></div></div>
                <p class="mt-2 truncate text-xs text-gray-400" :title="acc.error || acc.name">{{ acc.name }}<span v-if="acc.error" class="ml-1 text-amber-600">· {{ acc.error }}</span></p>
              </div>
              <div class="flex flex-wrap items-center justify-between gap-2 text-xs text-gray-500"><span>{{ t('fleet.expiry') }} · <time>{{ date(fleet.expires_at) }}</time></span><span>{{ currentMembers(fleet).length }} {{ t('fleet.members') }}</span></div>
            </div>
            <div class="mt-4 flex flex-wrap gap-2">
              <button class="btn btn-primary btn-sm" @click="open('add', fleet)">{{ t('fleet.add') }}</button>
              <button class="btn btn-secondary btn-sm" @click="open('expiry', fleet)">{{ t('fleet.adjust') }}</button>
              <button class="btn btn-secondary btn-sm" @click="open('reset', fleet)">{{ t('fleet.reset') }}</button>
              <button class="btn btn-secondary btn-sm" @click="open('rename', fleet)">{{ t('fleet.rename') }}</button>
              <button v-if="!currentMembers(fleet).length" class="btn btn-secondary btn-sm" @click="open('archive', fleet)">{{ t('fleet.archive') }}</button>
            </div>
            <p v-if="isExpired(fleet.expires_at)" class="mt-3 text-xs text-amber-600">{{ t('fleet.expiredHint') }}</p>
          </div>
          <ul class="divide-y divide-gray-100 dark:divide-dark-700">
            <li v-for="member in currentMembers(fleet)" :key="member.user_id" class="p-5" @dragstart="startDrag($event, fleet, member)" @dragend="dragged = null; dragOver = null">
              <div class="flex items-start gap-3">
                <span :draggable="!busy" class="flex h-10 w-10 cursor-grab shrink-0 items-center justify-center rounded-full bg-primary-50 font-semibold text-primary-700 dark:bg-primary-900/30">{{ (member.username || member.email).slice(0, 1).toUpperCase() }}</span>
                <div class="min-w-0 flex-1"><RouterLink :draggable="false" :to="{ path: '/admin/usage', query: { user_id: member.user_id } }" class="break-all text-sm font-semibold hover:text-primary-600">{{ member.email }}</RouterLink><p class="mt-1 text-xs text-gray-500">{{ t('fleet.subscription', { id: member.subscription_id }) }} · {{ t('fleet.keys', { count: member.key_count }) }}</p></div>
                <button class="rounded p-1.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-700" :title="t('fleet.transfer')" :aria-label="`${t('fleet.transfer')}: ${member.email}`" @click="open('transfer', fleet, member)"><Icon name="arrowRight" size="sm" /></button>
                <button class="rounded p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-600" :title="t('fleet.remove')" :aria-label="`${t('fleet.remove')}: ${member.email}`" @click="open('remove', fleet, member)"><Icon name="x" size="sm" /></button>
              </div>
              <div class="mt-3 grid grid-cols-3 gap-3 text-xs">
                <div v-for="q in memberQuota(fleet, member)" :key="q.period"><p class="mb-1 text-gray-500">{{ t(`fleet.${q.period}`) }}</p><span class="font-medium">${{ money(usage(member, q.period)) }}</span><span class="text-gray-400"> / {{ limit(fleet, q.period) }}</span>
                  <div v-if="q.pct !== null" class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600" role="progressbar" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="Math.round(q.pct)" :aria-label="`${member.email} ${t(`fleet.${q.period}`)} ${t('fleet.remaining')}`" :title="`${t('fleet.remaining')} ${Math.round(q.pct)}%`"><div class="h-full rounded-full transition-all" :class="barClass(q.pct)" :style="{ width: `${q.pct}%` }"></div></div>
                </div>
              </div>
              <button class="mt-3 flex flex-wrap items-center gap-2 text-xs text-gray-500 hover:text-primary-600" @click="open('member_expiry', fleet, member)"><span :class="member.independent_expiry ? 'rounded bg-amber-50 px-2 py-1 text-amber-700 dark:bg-amber-900/30' : ''">{{ member.independent_expiry ? t('fleet.independent') : t('fleet.inherited') }}</span><span>{{ date(member.expires_at) }}</span><span v-if="isExpired(member.expires_at)" class="text-amber-600">{{ t('fleet.expired') }}</span></button>
            </li>
          </ul>
          <p v-if="!currentMembers(fleet).length" class="p-8 text-center text-sm text-gray-400">{{ t('fleet.noMembers') }}</p>
          <p v-if="fleet.members.some(m => !m.active)" class="border-t border-gray-100 px-5 py-3 text-xs text-gray-400 dark:border-dark-700">{{ t('fleet.history', { count: fleet.members.filter(m => !m.active).length }) }}</p>
        </section>
      </div>
      <p class="text-xs text-gray-400">{{ t('fleet.timezone') }}</p>
    </div>

    <BaseDialog :show="mode !== null" :title="dialogTitle" :width="mode === 'create' || mode === 'add' ? 'wide' : 'normal'" @close="close">
      <form id="fleet-form" class="space-y-4" @submit.prevent="submit">
        <p v-if="dialogError" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700">{{ dialogError }}</p>
        <p v-if="selectedFleet" class="font-medium">{{ selectedFleet.name }}<span v-if="selectedMember" class="ml-2 break-all text-sm text-gray-500">{{ selectedMember.email }}</span></p>
        <label v-if="mode === 'create' || mode === 'rename'" class="block text-sm">{{ t('fleet.name') }}<input v-model="name" required maxlength="100" class="input mt-1 w-full" /></label>
        <label v-if="mode === 'create'" class="block text-sm">{{ t('fleet.group') }}<select v-model="groupId" class="input mt-1 w-full" required @change="loadPreview"><option :value="0" disabled>{{ t('fleet.choose') }}</option><option v-for="group in availableGroups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label>
        <template v-if="mode === 'create' || mode === 'expiry'">
          <div v-if="mode === 'expiry'" class="flex flex-wrap gap-3 text-sm"><label><input v-model="dateMode" type="radio" value="days" /> {{ t('fleet.byDays') }}</label><label><input v-model="dateMode" type="radio" value="date" /> {{ t('fleet.byDate') }}</label><button type="button" class="text-primary-600" @click="dateMode = 'days'; days = 30">{{ t('fleet.extend30') }}</button></div>
          <label v-if="mode === 'expiry' && dateMode === 'days'" class="block text-sm">{{ t('fleet.days') }}<input v-model.number="days" required type="number" min="-36500" max="36500" class="input mt-1 w-full" /></label>
          <label v-else class="block text-sm">{{ t('fleet.expiry') }}<input v-model="expiry" required type="datetime-local" step="1" class="input mt-1 w-full" /></label>
          <p class="text-xs text-gray-500">{{ t('fleet.timezone') }}</p><p v-if="mode === 'expiry'" class="text-sm text-gray-500">{{ t('fleet.expiryHint') }}</p>
        </template>
        <template v-if="mode === 'create' || mode === 'add'">
          <div v-if="mode === 'add'"><label class="block text-sm">{{ t('fleet.search') }}<input v-model="search" class="input mt-1 w-full" @input="searchUsers" /></label><div v-if="searchResults.length" class="mt-2 max-h-40 overflow-auto rounded-lg border dark:border-dark-600"><button v-for="user in searchResults" :key="user.id" type="button" class="block w-full p-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-700" @click="selectUser(user)">{{ user.email }}</button></div></div>
          <div v-if="candidates.length" class="space-y-3"><h3 class="font-medium">{{ t('fleet.preview') }}</h3><p class="text-sm text-gray-500">{{ t('fleet.previewHint') }}</p>
            <div v-for="candidate in candidates" :key="candidate.user_id" class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
              <label class="flex items-center gap-2 text-sm"><input v-model="candidate.selected" type="checkbox" :disabled="candidate.blocked" /><span class="break-all">{{ candidate.email }}</span></label>
              <div v-if="candidate.selected" class="mt-2 space-y-2 pl-5 text-xs"><p v-if="candidate.expires_at">{{ t('fleet.oldExpiry') }}: {{ date(candidate.expires_at) }}</p><label class="flex items-center gap-2"><input v-model="candidate.independent_expiry" type="checkbox" />{{ t('fleet.independent') }}</label><input v-if="candidate.independent_expiry && !candidate.subscription_id" v-model="candidate.newExpiry" type="datetime-local" class="input w-full" required /><p>{{ t('fleet.newExpiry') }}: {{ candidate.independent_expiry ? (candidate.expires_at ? date(candidate.expires_at) : candidate.newExpiry) : (mode === 'create' ? expiry.replace('T', ' ') : date(selectedFleet!.expires_at)) }}</p><span v-if="candidate.subscription_id" class="text-amber-600">{{ t('fleet.existing') }} · #{{ candidate.subscription_id }}</span></div>
            </div>
          </div>
        </template>
        <template v-if="mode === 'transfer'"><p class="text-sm text-gray-500">{{ t('fleet.transferHint') }}</p><label class="block text-sm">{{ t('fleet.target') }}<select v-model="targetId" class="input mt-1 w-full" required @change="loadTarget"><option :value="0" disabled>{{ t('fleet.choose') }}</option><option v-for="fleet in targetFleets" :key="fleet.id" :value="fleet.id">{{ fleet.name }} · {{ date(fleet.expires_at) }}</option></select></label><p v-if="!targetFleets.length" class="text-sm text-amber-600">{{ t('fleet.noTarget') }}</p><label v-if="targetExisting && !targetExisting.owner_fleet_id" class="flex items-center gap-2 text-sm"><input v-model="adoptTarget" type="checkbox" required />{{ t('fleet.existing') }} #{{ targetExisting.subscription_id }} · {{ date(targetExisting.expires_at) }}</label></template>
        <template v-if="mode === 'member_expiry'"><label class="flex items-center gap-2 text-sm"><input v-model="independent" type="checkbox" />{{ t('fleet.independent') }}</label><input v-if="independent" v-model="expiry" type="datetime-local" step="1" required class="input w-full" /><p class="text-sm text-gray-500">{{ t('fleet.independentHint') }}</p><p class="text-xs text-gray-400">{{ t('fleet.timezone') }}</p></template>
        <template v-if="mode === 'reset' || mode === 'reset_all'"><p class="text-sm text-gray-500">{{ t('fleet.resetHint') }}</p><div class="flex gap-5"><label v-for="period in periods" :key="period" class="flex items-center gap-2 text-sm"><input v-model="resetPeriods[period]" type="checkbox" />{{ t(`fleet.${period}`) }}</label></div></template>
        <p v-if="mode === 'remove'" class="text-sm text-gray-500">{{ t('fleet.removeHint') }}</p>
        <p v-if="mode === 'archive'" class="text-sm text-gray-500">{{ t('fleet.archiveHint') }}</p>
      </form>
      <template #footer><button class="btn btn-secondary" :disabled="busy" @click="close">{{ t('fleet.cancel') }}</button><button class="btn btn-primary" type="submit" form="fleet-form" :disabled="busy || previewLoading">{{ busy ? t('fleet.loading') : t('fleet.save') }}</button></template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import * as api from '@/api/admin/fleets'
import * as groupsAPI from '@/api/admin/groups'
import * as usersAPI from '@/api/admin/users'
import * as accountsAPI from '@/api/admin/accounts'
import type { AdminGroup, AdminUser } from '@/types'
import type { Fleet, FleetMember, FleetAction, FleetMemberInput } from '@/api/admin/fleets'

type Period = 'daily' | 'weekly' | 'monthly'
type AccountWeekly = { id: number; name: string; pct: number | null; resets_at: string | null; error?: string }
type Candidate = Partial<FleetMember> & { user_id: number; email: string; selected: boolean; independent_expiry: boolean; newExpiry: string; blocked?: boolean }
const { t } = useI18n()
const app = useAppStore()
const fleets = ref<Fleet[]>([]), groups = ref<AdminGroup[]>([])
const accountUsage = ref<Record<number, AccountWeekly[]>>({}), usageLoading = ref(false)
let usageSequence = 0
const loading = ref(false), busy = ref(false), previewLoading = ref(false)
const error = ref(''), dialogError = ref('')
const mode = ref<FleetAction['action'] | null>(null)
const selectedFleet = ref<Fleet>(), selectedMember = ref<FleetMember>()
const name = ref(''), groupId = ref(0), expiry = ref(''), dateMode = ref('days'), days = ref(30)
const independent = ref(false), candidates = ref<Candidate[]>([])
const search = ref(''), searchResults = ref<AdminUser[]>([])
const targetId = ref(0), targetExisting = ref<FleetMember>(), adoptTarget = ref(false)
const resetPeriods = ref<Record<Period, boolean>>({ daily: true, weekly: true, monthly: true })
const dragged = ref<{ fleet: Fleet; member: FleetMember } | null>(null), dragOver = ref<number | null>(null)
const periods: Period[] = ['daily', 'weekly', 'monthly']
let searchTimer: ReturnType<typeof setTimeout> | undefined
let searchSequence = 0, previewSequence = 0
let operationKey = '', operationBody = ''
const availableGroups = computed(() => groups.value.filter(g => g.subscription_type === 'subscription' && g.status === 'active' && !fleets.value.some(f => f.group_id === g.id)))
const targetFleets = computed(() => fleets.value.filter(f => f.id !== selectedFleet.value?.id && f.platform === selectedFleet.value?.platform))
const memberCount = computed(() => new Set(fleets.value.flatMap(f => currentMembers(f).map(m => m.user_id))).size)
const dialogTitle = computed(() => t(`fleet.${({ create: 'create', add: 'add', remove: 'remove', transfer: 'transfer', expiry: 'adjust', member_expiry: 'memberExpiry', reset: 'reset', reset_all: 'resetAll', rename: 'rename', archive: 'archive' } as Record<string, string>)[mode.value || 'create']}`))
function currentMembers(f: Fleet) { return f.members.filter(m => m.active) }
function isExpired(value: string) { return new Date(value).getTime() <= Date.now() }
function date(value: string) { return new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Singapore', dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value)) }
function inputDate(value: string) { return date(value).replace(' ', 'T') }
function toDate(value: string) { const parsed = new Date(`${value.length === 16 ? value + ':00' : value}+08:00`); if (!Number.isFinite(parsed.getTime())) throw new Error(t('fleet.expiryRequired')); return parsed.toISOString() }
function money(value: number) { return value.toFixed(2) }
function usage(m: FleetMember, p: Period) { return m[`${p}_usage_usd`] }
function limit(f: Fleet, p: Period) { const value = f[`${p}_limit_usd`]; return value == null || value <= 0 ? '∞' : `$${money(value)}` }
function limitValue(f: Fleet, p: Period) { const value = f[`${p}_limit_usd`]; return value == null || value <= 0 ? 0 : value }
function remainingPct(left: number, max: number) { return max > 0 ? Math.min(Math.max(left / max * 100, 0), 100) : null }
function memberQuota(f: Fleet, m: FleetMember) { return periods.map(period => { const max = limitValue(f, period); return { period, pct: remainingPct(max - (usage(m, period) || 0), max) } }) }
function shortDate(value: string) { return new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Singapore', dateStyle: 'short', timeStyle: 'short' }).format(new Date(value)) }
function relative(value: string) {
  const minutes = Math.max(Math.round((new Date(value).getTime() - Date.now()) / 60000), 0)
  const d = Math.floor(minutes / 1440), h = Math.floor(minutes % 1440 / 60), m = minutes % 60
  return d > 0 ? t('fleet.resetInDays', { d, h }) : t('fleet.resetInHours', { h, m })
}
async function loadAccountUsage() {
  const seq = ++usageSequence
  usageLoading.value = true
  try {
    const groupIds = [...new Set(fleets.value.map(f => f.group_id))]
    const lists = await Promise.all(groupIds.map(async id => [id, (await accountsAPI.list(1, 100, { group: String(id), lite: '1' })).items] as const))
    const ids = [...new Set(lists.flatMap(([, items]) => items.map(a => a.id)))]
    const batch = ids.length ? await accountsAPI.getBatchUsage(ids) : { usage: {}, errors: {} }
    if (seq !== usageSequence) return
    accountUsage.value = Object.fromEntries(lists.map(([gid, items]) => [gid, items.map(a => {
      const week = batch.usage[String(a.id)]?.seven_day
      return { id: a.id, name: a.name, pct: week ? Math.min(Math.max(100 - week.utilization, 0), 100) : null, resets_at: week?.resets_at || null, error: batch.errors[String(a.id)] }
    })]))
  } catch (e) { if (seq === usageSequence) error.value = errorText(e) } finally { if (seq === usageSequence) usageLoading.value = false }
}
function barClass(pct: number) { return pct < 10 ? 'bg-red-500' : pct < 30 ? 'bg-orange-500' : 'bg-green-500' }
function errorText(e: unknown) { const err = e as { response?: { data?: { message?: string } }; message?: string }; return err.response?.data?.message || err.message || t('fleet.failed') }
async function load() { loading.value = true; error.value = ''; try { [fleets.value, groups.value] = await Promise.all([api.list(), groupsAPI.getAll()]); void loadAccountUsage() } catch (e) { error.value = errorText(e) } finally { loading.value = false } }
function close() { if (!busy.value) { mode.value = null; ++previewSequence; ++searchSequence } }
function open(action: FleetAction['action'], f?: Fleet, m?: FleetMember) {
  if (busy.value) return
  mode.value = action; selectedFleet.value = f; selectedMember.value = m; dialogError.value = ''
  name.value = f?.name || ''; groupId.value = f?.group_id || 0; days.value = 30; dateMode.value = 'days'
  expiry.value = inputDate(m?.expires_at || f?.expires_at || new Date(Date.now() + 30 * 86400000).toISOString())
  independent.value = m?.independent_expiry || false; candidates.value = []; search.value = ''; searchResults.value = []
  targetId.value = 0; targetExisting.value = undefined; adoptTarget.value = false
  resetPeriods.value = { daily: true, weekly: true, monthly: true }; operationKey = ''; operationBody = ''
  if (action === 'add') void loadPreview()
}
async function loadPreview() {
  const seq = ++previewSequence
  if (!groupId.value) return
  previewLoading.value = true
  try { const rows = await api.preview(groupId.value); if (seq !== previewSequence) return; candidates.value = rows.filter(m => mode.value === 'create' || !selectedFleet.value?.members.some(existing => existing.user_id === m.user_id && existing.active)).map(m => ({ ...m, selected: false, newExpiry: inputDate(m.expires_at), blocked: !!m.owner_fleet_id && m.owner_fleet_id !== selectedFleet.value?.id })) } catch (e) { if (seq === previewSequence) dialogError.value = errorText(e) } finally { if (seq === previewSequence) previewLoading.value = false }
}
function searchUsers() { clearTimeout(searchTimer); const seq = ++searchSequence; searchTimer = setTimeout(async () => { try { const result = await usersAPI.list(1, 20, { search: search.value, status: 'active' }); if (seq === searchSequence) searchResults.value = result.items.filter(u => !selectedFleet.value?.members.some(m => m.active && m.user_id === u.id)) } catch (e) { dialogError.value = errorText(e) } }, 250) }
function selectUser(user: AdminUser) { const existing = candidates.value.find(c => c.user_id === user.id); if (existing) { if (!existing.blocked) existing.selected = true } else candidates.value.push({ user_id: user.id, email: user.email, independent_expiry: false, selected: true, newExpiry: expiry.value }); searchResults.value = []; search.value = ''; ++searchSequence }
async function loadTarget() { targetExisting.value = undefined; adoptTarget.value = false; const target = fleets.value.find(f => f.id === targetId.value); if (!target || !selectedMember.value) return; previewLoading.value = true; try { targetExisting.value = (await api.preview(target.group_id)).find(m => m.user_id === selectedMember.value!.user_id) } catch (e) { dialogError.value = errorText(e) } finally { previewLoading.value = false } }
function startDrag(event: DragEvent, fleet: Fleet, member: FleetMember) { dragged.value = { fleet, member }; event.dataTransfer?.setData('text/plain', String(member.user_id)); if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move' }
function drop(target: Fleet) { const source = dragged.value; dragOver.value = null; if (!source || source.fleet.id === target.id || source.fleet.platform !== target.platform) return; open('transfer', source.fleet, source.member); targetId.value = target.id; void loadTarget(); dragged.value = null }
function memberInput(c: Candidate): FleetMemberInput { return { user_id: c.user_id, independent_expiry: c.independent_expiry, adopt_existing: !!c.subscription_id, ...(c.independent_expiry && !c.subscription_id ? { expires_at: toDate(c.newExpiry) } : {}) } }
async function submit() {
  if (!mode.value || busy.value) return
  dialogError.value = ''
  try {
    const action = mode.value
    const input: FleetAction = { action, versions: Object.fromEntries(fleets.value.map(f => [f.id, f.version])) }
    if (selectedFleet.value) input.fleet_id = selectedFleet.value.id
    if (action === 'create') { input.name = name.value; input.group_id = groupId.value; input.expires_at = toDate(expiry.value); input.members = candidates.value.filter(c => c.selected && !c.blocked).map(memberInput) }
    if (action === 'rename') input.name = name.value
    if (action === 'add') { input.members = candidates.value.filter(c => c.selected && !c.blocked).map(memberInput); if (!input.members.length) throw new Error(t('fleet.noSelection')) }
    if (action === 'remove' || action === 'transfer') input.user_id = selectedMember.value!.user_id
    if (action === 'transfer') { if (!targetId.value) throw new Error(t('fleet.choose')); input.target_fleet_id = targetId.value; input.members = [{ user_id: input.user_id!, independent_expiry: selectedMember.value!.independent_expiry, expires_at: selectedMember.value!.expires_at, adopt_existing: adoptTarget.value }] }
    if (action === 'member_expiry') input.members = [{ user_id: selectedMember.value!.user_id, independent_expiry: independent.value, ...(independent.value ? { expires_at: toDate(expiry.value) } : {}), adopt_existing: false }]
    if (action === 'expiry') { if (dateMode.value === 'days') input.days = days.value; else input.expires_at = toDate(expiry.value) }
    if (action === 'reset' || action === 'reset_all') Object.assign(input, resetPeriods.value)
    const serialized = JSON.stringify(input)
    if (serialized !== operationBody) { operationBody = serialized; operationKey = crypto.randomUUID() }
    busy.value = true
    const result = await api.mutate(input, operationKey)
    if (result.cache_pending) app.showInfo(t('fleet.pending')); else app.showSuccess(t('fleet.success'))
    mode.value = null; await load()
  } catch (e) { dialogError.value = errorText(e) } finally { busy.value = false }
}
onMounted(load)
onBeforeUnmount(() => { clearTimeout(searchTimer); ++searchSequence; ++previewSequence; ++usageSequence })
</script>
