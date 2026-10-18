import { renderSafeMarkdown } from '@/utils/safeMarkdown'
export interface Heading { id: string; title: string; level: number }
/** 编辑预览与公开阅读共用净化及标题 ID，原始 HTML 不获得执行能力。 */
export function documentMarkdown(source: string): { html: string; headings: Heading[] } {
 const root = document.createElement('div')
 root.innerHTML = renderSafeMarkdown(source)
 const headings: Heading[] = []
 root.querySelectorAll('h1,h2,h3').forEach((element, index) => {
  const id = `section-${index + 1}`
  element.id = id
  headings.push({ id, title: element.textContent || '', level: Number(element.tagName[1]) })
 })
 root.querySelectorAll('a').forEach(a => {
  const href = a.getAttribute('href') || ''
  if (/^(https:\/\/|\/[^/]|#)/i.test(href) && !href.includes('\\')) {
   if (href.startsWith('https:')) { a.target = '_blank'; a.rel = 'noopener noreferrer' }
  } else a.removeAttribute('href')
 })
 root.querySelectorAll('img').forEach(img => {
  const src = img.getAttribute('src') || ''
  if (!/^(https:\/\/|\/[^/])/i.test(src) || src.includes('\\')) img.remove()
  else { img.loading = 'lazy'; img.referrerPolicy = 'no-referrer'; img.removeAttribute('srcset') }
 })
 root.querySelectorAll('iframe,video,audio,form,input,button,style,svg,math').forEach(el => el.remove())
 return { html: root.innerHTML, headings }
}
