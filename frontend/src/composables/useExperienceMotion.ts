import { computed, watch } from 'vue'
import { usePreferredReducedMotion, useStorage } from '@vueuse/core'

/** 用户关闭优先于系统；系统要求减少动态效果时，不允许强制打开。 */
export function useExperienceMotion() {
  const preference = useStorage<'auto' | 'off'>('xigong-motion', 'auto')
  const system = usePreferredReducedMotion()
  const enabled = computed(() => preference.value !== 'off' && system.value !== 'reduce')
  watch(enabled, value => {
    document.documentElement.dataset.xgMotion = value ? 'on' : 'off'
  }, { immediate: true })
  const toggle = () => { preference.value = preference.value === 'off' ? 'auto' : 'off' }
  return { enabled, preference, toggle }
}
