import { marked } from 'marked'
import DOMPurify from 'dompurify'

/** Render untrusted Markdown the same way payment help copy does. */
export function renderSafeMarkdown(source: string): string {
  const text = source?.trim() ?? ''
  if (!text) return ''
  const html = marked.parse(text, { async: false, gfm: true, breaks: true }) as string
  return DOMPurify.sanitize(html)
}
