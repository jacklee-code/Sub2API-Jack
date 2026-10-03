<template>
  <!--
    Jack replacement for src/components/layout/AuthLayout.vue (swapped in by the
    jack-theme Vite plugin). Keep the same slots and settings behaviour as the
    upstream layout; only the visual structure differs.
  -->
  <div class="relative grid min-h-screen grid-cols-1 overflow-hidden lg:grid-cols-[1.1fr_1fr]">
    <aside class="relative hidden flex-col justify-between p-12 lg:flex">
      <div class="flex min-h-10 items-center gap-3">
        <template v-if="settingsLoaded">
          <img :src="siteLogo || '/logo.svg'" alt="Logo" class="jack-brand-mark h-10 w-10 object-contain" />
          <span class="text-lg font-semibold text-gray-900 dark:text-white">{{ siteName }}</span>
        </template>
      </div>

      <div class="max-w-lg">
        <p class="jack-eyebrow">{{ tagline }}</p>
        <h2 class="jack-display text-gradient mt-4 text-5xl leading-[1.05]">
          {{ t('home.heroSubtitle') }}
        </h2>
        <p
          v-if="settingsLoaded"
          class="mt-5 max-w-md whitespace-pre-wrap text-[15px] leading-7 text-gray-600 dark:text-dark-300"
        >
          {{ siteSubtitle }}
        </p>
        <ul class="mt-10 grid max-w-md gap-5 border-t border-gray-900/10 pt-6 dark:border-white/10">
          <li v-for="feature in features" :key="feature.title">
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ feature.title }}</p>
            <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-dark-400">{{ feature.description }}</p>
          </li>
        </ul>
      </div>

      <p class="text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </p>
    </aside>

    <main class="relative flex items-center justify-center p-4 sm:p-6">
      <div class="jack-halo" aria-hidden="true"></div>

      <div class="relative z-10 w-full max-w-md">
        <div v-if="settingsLoaded" class="mb-8 flex items-center gap-3 lg:hidden">
          <img :src="siteLogo || '/logo.svg'" alt="Logo" class="jack-brand-mark h-10 w-10 object-contain" />
          <div class="min-w-0">
            <p class="truncate text-lg font-semibold text-gray-900 dark:text-white">{{ siteName }}</p>
            <p class="truncate text-xs text-gray-500 dark:text-dark-400">{{ siteSubtitle }}</p>
          </div>
        </div>

        <div class="card-glass p-8">
          <slot />
        </div>

        <div class="mt-6 text-center text-sm">
          <slot name="footer" />
        </div>

        <p class="mt-8 text-center text-xs text-gray-400 dark:text-dark-500 lg:hidden">
          &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
        </p>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const tagline = computed(() =>
  [t('home.tags.subscriptionToApi'), t('home.tags.stickySession'), t('home.tags.realtimeBilling')].join(' · ')
)

const features = computed(() => [
  { title: t('home.features.unifiedGateway'), description: t('home.features.unifiedGatewayDesc') },
  { title: t('home.features.multiAccount'), description: t('home.features.multiAccountDesc') },
  { title: t('home.features.balanceQuota'), description: t('home.features.balanceQuotaDesc') }
])

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>
