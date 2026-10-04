<template>
  <!--
    Jack replacement for src/views/HomeView.vue (swapped in by the jack-theme Vite
    plugin). Administrator-defined home content and the compact home page are
    still rendered by the upstream view.
  -->
  <HomeView v-if="useUpstreamHome" />

  <div v-else data-testid="jack-home" class="jh relative flex min-h-screen flex-col overflow-x-clip">
    <header class="glass sticky top-0 z-30 border-b border-gray-900/5 dark:border-white/5">
      <nav class="mx-auto flex max-w-6xl items-center justify-between gap-3 px-4 py-3 sm:px-6">
        <router-link to="/home" class="flex min-w-0 items-center gap-3">
          <img :src="siteLogo || '/logo.svg'" alt="Logo" class="jack-brand-mark h-9 w-9 shrink-0 object-contain" />
          <span class="truncate font-semibold text-gray-900 dark:text-white">{{ siteName }}</span>
        </router-link>

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
      <!-- Hero: paper, ink and the routing diagram -->
      <section class="jh-hero-stage">
        <div class="jh-paper" aria-hidden="true"></div>
        <InkCanvas />

        <div class="jh-hero mx-auto max-w-6xl px-4 sm:px-6">
          <div class="relative min-w-0">
            <p class="jack-eyebrow jh-rise" style="--d: 0ms">{{ th('jackHome.eyebrow') }}</p>
            <h1 class="jh-title jack-display mt-5">
              <span class="jh-title-lead">
                <span v-for="(char, i) in titleLead" :key="`l${i}`" class="jh-char" :style="{ '--i': i }">{{ char }}</span>
              </span>
              <br />
              <span class="jh-title-tail">
                <span
                  v-for="(char, i) in titleTail"
                  :key="`t${i}`"
                  class="jh-char"
                  :style="{ '--i': i + titleLead.length }"
                >{{ char }}</span>
              </span>
            </h1>
            <p class="jh-lead jh-rise mt-6 max-w-lg whitespace-pre-wrap [overflow-wrap:anywhere]" style="--d: 420ms">
              {{ siteSubtitle || th('jackHome.lead') }}
            </p>
            <div class="jh-rise mt-8 flex flex-wrap items-center gap-3" style="--d: 520ms">
              <router-link :to="heroCta.to" class="btn btn-primary btn-lg jh-arrow">
                {{ heroCta.label }}
                <Icon name="arrowRight" size="sm" />
              </router-link>
              <a href="#jack-home-start" class="btn btn-secondary btn-lg" @click.prevent="scrollToStart">
                {{ th('jackHome.ctaGuide') }}
              </a>
            </div>
            <div class="jh-rise jh-protocols mt-9" style="--d: 620ms">
              <span class="jh-live-dot" aria-hidden="true"></span>
              <span class="jh-protocols-label">{{ th('jackHome.protocolsLabel') }}</span>
              <span>Anthropic Messages</span>
              <span>OpenAI Chat / Responses</span>
              <span>Gemini</span>
            </div>
          </div>

          <figure class="jh-diagram jh-rise" style="--d: 260ms" :aria-label="th('jackHome.diagram.label')">
            <figcaption class="jh-diagram-cap">
              <span>route://{{ apiHost }}</span>
              <span>{{ th('jackHome.diagram.hint') }}</span>
            </figcaption>
            <svg class="jh-svg" viewBox="0 0 560 440" aria-hidden="true">
              <path class="jh-wire" d="M152 220H250" />
              <path class="jh-flow" d="M152 220H250" />
              <g v-for="node in diagramNodes" :key="node.platform" :class="{ 'is-active': activePlatform === node.platform, 'is-dim': activePlatform && activePlatform !== node.platform }">
                <path class="jh-wire" :d="node.path" />
                <path class="jh-flow" :d="node.path" :style="{ animationDuration: node.speed }" />
                <circle class="jh-pulse" r="2.8">
                  <animateMotion :dur="node.pulse" :begin="node.begin" repeatCount="indefinite" :path="node.path" />
                </circle>
              </g>
              <circle class="jh-pulse" r="3">
                <animateMotion dur="2.6s" repeatCount="indefinite" path="M152 220H250" />
              </circle>

              <text class="jh-sub" x="22" y="176">{{ th('jackHome.diagram.yourKey') }}</text>
              <rect class="jh-key" x="20" y="194" width="132" height="52" rx="12" />
              <text class="jh-key-text" x="40" y="225">sk-••••••</text>

              <circle class="jh-ring" cx="290" cy="220" r="58" />
              <circle class="jh-hub" cx="290" cy="220" r="40" />
              <text class="jh-hub-text" x="290" y="227" text-anchor="middle">{{ th('jackHome.diagram.gateway') }}</text>
              <text class="jh-sub" x="290" y="300" text-anchor="middle">GATEWAY</text>
              <g transform="rotate(-9 330 166)">
                <rect class="jh-seal" x="312" y="148" width="36" height="36" rx="4" />
                <rect x="315" y="151" width="30" height="30" rx="2.5" fill="none" stroke="#fff8f2" stroke-opacity=".75" />
                <text class="jh-seal-text" x="330" y="173" text-anchor="middle">通</text>
              </g>

              <g
                v-for="node in diagramNodes"
                :key="`n-${node.platform}`"
                class="jh-node"
                :class="{ 'is-active': activePlatform === node.platform }"
                @mouseenter="activePlatform = node.platform"
                @mouseleave="activePlatform = pinnedPlatform"
              >
                <rect class="jh-node-box" x="420" :y="node.y - 20" width="128" height="40" rx="10" />
                <rect x="436" :y="node.y - 4" width="8" height="8" rx="2" :fill="node.ink" />
                <text class="jh-node-label" x="454" :y="node.count ? node.y - 1 : node.y + 5">{{ node.label }}</text>
                <text v-if="node.count" class="jh-node-count" x="454" :y="node.y + 12">
                  {{ th('jackHome.diagram.models', { n: node.count }) }}
                </text>
              </g>
              <g v-if="extraPlatforms > 0">
                <path class="jh-wire" :d="extraNode.path" />
                <rect class="jh-node-box jh-node-more" x="420" :y="extraNode.y - 20" width="128" height="40" rx="10" />
                <text class="jh-sub" x="440" :y="extraNode.y + 4">{{ th('jackHome.diagram.more', { n: extraPlatforms }) }}</text>
              </g>
            </svg>
          </figure>
        </div>

        <div class="mx-auto max-w-6xl px-4 sm:px-6">
          <div class="jh-marquee" :aria-label="t('home.providers.supported')">
            <div class="jh-track">
              <span v-for="copy in 2" :key="copy" :aria-hidden="copy === 2 ? 'true' : undefined">
                <template v-for="item in marqueeItems" :key="`${copy}-${item}`">{{ item }}<i></i></template>
              </span>
            </div>
          </div>
        </div>
      </section>

      <!-- Three steps -->
      <section id="jack-home-start" v-reveal class="jh-section mx-auto max-w-6xl px-4 sm:px-6">
        <div class="jh-section-head">
          <div>
            <p class="jack-eyebrow">{{ th('jackHome.steps.eyebrow') }}</p>
            <h2 class="jh-h2 jack-display mt-3">{{ th('jackHome.steps.title') }}</h2>
          </div>
          <p class="jh-lead jh-section-lead">{{ th('jackHome.steps.lead') }}</p>
        </div>
        <ol class="jh-steps">
          <li v-for="(step, i) in steps" :key="step.key" class="jh-step" :style="{ '--i': i }">
            <div class="jh-step-n">{{ step.numeral }}<small>{{ step.tag }}</small></div>
            <h3>{{ th(`jackHome.steps.${step.key}.title`) }}</h3>
            <p>{{ th(`jackHome.steps.${step.key}.desc`) }}</p>
            <router-link v-if="step.link" :to="step.link.to" class="jh-step-link">
              {{ step.link.label }} <Icon name="arrowRight" size="xs" />
            </router-link>
          </li>
        </ol>
      </section>

      <!-- Code -->
      <section v-reveal class="jh-section mx-auto max-w-6xl px-4 sm:px-6">
        <div class="jh-code">
          <div>
            <p class="jack-eyebrow">{{ th('jackHome.code.eyebrow') }}</p>
            <h2 class="jh-h2 jack-display mt-3">{{ th('jackHome.code.title1') }}<br />{{ th('jackHome.code.title2') }}</h2>
            <ul class="jh-checks">
              <li>{{ th('jackHome.code.check1') }}</li>
              <li>{{ th('jackHome.code.check2') }}</li>
              <li>{{ th('jackHome.code.check3') }}</li>
            </ul>
            <button type="button" class="jh-baseurl" :title="th('jackHome.code.copy')" @click="copy(apiBaseUrl, 'base')">
              <span class="jh-baseurl-label">{{ th('jackHome.code.baseUrl') }}</span>
              <code>{{ apiBaseUrl }}</code>
              <Icon :name="copiedId === 'base' ? 'check' : 'copy'" size="sm" />
            </button>
          </div>

          <div class="jh-window">
            <div class="jh-window-bar">
              <i></i><i></i><i></i>
              <div class="jh-tabs" role="tablist">
                <button
                  v-for="snippet in snippets"
                  :id="`jh-tab-${snippet.id}`"
                  :key="snippet.id"
                  type="button"
                  role="tab"
                  class="jh-tab"
                  :class="{ 'is-active': activeSnippetId === snippet.id }"
                  :aria-selected="activeSnippetId === snippet.id"
                  :aria-controls="`jh-panel-${snippet.id}`"
                  :tabindex="activeSnippetId === snippet.id ? 0 : -1"
                  @click="selectSnippet(snippet.id)"
                  @keydown.right.prevent="cycleSnippet(1)"
                  @keydown.left.prevent="cycleSnippet(-1)"
                >
                  {{ snippet.label }}
                </button>
              </div>
              <button type="button" class="jh-copy" @click="copy(activeSnippetText, 'snippet')">
                <Icon :name="copiedId === 'snippet' ? 'check' : 'copy'" size="xs" />
                {{ copiedId === 'snippet' ? th('jackHome.code.copied') : th('jackHome.code.copy') }}
              </button>
            </div>
            <pre
              :id="`jh-panel-${activeSnippet.id}`"
              :key="activeSnippet.id"
              role="tabpanel"
              :aria-labelledby="`jh-tab-${activeSnippet.id}`"
              class="jh-pre"
            ><span
              v-for="(line, i) in activeSnippet.lines"
              :key="i"
              class="jh-line"
              :class="line.kind ? `is-${line.kind}` : ''"
              :style="{ '--i': i }"
            >{{ line.text || ' ' }}</span><span class="jh-caret" aria-hidden="true"></span></pre>
          </div>
        </div>
      </section>

      <!-- Live model catalogue (only when the Model Plaza is open to this visitor) -->
      <section
        v-if="catalogState !== 'hidden'"
        v-reveal
        data-testid="jack-home-models"
        class="jh-section mx-auto max-w-6xl px-4 sm:px-6"
      >
        <div class="jh-section-head">
          <div>
            <p class="jack-eyebrow">{{ th('jackHome.models.eyebrow') }}</p>
            <h2 class="jh-h2 jack-display mt-3">{{ th('jackHome.models.title') }}</h2>
          </div>
          <p class="jh-lead jh-section-lead">{{ th('jackHome.models.lead') }}</p>
        </div>

        <div class="jh-catalog">
          <div class="jh-catalog-head">
            <div class="jh-filter" role="group">
              <button
                type="button"
                class="jh-filter-btn"
                :class="{ 'is-active': !pinnedPlatform }"
                :aria-pressed="!pinnedPlatform"
                @click="pinPlatform('')"
              >
                {{ th('jackHome.models.all') }}
              </button>
              <button
                v-for="platform in catalog.platforms"
                :key="platform.platform"
                type="button"
                class="jh-filter-btn"
                :class="{ 'is-active': pinnedPlatform === platform.platform }"
                :aria-pressed="pinnedPlatform === platform.platform"
                @click="pinPlatform(platform.platform)"
              >
                <i :style="{ background: platformInk(platform.platform) }"></i>{{ platform.label }}
                <span>{{ platform.models.length }}</span>
              </button>
            </div>
            <span v-if="catalogState === 'ready'" class="jh-catalog-count">
              {{ th('jackHome.models.count', { models: catalog.total, platforms: catalog.platforms.length }) }}
            </span>
          </div>

          <div v-if="catalogState === 'loading'" class="jh-chips" aria-busy="true">
            <span v-for="n in 12" :key="n" class="jh-chip jh-chip-skeleton" :style="{ width: `${80 + ((n * 37) % 90)}px` }"></span>
          </div>
          <TransitionGroup v-else tag="div" name="jh-chip" class="jh-chips">
            <span
              v-for="model in visibleModels"
              :key="`${model.platform}/${model.name}`"
              class="jh-chip"
              :title="model.name"
            >
              <i :style="{ background: platformInk(model.platform) }"></i>{{ model.name }}
            </span>
            <router-link v-if="hiddenModelCount > 0" key="more" to="/model-plaza" class="jh-chip jh-chip-more">
              {{ th('jackHome.models.more', { n: hiddenModelCount }) }}
            </router-link>
          </TransitionGroup>

          <router-link to="/model-plaza" class="jh-catalog-link jh-arrow">
            {{ th('jackHome.models.viewAll') }} <Icon name="arrowRight" size="sm" />
          </router-link>
        </div>
      </section>

      <!-- Features -->
      <section v-reveal class="jh-section mx-auto max-w-6xl px-4 sm:px-6">
        <div class="jh-section-head">
          <div>
            <p class="jack-eyebrow">{{ th('jackHome.features.eyebrow') }}</p>
            <h2 class="jh-h2 jack-display mt-3">{{ th('jackHome.features.title') }}</h2>
          </div>
          <p class="jh-lead jh-section-lead">{{ th('jackHome.features.lead') }}</p>
        </div>
        <div class="jh-bento" @pointermove="spotlight">
          <article class="jh-card jh-span-4">
            <span class="jack-eyebrow">{{ th('jackHome.features.gateway.eyebrow') }}</span>
            <h3>{{ th('jackHome.features.gateway.title') }}</h3>
            <p>{{ th('jackHome.features.gateway.desc') }}</p>
            <div class="jh-endpoints">
              <span v-for="endpoint in endpoints" :key="endpoint.path" class="jh-endpoint">
                <i :style="{ background: platformInk(endpoint.platform) }"></i>{{ endpoint.path }}
              </span>
            </div>
          </article>
          <article class="jh-card jh-span-2">
            <span class="jack-eyebrow">{{ th('jackHome.features.metered.eyebrow') }}</span>
            <h3>{{ th('jackHome.features.metered.title') }}</h3>
            <p>{{ th('jackHome.features.metered.desc') }}</p>
            <div class="jh-bars" aria-hidden="true">
              <i v-for="(h, i) in bars" :key="i" :style="{ height: `${h}%`, '--i': i }"></i>
            </div>
          </article>
          <article class="jh-card jh-span-2">
            <span class="jack-eyebrow">{{ th('jackHome.features.sticky.eyebrow') }}</span>
            <h3>{{ th('jackHome.features.sticky.title') }}</h3>
            <p>{{ th('jackHome.features.sticky.desc') }}</p>
            <div class="jh-sticky" aria-hidden="true"><b>sk</b><span></span><b>A</b></div>
          </article>
          <article class="jh-card jh-span-2">
            <span class="jack-eyebrow">{{ th('jackHome.features.resilient.eyebrow') }}</span>
            <h3>{{ th('jackHome.features.resilient.title') }}</h3>
            <p>{{ th('jackHome.features.resilient.desc') }}</p>
            <div class="jh-pool" aria-hidden="true"><i></i><i></i><i class="is-down"></i><i></i><i></i></div>
          </article>
          <article class="jh-card jh-span-2">
            <span class="jack-eyebrow">{{ th('jackHome.features.transparent.eyebrow') }}</span>
            <h3>{{ th('jackHome.features.transparent.title') }}</h3>
            <p>{{ th('jackHome.features.transparent.desc') }}</p>
            <div class="jh-figure">100<small>% {{ th('jackHome.features.transparent.figure') }}</small></div>
          </article>
        </div>
      </section>

      <!-- FAQ -->
      <section v-reveal class="jh-section mx-auto max-w-6xl px-4 sm:px-6">
        <div class="jh-faq">
          <div>
            <p class="jack-eyebrow">{{ th('jackHome.faq.eyebrow') }}</p>
            <h2 class="jh-h2 jack-display mt-3">{{ th('jackHome.faq.title') }}</h2>
            <p class="jh-lead mt-4 text-[0.95rem]">{{ th('jackHome.faq.lead') }}</p>
          </div>
          <div>
            <details v-for="n in 4" :key="n" :open="n === 1">
              <summary>{{ th(`jackHome.faq.q${n}`) }}</summary>
              <p>{{ th(`jackHome.faq.a${n}`) }}</p>
            </details>
          </div>
        </div>
      </section>

      <!-- Closing -->
      <section class="mx-auto max-w-6xl px-4 sm:px-6">
        <div v-reveal class="jh-end jack-canvas">
          <div class="jh-stamp" aria-hidden="true">通</div>
          <p class="jack-eyebrow jh-end-eyebrow">{{ th('jackHome.end.eyebrow') }}</p>
          <h2 class="jack-display jh-end-title">{{ th('jackHome.end.title') }}</h2>
          <p class="jh-end-lead">{{ th('jackHome.end.lead') }}</p>
          <div class="flex flex-wrap items-center justify-center gap-3">
            <router-link :to="heroCta.to" class="jh-end-primary jh-arrow">
              {{ heroCta.label }} <Icon name="arrowRight" size="sm" />
            </router-link>
            <router-link v-if="!isAuthenticated && heroCta.to !== '/login'" to="/login" class="jh-end-secondary">
              {{ th('jackHome.end.login') }}
            </router-link>
          </div>
        </div>
      </section>
    </main>

    <footer class="mt-16 border-t border-gray-900/5 py-6 text-sm text-gray-500 dark:border-white/5 dark:text-dark-400">
      <div class="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3 px-4 sm:px-6">
        <span>&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</span>
        <a :href="githubUrl" target="_blank" rel="noopener noreferrer" class="hover:text-gray-900 dark:hover:text-white">
          Powered by Sub2API
        </a>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, type Directive } from 'vue'
import { useI18n, type UseI18nOptions } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import HomeView from '@/views/HomeView.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { getModelPlaza } from '@/api/modelPlaza'
import { useClipboard } from '@/composables/useClipboard'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import InkCanvas from '@/jack/home/InkCanvas.vue'
import { jackHomeMessages } from '@/jack/home/messages'
import {
  DEFAULT_PLATFORMS,
  buildSnippets,
  platformInk,
  snippetText,
  summarizePlaza,
  type Catalog,
  type Snippet
} from '@/jack/home/catalog'
import { platformLabel } from '@/utils/platformColors'

// Global messages (shared upstream keys) and this page's own copy.
const { t } = useI18n()
const localOptions: UseI18nOptions = { useScope: 'local', messages: jackHomeMessages }
const { t: th } = useI18n(localOptions)
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
const apiBaseUrl = computed(() => {
  const configured = sanitizeUrl(appStore.cachedPublicSettings?.api_base_url || '')
  return (configured || window.location.origin).replace(/\/+$/, '')
})
const apiHost = computed(() => apiBaseUrl.value.replace(/^https?:\/\//, ''))
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

const isAuthenticated = computed(() => authStore.isAuthenticated)
const primaryLink = computed(() => {
  if (!isAuthenticated.value) return '/login'
  return authStore.isAdmin ? '/admin/dashboard' : '/dashboard'
})
/** Hero and closing call to action: dashboard, sign-up when open, otherwise sign-in. */
const heroCta = computed(() => {
  if (isAuthenticated.value) return { to: primaryLink.value, label: th('jackHome.ctaDashboard') }
  if (appStore.cachedPublicSettings?.registration_enabled === true) {
    return { to: '/register', label: th('jackHome.ctaRegister') }
  }
  return { to: '/login', label: th('jackHome.ctaLogin') }
})
const showModelPlazaEntry = computed(
  () =>
    isFeatureFlagEnabled(FeatureFlags.modelPlaza) &&
    (isAuthenticated.value || appStore.cachedPublicSettings?.model_plaza_require_auth !== true)
)

const titleLead = computed(() => Array.from(th('jackHome.titleLead')))
const titleTail = computed(() => Array.from(th('jackHome.titleTail')))

// ---------- Model catalogue (Model Plaza) ----------
const catalog = ref<Catalog>({ platforms: [], total: 0 })
const catalogState = ref<'hidden' | 'loading' | 'ready'>('hidden')
let catalogRequest: AbortController | null = null

async function loadCatalog() {
  catalogRequest?.abort()
  const request = new AbortController()
  catalogRequest = request
  catalogState.value = 'loading'
  try {
    const summary = summarizePlaza(await getModelPlaza({ signal: request.signal }))
    if (request.signal.aborted) return
    catalog.value = summary
    catalogState.value = summary.total > 0 ? 'ready' : 'hidden'
  } catch {
    // Plaza switched off (404), sign-in required, or offline: the page simply
    // leaves the catalogue out.
    if (!request.signal.aborted) catalogState.value = 'hidden'
  }
}

watch(
  [showModelPlazaEntry, useUpstreamHome],
  ([open, upstream]) => {
    if (open && !upstream) {
      if (catalogState.value === 'hidden' && !catalog.value.total) void loadCatalog()
    } else {
      catalogRequest?.abort()
      catalogState.value = 'hidden'
    }
  },
  { immediate: true }
)

const pinnedPlatform = ref('')
const activePlatform = ref('')
function pinPlatform(platform: string) {
  pinnedPlatform.value = platform
  activePlatform.value = platform
}

const MODEL_LIMIT = 28
const filteredModels = computed(() =>
  catalog.value.platforms
    .filter((p) => !pinnedPlatform.value || p.platform === pinnedPlatform.value)
    .flatMap((p) => p.models.map((name) => ({ platform: p.platform, name })))
)
const visibleModels = computed(() => filteredModels.value.slice(0, MODEL_LIMIT))
const hiddenModelCount = computed(() => Math.max(0, filteredModels.value.length - MODEL_LIMIT))

// ---------- Hero diagram ----------
const MAX_NODES = 4
const diagramPlatforms = computed(() => {
  const live = catalog.value.platforms
  if (!live.length) return DEFAULT_PLATFORMS.map((platform) => ({ platform, count: 0 }))
  return live.slice(0, MAX_NODES).map((p) => ({ platform: p.platform, count: p.models.length }))
})
const extraPlatforms = computed(() => Math.max(0, catalog.value.platforms.length - MAX_NODES))

function nodeY(index: number, count: number) {
  const gap = 77
  return 220 - ((count - 1) * gap) / 2 + index * gap
}
function routePath(y: number) {
  return y === 220 ? 'M330 220H420' : `M330 220C378 220 372 ${y} 420 ${y}`
}

const diagramNodes = computed(() => {
  const slots = diagramPlatforms.value.length + (extraPlatforms.value > 0 ? 1 : 0)
  return diagramPlatforms.value.map((p, i) => {
    const y = nodeY(i, slots)
    return {
      platform: p.platform,
      label: platformLabel(p.platform),
      count: p.count,
      ink: platformInk(p.platform),
      y,
      path: routePath(y),
      speed: `${1.2 + ((i * 0.35) % 0.8)}s`,
      pulse: `${2.8 + i * 0.4}s`,
      begin: `${(i * 0.55) % 1.8}s`
    }
  })
})
const extraNode = computed(() => {
  const slots = diagramPlatforms.value.length + 1
  const y = nodeY(slots - 1, slots)
  return { y, path: routePath(y) }
})

const marqueeItems = computed(() => {
  const models = catalog.value.platforms.flatMap((p) => p.models)
  if (models.length >= 6) return models.slice(0, 24)
  return ['Claude Code', 'Codex CLI', 'Anthropic Messages', 'OpenAI Chat Completions', 'OpenAI Responses', 'Gemini', 'Streaming', 'Tool use']
})

// ---------- Steps ----------
const steps = computed(() => [
  { key: 's1', numeral: '壹', tag: 'SIGN UP', link: isAuthenticated.value ? null : { to: heroCta.value.to, label: heroCta.value.label } },
  { key: 's2', numeral: '貳', tag: 'CREATE KEY', link: isAuthenticated.value ? { to: '/keys', label: th('jackHome.code.createKey') } : null },
  { key: 's3', numeral: '參', tag: 'POINT HERE', link: null }
])

// ---------- Code ----------
const snippets = computed<Snippet[]>(() =>
  buildSnippets(apiBaseUrl.value, th('jackHome.code.keyPlaceholder'), catalog.value)
)
const activeSnippetId = ref<Snippet['id']>('claude-code')
const activeSnippet = computed(() => snippets.value.find((s) => s.id === activeSnippetId.value) ?? snippets.value[0])
const activeSnippetText = computed(() => snippetText(activeSnippet.value))

/** Select a snippet tab and keep it visible in the strip, which scrolls on phones. */
function selectSnippet(id: Snippet['id'], focus = false) {
  activeSnippetId.value = id
  const tab = document.getElementById(`jh-tab-${id}`)
  if (focus) tab?.focus()
  tab?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
}

function cycleSnippet(step: number) {
  const list = snippets.value
  const index = list.findIndex((s) => s.id === activeSnippetId.value)
  selectSnippet(list[(index + step + list.length) % list.length].id, true)
}

const { copyToClipboard } = useClipboard()
const copiedId = ref('')
let copiedTimer = 0
async function copy(text: string, id: string) {
  if (await copyToClipboard(text)) {
    copiedId.value = id
    window.clearTimeout(copiedTimer)
    copiedTimer = window.setTimeout(() => (copiedId.value = ''), 1800)
  }
}

// ---------- Features ----------
const endpoints = [
  { path: '/v1/messages', platform: 'anthropic' },
  { path: '/v1/chat/completions', platform: 'openai' },
  { path: '/v1/responses', platform: 'openai' },
  { path: '/v1beta/models', platform: 'gemini' }
]
const bars = [34, 52, 41, 68, 57, 80, 72, 94]

/** Cards light up under the pointer. */
function spotlight(event: PointerEvent) {
  const card = (event.target as HTMLElement | null)?.closest?.('.jh-card') as HTMLElement | null
  if (!card) return
  const rect = card.getBoundingClientRect()
  card.style.setProperty('--mx', `${event.clientX - rect.left}px`)
  card.style.setProperty('--my', `${event.clientY - rect.top}px`)
}

// ---------- Scroll reveal ----------
let revealObserver: IntersectionObserver | null = null
const vReveal: Directive<HTMLElement> = {
  mounted(el) {
    el.classList.add('jh-reveal')
    if (typeof IntersectionObserver === 'undefined') {
      el.classList.add('is-in')
      return
    }
    revealObserver ??= new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue
          entry.target.classList.add('is-in')
          revealObserver?.unobserve(entry.target)
        }
      },
      { rootMargin: '0px 0px -12% 0px', threshold: 0.08 }
    )
    revealObserver.observe(el)
  },
  beforeUnmount(el) {
    revealObserver?.unobserve(el)
  }
}

function scrollToStart() {
  document.getElementById('jack-home-start')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

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

onBeforeUnmount(() => {
  catalogRequest?.abort()
  revealObserver?.disconnect()
  window.clearTimeout(copiedTimer)
})
</script>

<style scoped>
.jh {
  --jh-muted: rgb(var(--jack-gray-500));
  --jh-soft: rgb(var(--jack-gray-600));
  --jh-madder: oklch(0.558 0.12 24);
  --jh-verdigris: oklch(0.648 0.071 172);
  --jh-ease: cubic-bezier(0.2, 0.7, 0.2, 1);
  color: var(--jack-ink);
}

:global(.dark) .jh {
  --jh-muted: rgb(var(--jack-dark-400));
  --jh-soft: rgb(var(--jack-dark-300));
  --jh-madder: oklch(0.648 0.12 24);
}

.jh-lead {
  color: var(--jh-soft);
  font-size: 1rem;
  line-height: 1.8;
}

.jh-arrow :deep(svg) {
  transition: transform 0.25s var(--jh-ease);
}

.jh-arrow:hover :deep(svg) {
  transform: translateX(3px);
}

/* ---------- Hero ---------- */
.jh-hero-stage {
  position: relative;
  isolation: isolate;
}

.jh-paper {
  position: absolute;
  inset: 0;
  z-index: -1;
  pointer-events: none;
  background-image: linear-gradient(var(--jack-line) 1px, transparent 1px),
    linear-gradient(90deg, var(--jack-line) 1px, transparent 1px);
  background-size: 56px 56px;
  mask-image: radial-gradient(ellipse 70% 60% at 70% 35%, #000 10%, transparent 75%);
  opacity: 0.55;
}

.jh-hero {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  align-items: center;
  gap: 3rem;
  padding-top: 4.5rem;
  padding-bottom: 3rem;
}

@media (min-width: 1024px) {
  .jh-hero {
    grid-template-columns: minmax(0, 1.02fr) minmax(0, 1fr);
    padding-top: 6.5rem;
    padding-bottom: 4rem;
  }
}

.jh-title {
  font-size: clamp(2.6rem, 6vw, 4.6rem);
  line-height: 1.06;
}

.jh-title-lead {
  background-image: var(--jack-chrome);
  -webkit-background-clip: text;
  background-clip: text;
}

.jh-title-lead .jh-char {
  color: transparent;
  background: inherit;
  -webkit-background-clip: text;
  background-clip: text;
}

.jh-title-tail {
  white-space: nowrap;
}

/* Characters soak into the paper one by one. */
.jh-char {
  display: inline-block;
  opacity: 0;
  filter: blur(10px);
  transform: translateY(0.08em);
  animation: jh-soak 0.9s var(--jh-ease) forwards;
  animation-delay: calc(80ms + var(--i) * 45ms);
}

@keyframes jh-soak {
  60% {
    opacity: 1;
  }
  to {
    opacity: 1;
    filter: blur(0);
    transform: none;
  }
}

.jh-rise {
  opacity: 0;
  transform: translateY(14px);
  animation: jh-rise 0.9s var(--jh-ease) forwards;
  animation-delay: var(--d, 0ms);
}

@keyframes jh-rise {
  to {
    opacity: 1;
    transform: none;
  }
}

.jh-protocols {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.4rem 1rem;
  font-size: 0.8rem;
  color: var(--jh-muted);
}

.jh-protocols span:not(.jh-protocols-label):not(.jh-live-dot) {
  color: var(--jh-soft);
  font-weight: 500;
}

.jh-protocols-label {
  margin-left: -0.5rem;
}

.jh-live-dot {
  width: 6px;
  height: 6px;
  border-radius: 9999px;
  background: var(--jh-verdigris);
  box-shadow: 0 0 0 3px color-mix(in oklab, var(--jh-verdigris) 22%, transparent);
}

/* ---------- Diagram ---------- */
.jh-diagram {
  margin: 0;
  border-radius: 1.25rem;
  border: 1px solid var(--jack-line);
  background: var(--jack-raised);
  box-shadow: var(--jack-shadow-card-hover);
  padding: 1.1rem 1rem 0.6rem;
}

.jh-diagram-cap {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  padding: 0 0.4rem 0.4rem;
  font-family: theme('fontFamily.mono');
  font-size: 0.7rem;
  color: var(--jh-muted);
}

.jh-diagram-cap span:first-child {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 480px) {
  .jh-diagram-cap span:last-child {
    display: none;
  }
}

.jh-svg {
  display: block;
  width: 100%;
  height: auto;
  overflow: visible;
}

.jh-wire {
  fill: none;
  stroke: var(--jack-line-strong);
  stroke-width: 1.2;
}

.jh-flow {
  fill: none;
  stroke: var(--jack-ink);
  stroke-width: 1.4;
  stroke-dasharray: 3 11;
  stroke-linecap: round;
  opacity: 0.7;
  animation: jh-flow 1.4s linear infinite;
  transition: opacity 0.3s, stroke-width 0.3s;
}

.is-active > .jh-flow {
  stroke-width: 2.2;
  opacity: 1;
}

.is-dim > .jh-flow,
.is-dim > .jh-pulse {
  opacity: 0.15;
}

@keyframes jh-flow {
  to {
    stroke-dashoffset: -28;
  }
}

.jh-pulse {
  fill: var(--jack-ink);
  transition: opacity 0.3s;
}

.jh-key {
  fill: var(--jack-ink);
}

.jh-key-text {
  fill: var(--jack-btn-fg);
  font-family: theme('fontFamily.mono');
  font-size: 13px;
}

:global(.dark) .jh-key-text {
  fill: #111113;
}

.jh-sub {
  fill: var(--jh-muted);
  font-family: theme('fontFamily.mono');
  font-size: 10px;
  letter-spacing: 0.08em;
}

.jh-hub {
  fill: var(--jack-raised);
  stroke: var(--jack-ink);
  stroke-width: 1.2;
}

.jh-ring {
  fill: none;
  stroke: var(--jack-line-strong);
  stroke-dasharray: 2 5;
  transform-origin: 290px 220px;
  animation: jh-spin 28s linear infinite;
}

@keyframes jh-spin {
  to {
    transform: rotate(360deg);
  }
}

.jh-hub-text {
  fill: var(--jack-ink);
  font-family: theme('fontFamily.display');
  font-size: 21px;
}

.jh-seal {
  fill: var(--jh-madder);
}

.jh-seal-text {
  fill: #fff8f2;
  font-family: theme('fontFamily.display');
  font-size: 20px;
}

.jh-node {
  cursor: default;
}

.jh-node-box {
  fill: var(--jack-raised);
  stroke: var(--jack-line-strong);
  stroke-width: 1;
  transition: stroke 0.25s, fill 0.25s;
}

.jh-node.is-active .jh-node-box {
  stroke: var(--jack-ink);
  fill: var(--jack-raised-hover);
}

.jh-node-more {
  stroke-dasharray: 3 4;
}

.jh-node-label {
  fill: var(--jack-ink);
  font-size: 13px;
  font-weight: 500;
}

.jh-node-count {
  fill: var(--jh-muted);
  font-family: theme('fontFamily.mono');
  font-size: 9.5px;
}

/* The diagram scales down on phones; keep its labels legible. */
@media (max-width: 640px) {
  .jh-node-label {
    font-size: 16px;
  }

  .jh-node-count {
    font-size: 12px;
  }

  .jh-sub {
    font-size: 12.5px;
  }

  .jh-key-text {
    font-size: 15px;
  }
}

/* ---------- Marquee ---------- */
.jh-marquee {
  overflow: hidden;
  border-block: 1px solid var(--jack-line);
  padding: 1.1rem 0;
  mask-image: linear-gradient(90deg, transparent, #000 12%, #000 88%, transparent);
}

.jh-track {
  display: flex;
  width: max-content;
  animation: jh-scroll 42s linear infinite;
}

.jh-marquee:hover .jh-track {
  animation-play-state: paused;
}

.jh-track > span {
  display: inline-flex;
  align-items: center;
  gap: 2.5rem;
  padding-right: 2.5rem;
  font-family: theme('fontFamily.mono');
  font-size: 0.82rem;
  color: var(--jh-soft);
  white-space: nowrap;
}

.jh-track i {
  width: 4px;
  height: 4px;
  border-radius: 9999px;
  background: var(--jack-line-strong);
}

@keyframes jh-scroll {
  to {
    transform: translateX(-50%);
  }
}

/* ---------- Sections ---------- */
.jh-section {
  padding-top: 6rem;
  scroll-margin-top: 4rem;
}

.jh-section-head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 1.25rem 2rem;
  margin-bottom: 2.5rem;
}

.jh-section-lead {
  max-width: 26rem;
  font-size: 0.95rem;
}

.jh-h2 {
  font-size: clamp(2rem, 4.2vw, 3rem);
  line-height: 1.1;
}

.jh-reveal {
  opacity: 0;
  transform: translateY(28px);
  transition: opacity 0.9s var(--jh-ease), transform 0.9s var(--jh-ease);
}

.jh-reveal.is-in {
  opacity: 1;
  transform: none;
}

/* ---------- Steps ---------- */
.jh-steps {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  border-top: 1px solid var(--jack-ink);
}

@media (min-width: 768px) {
  .jh-steps {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

.jh-step {
  position: relative;
  padding: 1.75rem 1.5rem 2rem 0;
  border-bottom: 1px solid var(--jack-line);
}

@media (min-width: 768px) {
  .jh-step {
    border-bottom: 0;
    padding-right: 2rem;
  }

  .jh-step + .jh-step {
    padding-left: 2rem;
    border-left: 1px solid var(--jack-line);
  }
}

.jh-step-n {
  font-family: theme('fontFamily.display');
  font-size: 3.25rem;
  line-height: 1;
}

.jh-step-n small {
  margin-left: 0.6rem;
  vertical-align: 0.9rem;
  font-family: theme('fontFamily.mono');
  font-size: 0.7rem;
  letter-spacing: 0.12em;
  color: var(--jh-muted);
}

.jh-reveal .jh-step-n {
  opacity: 0;
  filter: blur(8px);
  transition: opacity 0.8s var(--jh-ease), filter 0.8s var(--jh-ease);
  transition-delay: calc(200ms + var(--i) * 160ms);
}

.jh-reveal.is-in .jh-step-n {
  opacity: 1;
  filter: none;
}

.jh-step h3 {
  margin: 1.1rem 0 0.5rem;
  font-size: 1.1rem;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.jh-step p {
  color: var(--jh-soft);
  font-size: 0.9rem;
  line-height: 1.75;
}

.jh-step-link {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  margin-top: 0.9rem;
  font-size: 0.85rem;
  font-weight: 500;
  border-bottom: 1px solid var(--jack-line-strong);
  transition: border-color 0.2s;
}

.jh-step-link:hover {
  border-color: var(--jack-ink);
}

/* ---------- Code ---------- */
.jh-code {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  align-items: start;
  gap: 2.5rem;
}

@media (min-width: 1024px) {
  .jh-code {
    grid-template-columns: minmax(0, 0.8fr) minmax(0, 1.2fr);
  }
}

.jh-checks {
  display: grid;
  gap: 0.85rem;
  margin-top: 2rem;
}

.jh-checks li {
  display: flex;
  gap: 0.75rem;
  color: var(--jh-soft);
  font-size: 0.9rem;
  line-height: 1.6;
}

.jh-checks li::before {
  content: '';
  flex: none;
  width: 18px;
  height: 18px;
  margin-top: 2px;
  border: 1px solid var(--jack-line);
  border-radius: 9999px;
  background: var(--jack-chip)
    url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16' fill='none' stroke='%2377797e' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M4 8.5l2.5 2.5L12 5.5'/%3E%3C/svg%3E")
    center / 12px no-repeat;
}

.jh-baseurl {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  width: 100%;
  margin-top: 2rem;
  padding: 0.8rem 1rem;
  border: 1px dashed var(--jack-line-strong);
  border-radius: 0.75rem;
  background: var(--jack-raised);
  color: var(--jh-muted);
  text-align: left;
  transition: border-color 0.2s, box-shadow 0.2s;
}

.jh-baseurl:hover {
  border-color: var(--jack-ink);
  box-shadow: var(--jack-shadow-card-hover);
}

.jh-baseurl-label {
  flex: none;
  font-size: 0.72rem;
  letter-spacing: 0.06em;
}

.jh-baseurl code {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: theme('fontFamily.mono');
  font-size: 0.85rem;
  color: var(--jack-ink);
}

.jh-window {
  overflow: hidden;
  border-radius: 0.875rem;
  background: var(--jack-code-bg);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.06), 0 40px 80px -36px rgba(10, 11, 13, 0.55);
}

.jh-window-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.jh-window-bar > i {
  flex: none;
  width: 10px;
  height: 10px;
  border-radius: 9999px;
  background: rgba(255, 255, 255, 0.16);
}

.jh-tabs {
  display: flex;
  flex: 1;
  min-width: 0;
  gap: 2px;
  margin-left: 0.6rem;
  overflow-x: auto;
  scrollbar-width: none;
}

.jh-tab,
.jh-copy {
  flex: none;
  padding: 0.35rem 0.7rem;
  border-radius: 0.45rem;
  font-family: theme('fontFamily.mono');
  font-size: 0.74rem;
  color: rgba(255, 255, 255, 0.45);
  transition: color 0.2s, background 0.2s;
}

.jh-tab:hover,
.jh-copy:hover {
  color: rgba(255, 255, 255, 0.85);
}

.jh-tab.is-active {
  color: #fff;
  background: rgba(255, 255, 255, 0.09);
}

.jh-tab:focus-visible,
.jh-copy:focus-visible {
  outline: 2px solid rgba(255, 255, 255, 0.6);
  outline-offset: 1px;
}

.jh-copy {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.jh-pre {
  min-height: 17rem;
  margin: 0;
  padding: 1.1rem 1.25rem 1.5rem;
  overflow-x: auto;
  font-family: theme('fontFamily.mono');
  font-size: 12.8px;
  line-height: 1.85;
  color: rgba(255, 255, 255, 0.86);
}

.jh-line {
  display: block;
  white-space: pre;
  opacity: 0;
  transform: translateX(-6px);
  animation: jh-type 0.4s var(--jh-ease) forwards;
  animation-delay: calc(var(--i) * 70ms);
  animation-play-state: paused;
}

.jh-reveal.is-in .jh-line {
  animation-play-state: running;
}

.jh-line.is-comment {
  color: rgba(255, 255, 255, 0.34);
}

.jh-line.is-prompt {
  color: #e9d9b6;
}

@keyframes jh-type {
  to {
    opacity: 1;
    transform: none;
  }
}

.jh-caret {
  display: inline-block;
  width: 7px;
  height: 1.05em;
  margin-top: 0.3rem;
  background: rgba(255, 255, 255, 0.7);
  animation: jh-blink 1.1s steps(1) infinite;
}

@keyframes jh-blink {
  50% {
    opacity: 0;
  }
}

/* ---------- Catalogue ---------- */
.jh-catalog {
  padding: 1.5rem;
  border: 1px solid var(--jack-line);
  border-radius: 1.25rem;
  background: var(--jack-raised);
  box-shadow: var(--jack-shadow-card);
}

.jh-catalog-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding-bottom: 1.25rem;
  border-bottom: 1px solid var(--jack-line);
}

.jh-filter {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  padding: 0.25rem;
  border-radius: 0.6rem;
  background: var(--jack-chip);
}

.jh-filter-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.35rem 0.75rem;
  border-radius: 0.45rem;
  font-size: 0.8rem;
  color: var(--jh-soft);
  transition: background 0.2s, color 0.2s, box-shadow 0.2s;
}

.jh-filter-btn i {
  width: 7px;
  height: 7px;
  border-radius: 2px;
}

.jh-filter-btn span {
  font-family: theme('fontFamily.mono');
  font-size: 0.7rem;
  color: var(--jh-muted);
}

.jh-filter-btn.is-active {
  background: var(--jack-raised-solid);
  color: var(--jack-ink);
  box-shadow: 0 0 0 1px var(--jack-line), 0 1px 2px rgba(0, 0, 0, 0.06);
}

.jh-catalog-count {
  font-family: theme('fontFamily.mono');
  font-size: 0.75rem;
  color: var(--jh-muted);
}

.jh-chips {
  position: relative;
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  min-height: 5rem;
  padding: 1.5rem 0;
}

.jh-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  max-width: 100%;
  height: 2rem;
  padding: 0 0.75rem;
  overflow: hidden;
  border: 1px solid var(--jack-line);
  border-radius: 0.5rem;
  background: var(--jack-sheet);
  font-family: theme('fontFamily.mono');
  font-size: 0.76rem;
  color: var(--jh-soft);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.jh-chip i {
  flex: none;
  width: 6px;
  height: 6px;
  border-radius: 2px;
}

.jh-chip-more {
  border-style: dashed;
  color: var(--jack-ink);
}

.jh-chip-skeleton {
  background: var(--jack-chip);
  animation: jh-breathe 1.4s ease-in-out infinite alternate;
}

@keyframes jh-breathe {
  to {
    opacity: 0.45;
  }
}

.jh-chip-enter-active,
.jh-chip-leave-active {
  transition: opacity 0.3s, transform 0.3s var(--jh-ease);
}

.jh-chip-enter-from,
.jh-chip-leave-to {
  opacity: 0;
  transform: scale(0.94);
}

.jh-chip-leave-active {
  position: absolute;
}

.jh-chip-move {
  transition: transform 0.35s var(--jh-ease);
}

.jh-catalog-link {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding-top: 1.1rem;
  border-top: 1px solid var(--jack-line);
  width: 100%;
  font-size: 0.875rem;
  font-weight: 500;
}

/* ---------- Bento ---------- */
.jh-bento {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 1rem;
}

@media (min-width: 768px) {
  .jh-bento {
    grid-template-columns: repeat(6, minmax(0, 1fr));
  }

  .jh-span-4 {
    grid-column: span 4;
  }

  .jh-span-2 {
    grid-column: span 2;
  }
}

.jh-card {
  position: relative;
  overflow: hidden;
  padding: 1.75rem;
  border: 1px solid var(--jack-line);
  border-radius: 1rem;
  background: var(--jack-raised);
  box-shadow: var(--jack-shadow-card);
  transition: box-shadow 0.3s, transform 0.3s var(--jh-ease);
}

.jh-card::before {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  opacity: 0;
  background: radial-gradient(360px circle at var(--mx, 50%) var(--my, 50%), var(--jack-halo), transparent 65%);
  transition: opacity 0.3s;
}

.jh-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--jack-shadow-card-hover);
}

.jh-card:hover::before {
  opacity: 1;
}

.jh-card .jack-eyebrow {
  display: block;
  margin-bottom: 1.1rem;
}

.jh-card h3 {
  margin-bottom: 0.5rem;
  font-size: 1.05rem;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.jh-card p {
  color: var(--jh-soft);
  font-size: 0.875rem;
  line-height: 1.75;
}

.jh-endpoints {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 1.5rem;
}

.jh-endpoint {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.4rem 0.7rem;
  border: 1px solid var(--jack-line);
  border-radius: 0.5rem;
  background: var(--jack-chip);
  font-family: theme('fontFamily.mono');
  font-size: 0.74rem;
  color: var(--jh-soft);
}

.jh-endpoint i {
  width: 6px;
  height: 6px;
  border-radius: 2px;
}

.jh-bars {
  display: flex;
  align-items: flex-end;
  gap: 5px;
  height: 76px;
  margin-top: 1.5rem;
}

.jh-bars i {
  flex: 1;
  border-radius: 3px 3px 1px 1px;
  background: linear-gradient(180deg, var(--jack-ink), color-mix(in oklab, var(--jack-ink) 40%, transparent));
  opacity: 0.8;
  transform: scaleY(0.08);
  transform-origin: bottom;
  transition: transform 1.1s var(--jh-ease);
  transition-delay: calc(300ms + var(--i) * 60ms);
}

.jh-bars i:last-child {
  background: var(--jh-verdigris);
  opacity: 1;
}

.jh-reveal.is-in .jh-bars i {
  transform: none;
}

.jh-sticky {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  margin-top: 1.5rem;
}

.jh-sticky b {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  border: 1px solid var(--jack-line-strong);
  border-radius: 0.5rem;
  background: var(--jack-chip);
  font-family: theme('fontFamily.mono');
  font-size: 0.72rem;
  font-weight: 500;
}

.jh-sticky span {
  flex: 1;
  height: 1px;
  background: repeating-linear-gradient(90deg, var(--jack-ink) 0 4px, transparent 4px 9px);
  background-size: 18px 1px;
  opacity: 0.6;
  animation: jh-march 1s linear infinite;
}

@keyframes jh-march {
  to {
    background-position: 18px 0;
  }
}

.jh-pool {
  display: flex;
  gap: 6px;
  margin-top: 1.5rem;
}

.jh-pool i {
  flex: 1;
  height: 30px;
  border: 1px solid var(--jack-line-strong);
  border-radius: 0.45rem;
  background: var(--jack-chip);
  position: relative;
}

.jh-pool i::after {
  content: '';
  position: absolute;
  left: 50%;
  top: 50%;
  width: 6px;
  height: 6px;
  margin: -3px 0 0 -3px;
  border-radius: 9999px;
  background: var(--jh-verdigris);
}

.jh-pool i.is-down {
  animation: jh-failover 4s ease-in-out infinite;
}

.jh-pool i.is-down::after {
  animation: jh-failover-dot 4s ease-in-out infinite;
}

@keyframes jh-failover {
  30%,
  60% {
    opacity: 0.4;
    border-style: dashed;
  }
}

@keyframes jh-failover-dot {
  30%,
  60% {
    background: var(--jh-madder);
  }
}

.jh-figure {
  margin-top: 1.25rem;
  font-family: theme('fontFamily.display');
  font-size: 3.4rem;
  line-height: 1;
  letter-spacing: -0.02em;
}

.jh-figure small {
  margin-left: 0.3rem;
  font-size: 1.1rem;
  color: var(--jh-muted);
}

/* ---------- FAQ ---------- */
.jh-faq {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 2.5rem;
}

@media (min-width: 1024px) {
  .jh-faq {
    grid-template-columns: minmax(0, 0.8fr) minmax(0, 1.2fr);
  }
}

.jh-faq details {
  border-bottom: 1px solid var(--jack-line);
}

.jh-faq details:first-child {
  border-top: 1px solid var(--jack-line);
}

.jh-faq summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 1.25rem 0;
  cursor: pointer;
  list-style: none;
  font-weight: 500;
}

.jh-faq summary::-webkit-details-marker {
  display: none;
}

.jh-faq summary::after {
  content: '';
  flex: none;
  width: 22px;
  height: 22px;
  border: 1px solid var(--jack-line-strong);
  border-radius: 9999px;
  background: linear-gradient(var(--jack-ink), var(--jack-ink)) center / 9px 1.2px no-repeat,
    linear-gradient(var(--jack-ink), var(--jack-ink)) center / 1.2px 9px no-repeat;
  transition: transform 0.3s var(--jh-ease);
}

.jh-faq details[open] summary::after {
  transform: rotate(135deg);
}

.jh-faq details p {
  padding: 0 2.5rem 1.4rem 0;
  color: var(--jh-soft);
  font-size: 0.9rem;
  line-height: 1.8;
}

/* ---------- Closing ---------- */
.jh-end {
  position: relative;
  overflow: hidden;
  margin-top: 6rem;
  padding: 4.5rem 1.75rem;
  border-radius: 1.5rem;
  color: #f4f4f3;
  text-align: center;
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.05);
}

.jh-end::before {
  content: '';
  position: absolute;
  inset: 0;
  background-image: linear-gradient(rgba(255, 255, 255, 0.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.04) 1px, transparent 1px);
  background-size: 56px 56px;
  mask-image: radial-gradient(ellipse 60% 70% at 50% 50%, #000, transparent);
}

.jh-end > * {
  position: relative;
}

.jh-end-eyebrow {
  color: #74777e;
}

.jh-end-title {
  margin-top: 1rem;
  font-size: clamp(2.4rem, 6vw, 4.25rem);
  line-height: 1.08;
  background-image: linear-gradient(180deg, #fff, #a3a3a7);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}

.jh-end-lead {
  max-width: 30rem;
  margin: 1.25rem auto 2.25rem;
  color: #a4a7ad;
  line-height: 1.8;
}

.jh-end-primary,
.jh-end-secondary {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  height: 3rem;
  padding: 0 1.35rem;
  border-radius: 0.625rem;
  font-size: 0.95rem;
  font-weight: 500;
  transition: transform 0.2s, background 0.2s;
}

.jh-end-primary {
  background-image: linear-gradient(180deg, #fff, #d7d7d9);
  color: #111113;
  box-shadow: inset 0 1px 0 #fff, 0 0 0 1px rgba(255, 255, 255, 0.2), 0 8px 24px -8px rgba(255, 255, 255, 0.3);
}

.jh-end-secondary {
  border: 1px solid rgba(255, 255, 255, 0.14);
  background: rgba(255, 255, 255, 0.04);
  color: #f4f4f3;
}

.jh-end-primary:hover,
.jh-end-secondary:hover {
  transform: translateY(-1px);
}

/* The seal is pressed onto the paper when the section comes into view. */
.jh-stamp {
  position: absolute;
  right: 8%;
  top: 20%;
  display: grid;
  place-items: center;
  width: 64px;
  height: 64px;
  border-radius: 6px;
  background: var(--jh-madder);
  color: #fff8f2;
  font-family: theme('fontFamily.display');
  font-size: 1.6rem;
  line-height: 1;
  box-shadow: inset 0 0 0 3px rgba(255, 248, 242, 0.85), inset 0 0 0 5px var(--jh-madder);
  opacity: 0;
  transform: rotate(-9deg) scale(1.6);
}

.jh-reveal.is-in .jh-stamp {
  animation: jh-press 0.7s cubic-bezier(0.3, 1.4, 0.5, 1) 0.5s forwards;
}

@keyframes jh-press {
  60% {
    opacity: 0.95;
    transform: rotate(-9deg) scale(0.94);
  }
  to {
    opacity: 0.92;
    transform: rotate(-9deg) scale(1);
  }
}

@media (max-width: 640px) {
  .jh-stamp {
    right: 6%;
    top: 7%;
    width: 48px;
    height: 48px;
    font-size: 1.2rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .jh *,
  .jh *::before,
  .jh *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    animation-delay: 0ms !important;
    transition-duration: 0.01ms !important;
    transition-delay: 0ms !important;
  }

  .jh-pulse {
    display: none;
  }
}
</style>
