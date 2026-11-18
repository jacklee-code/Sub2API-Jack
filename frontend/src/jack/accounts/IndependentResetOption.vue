<template>
  <label class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
    <input
      type="checkbox"
      data-testid="independent-reset"
      :checked="modelValue"
      :disabled="locked"
      class="mt-1 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
      @change="emit('update:modelValue', ($event.target as HTMLInputElement).checked)"
    >
    <span>
      <span class="font-medium">{{ t('fleet.independentReset') }}</span>
      <span class="mt-1 block text-xs text-gray-500">{{ t('fleet.independentResetHint') }}</span>
      <span v-if="locked" class="mt-1 block text-xs text-amber-600">{{ t('fleet.creditRetryModeLocked') }}</span>
    </span>
  </label>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{ modelValue: boolean; locked: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
const { t } = useI18n()
</script>
