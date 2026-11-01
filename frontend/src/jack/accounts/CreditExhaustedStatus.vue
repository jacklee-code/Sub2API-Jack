<template>
  <div class="flex flex-col items-center gap-1" :title="text.detail">
    <span class="badge badge-danger text-xs">{{ text.label }}</span>
    <span class="text-[11px] text-gray-400 dark:text-gray-500">{{ text.hint }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { locale } = useI18n()
const messages = {
  en: { label: 'Credit exhausted', hint: 'Restore status manually', detail: 'OpenAI confirmed that API credits are exhausted. This account is skipped until you select Restore status.' },
  zh: { label: '额度耗尽', hint: '需手动恢复状态', detail: 'OpenAI 已确认 API Credit 用尽。后续请求会跳过此账号，直至你点击「恢复状态」。' },
  'zh-Hant': { label: '額度耗盡', hint: '需手動恢復狀態', detail: 'OpenAI 已確認 API Credit 用盡。後續請求會跳過此帳號，直至你點擊「恢復狀態」。' },
}
const text = computed(() => messages[locale.value as keyof typeof messages] ?? messages.en)
</script>
