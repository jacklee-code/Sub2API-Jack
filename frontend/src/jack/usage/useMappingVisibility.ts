import { computed, ref, watch, type Ref } from 'vue'
import type { AdminUsageLog } from '@/types'

const STORAGE_KEY = 'jack-usage-show-mappings'

// Presentation only. Keep original admin records for exports, billing and audit.
export function useMappingVisibility(records: Ref<AdminUsageLog[]>) {
  const showMappings = ref(false)
  try {
    showMappings.value = localStorage.getItem(STORAGE_KEY) === 'true'
  } catch { /* Storage may be unavailable; the default stays private. */ }

  watch(showMappings, (value) => {
    try { localStorage.setItem(STORAGE_KEY, String(value)) } catch { /* Optional preference. */ }
  })

  const displayRecords = computed(() => showMappings.value ? records.value : records.value.map((record) => ({
    ...record,
    upstream_model: undefined,
    upstream_reasoning_effort: undefined,
    upstream_response_model: undefined,
    upstream_model_mismatch: undefined,
    model_mapping_chain: undefined,
  })))

  return { showMappings, displayRecords }
}
