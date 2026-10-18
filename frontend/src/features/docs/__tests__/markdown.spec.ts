import { describe, it, expect } from 'vitest'
import { documentMarkdown } from '../markdown'
describe('document Markdown boundary', () => {
 it('removes scripts, event handlers and executable embeds', () => {
  const { html } = documentMarkdown('<script>alert(1)</script><iframe src="https://example.com"></iframe><img src="/x.png" onerror="alert(1)"><svg onload="alert(1)"></svg>')
  expect(html).not.toMatch(/script|onerror|onload|iframe|svg/)
 })
 it('rejects unsafe links and remote cleartext images', () => {
  const { html } = documentMarkdown('[unsafe](javascript:alert) ![image](http://example.com/image.png) [safe](https://example.com)')
  expect(html).not.toContain('javascript:')
  expect(html).not.toContain('<img')
  expect(html).toContain('noopener noreferrer')
 })
 it('makes duplicate headings addressable without DOM clobbering', () => {
  const result = documentMarkdown('# 中文\n## 中文\n## constructor')
  expect(result.headings.map(h => h.id)).toEqual(['section-1','section-2','section-3'])
  expect(result.html).toContain('id="section-3"')
 })
 it('retains fenced code and tables', () => {
  const { html } = documentMarkdown('```js\nconst x = 1\n```\n\n| A | B |\n|---|---|\n| 1 | 2 |')
  expect(html).toContain('<pre>'); expect(html).toContain('<table>')
 })
})
