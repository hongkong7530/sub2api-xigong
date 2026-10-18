import { describe, expect, it } from 'vitest'
import en from '../locales/en/admin/jailbreakTemplates'
import zh from '../locales/zh/admin/jailbreakTemplates'

describe('jailbreak template locale resources', () => {
  it('provides matching management copy in English and Chinese', () => {
    expect(Object.keys(en.jailbreakTemplates).sort()).toEqual(Object.keys(zh.jailbreakTemplates).sort())
    expect(zh.jailbreakTemplates.title).toContain('破甲模板')
    expect(en.jailbreakTemplates.title).toContain('Jailbreak')
  })
})
