import type { ProfitMetric } from '@/api/profit'

// Format fixed decimal strings without converting money to binary floats.
export function profitMoney(value: string | null | undefined, digits = 2): string {
  if (value == null || !/^-?\d+(\.\d+)?$/.test(value)) return '—'
  const negative = value.startsWith('-')
  const [whole = '0', fraction = ''] = value.replace('-', '').split('.')
  const places = Math.max(0, Math.min(8, digits))
  const minor = BigInt(whole + fraction.padEnd(places, '0').slice(0, places))
    + (Number(fraction.charAt(places) || '0') >= 5 ? 1n : 0n)
  const text = minor.toString().padStart(places + 1, '0')
  const major = (places ? text.slice(0, -places) : text).replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  const decimal = places ? '.' + text.slice(-places) : ''
  return `${negative && minor !== 0n ? '−' : ''}$${major}${decimal}`
}
export function profitPercent(value: string | null | undefined): string {
  if (value == null || !/^-?\d+(\.\d+)?$/.test(value)) return '—'
  // Ratios only are converted for display, never for ledger calculations.
  return `${(Number(value) * 100).toLocaleString(undefined, { maximumFractionDigits: 2, minimumFractionDigits: 1 })}%`
}
export function metricProfit(metric?: ProfitMetric): string {
  return !metric || metric.known_requests === 0 ? '—' : profitMoney(metric.profit)
}
export function negativeMoney(value?: string | null): boolean { return !!value && /^-0*[1-9]|^-\d+\.\d*[1-9]/.test(value) }
export function profitTime(value?: string | null): string { return value ? new Date(value).toLocaleString() : '—' }
export const providerName: Record<string, string> = {
  openai: 'OpenAI', anthropic: 'Anthropic', gemini: 'Google Gemini', antigravity: 'Antigravity', grok: 'xAI Grok',
  deepseek: 'DeepSeek', qwen: 'Qwen', zhipu: 'Z.ai GLM', kimi: 'Kimi', moonshot: 'Kimi', minimax: 'MiniMax', composite: 'Composite',
}
export const providerPortrait: Record<string, string> = {
  openai: 'chatgpt', anthropic: 'claude', gemini: 'gemini', antigravity: 'gemini', grok: 'grok',
  deepseek: 'deepseek', zhipu: 'glm', kimi: 'kimi', moonshot: 'kimi', minimax: 'minimax', qwen: 'mimo', composite: 'chatgpt',
}
