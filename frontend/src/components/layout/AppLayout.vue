<template>
  <ExperienceShell v-slot="{ flow, liquid }">
    <!-- Background Decoration -->

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
    >
      <!-- Header -->
      <AppHeader :scene-flow="flow" :scene-liquid="liquid" />

      <!-- Main Content -->
      <main id="main-content" class="xg-console-main p-4 md:p-6 lg:p-8" tabindex="-1">
        <PixelTransition :identity="route.path" :active="experience.appearance?.motion_enabled ?? false"><slot /></PixelTransition>
      </main>
    </div>
  </ExperienceShell>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'
import { useRoute } from 'vue-router'
import { useExperienceStore } from '@/stores/experience'
import ExperienceShell from '@/features/appearance/ExperienceShell.vue'
import PixelTransition from '@/components/common/PixelTransition.vue'
const route = useRoute()
const experience = useExperienceStore()

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  void experience.load()
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
