import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const frontendRoot = resolve(__dirname, '../../../..')
const stripeConsumers = [
  'src/views/user/StripePaymentView.vue',
  'src/views/user/StripePopupView.vue',
  'src/components/payment/StripePaymentInline.vue',
]

function readFrontendFile(path: string): string {
  return readFileSync(resolve(frontendRoot, path), 'utf8')
}

describe('payment SDK lazy-loading contract', () => {
  it('keeps Airwallex side effects out of the shared vendor chunk', () => {
    const source = readFrontendFile('src/views/user/AirwallexPaymentView.vue')
    expect(source).toContain("await import('@airwallex/components-sdk')")
    const config = readFrontendFile('vite.config.ts')
    const sdkRule = config.indexOf("id.includes('/@airwallex/components-sdk/')")
    const fallback = config.indexOf("return 'vendor-misc'")
    expect(sdkRule).toBeGreaterThan(-1)
    expect(sdkRule).toBeLessThan(fallback)
    expect(config.slice(sdkRule, fallback)).toContain("return 'vendor-airwallex'")
  })

  it.each(stripeConsumers)('%s uses the side-effect-free Stripe loader', (path) => {
    const source = readFrontendFile(path)

    expect(source).toContain("await import('@stripe/stripe-js/pure')")
    expect(source).not.toMatch(/await import\(['"]@stripe\/stripe-js['"]\)/)
  })

  it('keeps Stripe out of the shared vendor chunk', () => {
    const viteConfig = readFrontendFile('vite.config.ts')
    const stripeRule = viteConfig.indexOf("id.includes('/@stripe/stripe-js/')")
    const miscFallback = viteConfig.indexOf("return 'vendor-misc'")

    expect(stripeRule).toBeGreaterThan(-1)
    expect(viteConfig.slice(stripeRule, miscFallback)).toContain("return 'vendor-stripe'")
    expect(stripeRule).toBeLessThan(miscFallback)
  })
})
