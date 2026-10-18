/** 只接受站内路径或无用户信息的HTTPS图片地址。 */
export function siteImageUrl(value: string): string {
 if (!value || value.length > 2048 || /[\\\r\n\t]/.test(value)) return ''
 if (value.startsWith('/') && !value.startsWith('//')) return value
 try { const url = new URL(value); return url.protocol === 'https:' && !!url.hostname && !url.username && !url.password ? value : '' } catch { return '' }
}
