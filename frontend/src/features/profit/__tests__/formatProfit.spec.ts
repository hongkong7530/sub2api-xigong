import { describe, expect, it } from 'vitest'
import { profitMoney, profitPercent, metricProfit, negativeMoney } from '../formatProfit'

describe('profit decimal presentation', () => {
  it('formats decimal money without binary precision loss', () => {
    expect(profitMoney('9007199254740993.125')).toBe('$9,007,199,254,740,993.13')
    expect(profitMoney('0.00000001', 8)).toBe('$0.00000001')
    expect(profitMoney('-0.005')).toBe('−$0.01')
    expect(profitMoney('-0.00000001')).toBe('$0.00')
    expect(profitMoney('999.999')).toBe('$1,000.00')
  })
  it.each([null, undefined, '', 'NaN', '1e2'])('keeps invalid/unknown %s distinct from zero', value => { expect(profitMoney(value)).toBe('—') })
  it('keeps null margins and unknown costs unknown', () => {
    expect(profitPercent(null)).toBe('—')
    expect(profitPercent('0.33333333')).toContain('33.3')
    expect(metricProfit({ requests: 1, known_requests: 0, profit: '0', cost: '0', revenue: '1', known_revenue: '0', margin: null, coverage: '0' })).toBe('—')
    expect(negativeMoney('-1.0')).toBe(true)
    expect(negativeMoney('-0.00000001')).toBe(true)
    expect(negativeMoney('-0.00000000')).toBe(false)
  })
})
