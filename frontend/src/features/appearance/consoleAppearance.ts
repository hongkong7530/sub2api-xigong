import { defaultConsoleAppearance, type ConsoleAppearance } from '@/api/siteContent'

import {siteImageUrl as consoleImageUrl} from '@/utils/siteImageUrl'
export {consoleImageUrl}

export function resolveConsole(value?: Partial<ConsoleAppearance> | null): ConsoleAppearance {
  return { ...defaultConsoleAppearance(), ...value }
}

export function consoleSource(config: ConsoleAppearance, mobile = false): string {
  if (!config.enabled || config.background_mode === 'none') return ''
  if (config.background_mode === 'custom') return consoleImageUrl((mobile ? config.mobile_background_url : '') || config.background_url)
  return /^scene-[1-6]$/.test(config.wallpaper_id)
    ? `/experience/wallpapers/${config.wallpaper_id}-desktop.webp` : ''
}
