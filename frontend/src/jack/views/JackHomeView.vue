<template>
  <!--
    Jack replacement for src/views/HomeView.vue (swapped in by the jack-theme Vite
    plugin). Administrator-defined home content and the compact home page are
    still rendered by the upstream view.
  -->
  <HomeView v-if="useUpstreamHome" />

  <div v-else data-testid="jack-home" class="relative flex min-h-screen flex-col overflow-x-hidden">
    <header class="glass sticky top-0 z-20 border-b border-gray-900/5 dark:border-white/5">
      <nav class="mx-auto flex max-w-6xl items-center justify-between gap-3 px-4 py-3 sm:px-6">
        <div class="flex min-w-0 items-center gap-3">
          <img :src="siteLogo || '/logo.svg'" alt="Logo" class="jack-brand-mark h-9 w-9 shrink-0 object-contain" />
          <span class="truncate font-semibold text-gray-900 dark:text-white">{{ siteName }}</span>
        </div>

        <div class="flex shrink-0 items-center gap-1 sm:gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-ghost btn-sm"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="sm" />
            <span class="hidden sm:inline">{{ t('home.docs') }}</span>
          </a>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="btn btn-ghost btn-sm"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="sm" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <button
            type="button"
            class="btn btn-ghost btn-icon"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="sm" />
            <Icon v-else name="moon" size="sm" />
          </button>
          <router-link :to="primaryLink" class="btn btn-primary btn-sm">
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="relative flex-1">
      <section class="relative mx-auto grid max-w-6xl grid-cols-1 items-center gap-14 px-4 pb-16 pt-16 sm:px-6 sm:pt-24 lg:grid-cols-[1.05fr_1fr]">
        <div class="jack-halo -left-40 -top-40" aria-hidden="true"></div>

        <div class="relative min-w-0">
          <p class="jack-eyebrow">{{ tagline }}</p>
          <h1 class="jack-display text-gradient mt-5 text-5xl leading-[1.04] sm:text-6xl">
            {{ t('home.heroSubtitle') }}
          </h1>
          <p class="mt-6 max-w-lg whitespace-pre-wrap text-base leading-7 text-gray-600 [overflow-wrap:anywhere] dark:text-dark-300">
            {{ siteSubtitle || t('home.heroDescription') }}
          </p>
          <div class="mt-8 flex flex-wrap items-center gap-3">
            <router-link :to="primaryLink" class="btn btn-primary btn-lg">
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              <Icon name="arrowRight" size="sm" />
            </router-link>
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="btn btn-secondary btn-lg"
            >
              {{ t('home.viewDocs') }}
            </a>
          </div>
        </div>

        <div class="relative min-w-0">
          <div class="jack-window">
            <div class="jack-window-bar" aria-hidden="true">
              <i></i><i></i><i></i>
              <span class="ml-3 font-mono text-xs text-white/40">zsh</span>
            </div>
            <pre class="overflow-x-auto px-5 pb-6 pt-1 font-mono text-[13px] leading-6 text-white/85"><span class="text-white/35"># Claude Code</span>
<span class="text-white/45">$</span> export ANTHROPIC_BASE_URL={{ apiBaseUrl }}
<span class="text-white/45">$</span> export ANTHROPIC_AUTH_TOKEN=sk-••••
<span class="text-white/45">$</span> claude

<span class="text-white/35"># OpenAI compatible</span>
<span class="text-white/45">$</span> curl {{ apiBaseUrl }}/v1/chat/completions \
    -H "Authorization: Bearer sk-••••"</pre>
          </div>
        </div>
      </section>

      <section class="mx-auto max-w-6xl px-4 sm:px-6">
        <div class="flex flex-wrap items-center gap-x-8 gap-y-3 border-y border-gray-900/10 py-5 text-sm text-gray-500 dark:border-white/10 dark:text-dark-400">
          <span class="jack-eyebrow">{{ t('home.providers.supported') }}</span>
          <span>{{ t('home.providers.claude') }}</span>
          <span>{{ t('home.providers.gemini') }}</span>
          <span>{{ t('home.providers.antigravity') }}</span>
          <span>{{ t('home.providers.more') }}</span>
        </div>
      </section>

      <section class="mx-auto grid max-w-6xl grid-cols-1 gap-5 px-4 pb-20 pt-10 sm:px-6 md:grid-cols-3">
        <div v-for="feature in features" :key="feature.title" class="card p-7">
          <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-primary-500/10 text-primary-600 dark:text-primary-300">
            <Icon :name="feature.icon" size="md" />
          </div>
          <h2 class="mt-5 text-lg font-semibold tracking-tight text-gray-900 dark:text-white">{{ feature.title }}</h2>
          <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ feature.description }}</p>
        </div>
      </section>
    </main>

    <footer class="border-t border-gray-900/5 px-4 py-6 text-sm text-gray-500 dark:border-white/5 dark:text-dark-400 sm:px-6">
      <div class="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3">
        <span>&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</span>
        <a :href="githubUrl" target="_blank" rel="noopener noreferrer" class="hover:text-gray-900 dark:hover:text-white">
          Powered by Sub2API
        </a>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import HomeView from '@/views/HomeView.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const useUpstreamHome = computed(() => homeContent.value.trim().length > 0 || compactHomeEnabled.value)

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true })
)
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || '')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const apiBaseUrl = computed(() => window.location.origin)
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

const isAuthenticated = computed(() => authStore.isAuthenticated)
const primaryLink = computed(() => {
  if (!isAuthenticated.value) return '/login'
  return authStore.isAdmin ? '/admin/dashboard' : '/dashboard'
})
const showModelPlazaEntry = computed(
  () =>
    isFeatureFlagEnabled(FeatureFlags.modelPlaza) &&
    (isAuthenticated.value || appStore.cachedPublicSettings?.model_plaza_require_auth !== true)
)

const tagline = computed(() =>
  [t('home.tags.subscriptionToApi'), t('home.tags.stickySession'), t('home.tags.realtimeBilling')].join(' · ')
)
const features = computed(() => [
  { icon: 'swap' as const, title: t('home.features.unifiedGateway'), description: t('home.features.unifiedGatewayDesc') },
  { icon: 'shield' as const, title: t('home.features.multiAccount'), description: t('home.features.multiAccountDesc') },
  { icon: 'chart' as const, title: t('home.features.balanceQuota'), description: t('home.features.balanceQuotaDesc') }
])

const currentYear = computed(() => new Date().getFullYear())

const isDark = ref(document.documentElement.classList.contains('dark'))

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

onMounted(() => {
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>
