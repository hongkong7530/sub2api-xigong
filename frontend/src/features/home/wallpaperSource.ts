import type { Wallpaper } from '@/api/siteContent'
import {siteImageUrl as consoleImageUrl} from '@/utils/siteImageUrl'
export function wallpaperSource(wallpaper: Wallpaper, mobile = false): string {
 const custom = consoleImageUrl((mobile ? wallpaper.mobile_url : '') || wallpaper.desktop_url || '')
 // 横图铺满竖屏时按高度缩放，960px缩略图会被放大数倍；内置场景使用完整资源。
 return custom || `/experience/wallpapers/${wallpaper.id}-desktop.webp`
}
export function wallpaperFocus(wallpaper: Wallpaper, mobile = false) {
 return { x: mobile ? wallpaper.mobile_focus_x ?? wallpaper.focus_x : wallpaper.focus_x, y: mobile ? wallpaper.mobile_focus_y ?? wallpaper.focus_y : wallpaper.focus_y }
}
