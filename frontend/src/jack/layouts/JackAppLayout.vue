<template>
  <div class="jack-shell" :class="{ 'jack-shell--collapsed': sidebarCollapsed }" :style="crumbStyle">
    <!--
      Jack replacement for src/components/layout/AppLayout.vue (swapped in by the
      jack-theme Vite plugin). Upstream AppSidebar and AppHeader keep all of their
      logic: menus, feature flags, tour anchors, page titles, balance and user
      menu. This layout only composes them. The sidebar sits on the graphite
      canvas and the header and page share one paper sheet; shell.css styles it.
    -->
    <!-- The canvas is graphite in both colour modes, so the sidebar always renders dark. -->
    <div class="jack-rail dark">
      <AppSidebar />
    </div>

    <div class="jack-main">
      <div class="jack-sheet">
        <AppHeader class="jack-toolbar" />
        <main class="jack-content">
          <slot />
        </main>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from '@/components/layout/AppSidebar.vue'
import AppHeader from '@/components/layout/AppHeader.vue'

const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

/**
 * Small-caps path shown above the page title, e.g. "admin · accounts". Only
 * word segments are kept, so ids never show and the value is safe to embed as a
 * CSS string.
 */
const crumb = computed(() =>
  route.path
    .split('/')
    .filter((segment) => /^[a-z][a-z-]*$/i.test(segment))
    .map((segment) => segment.replace(/-/g, ' '))
    .join(' · ')
)
const crumbStyle = computed(() => ({ '--jack-crumb': crumb.value ? `'${crumb.value}'` : 'none' }))

// Same onboarding wiring as upstream AppLayout.
const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
