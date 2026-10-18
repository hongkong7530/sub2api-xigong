import { describe, expect, it } from 'vitest'
import { defaultAppearance, defaultConsoleAppearance, normalizeAppearance } from '../../../api/siteContent'
import { consoleImageUrl, consoleSource } from '../consoleAppearance'

describe('console appearance contract', () => {
  it('normalizes legacy responses without overriding explicit zero and false', () => {
    const legacy = defaultAppearance(); delete legacy.console
    expect(normalizeAppearance(legacy).console).toEqual(defaultConsoleAppearance())
    const a = defaultAppearance(); a.console = { ...defaultConsoleAppearance(), blur: 0, enabled: false }
    expect(normalizeAppearance(a).console?.blur).toBe(0)
    expect(normalizeAppearance(a).console?.enabled).toBe(false)
  })
  it.each(['javascript:alert(1)', '//evil.example/x', '/\\evil.example/x', 'http://example.com/x', 'https://u:p@example.com/x', 'https://example.com/\nx', 'data:image/svg+xml,x'])('rejects unsafe image %s', value => {
    expect(consoleImageUrl(value)).toBe('')
  })
  it('selects responsive built-in resources independently of the homepage', () => {
    const config = defaultConsoleAppearance()
    expect(consoleSource(config)).toBe('/experience/wallpapers/scene-1-desktop.webp')
    expect(consoleSource(config, true)).toBe('/experience/wallpapers/scene-1-mobile.webp')
    config.background_mode = 'custom'; config.background_url = 'https://example.com/a.webp'
    expect(consoleSource(config)).toBe(config.background_url)
    config.enabled = false; expect(consoleSource(config)).toBe('')
  })
})
