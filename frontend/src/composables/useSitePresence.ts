import { onMounted, onBeforeUnmount } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { siteContent } from '@/api/siteContent'
/** 只向服务端发送已认证的可见页面活跃心跳，不制造匿名在线人数。 */
export function useSitePresence() {
 const auth = useAuthStore()
 let timer: ReturnType<typeof setInterval> | undefined
 const touch = () => { if (auth.isAuthenticated && document.visibilityState === 'visible') void siteContent.presence().catch(() => {}) }
 onMounted(() => {touch();timer=setInterval(touch,60000);document.addEventListener('visibilitychange',touch)})
 onBeforeUnmount(() => {clearInterval(timer);document.removeEventListener('visibilitychange',touch)})
}
