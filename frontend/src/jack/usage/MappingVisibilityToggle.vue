<template>
  <button
    type="button"
    role="menuitemcheckbox"
    :aria-checked="modelValue"
    data-testid="usage-show-mappings"
    class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
    @click="$emit('update:modelValue', !modelValue)"
  >
    <span>{{ label }}</span>
    <Icon v-if="modelValue" name="check" size="sm" class="text-primary-500" :stroke-width="2" />
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ modelValue: boolean }>()
defineEmits<{ 'update:modelValue': [value: boolean] }>()
const { locale } = useI18n()
const label = computed(() => locale.value === 'zh-Hant' ? '顯示映射' : locale.value.startsWith('zh') ? '显示映射' : 'Show mappings')
</script>
