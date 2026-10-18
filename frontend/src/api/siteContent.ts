import { apiClient } from './client'
import type { UserMonitorView } from './channelMonitor'
export interface PublicMonitorList { enabled: boolean; items: Omit<UserMonitorView, 'latest_quota'>[]; sampled_at: string; refresh_seconds: number }
export interface ConsoleAppearance {
 enabled: boolean; background_mode: 'wallpaper' | 'custom' | 'none'; wallpaper_id: string; background_url: string
 mobile_background_url?: string; mobile_focus_x?: number; mobile_focus_y?: number
 focus_x: number; focus_y: number; surface_mode: 'liquid' | 'frosted'; surface_opacity: number; blur: number
 background_dim: number; pointer_reveal: boolean; colorful_headings: boolean; flow_enabled: boolean
}
export const defaultConsoleAppearance = (): ConsoleAppearance => ({ enabled: true, background_mode: 'wallpaper', wallpaper_id: 'scene-1', background_url: '', focus_x: 70, focus_y: 45, surface_mode: 'liquid', surface_opacity: .72, blur: 12, background_dim: .18, pointer_reveal: true, colorful_headings: true, flow_enabled: true })
export interface Wallpaper { id: string; focus_x: number; focus_y: number; desktop_url?: string; mobile_url?: string; mobile_focus_x?: number; mobile_focus_y?: number }
export interface Appearance { version: number; home_mode: 'immersive' | 'legacy'; docs_enabled: boolean; partners_enabled: boolean; motion_enabled: boolean; intensity: number; interval: number; phrases: string[]; quotes: { en: string; zh: string }[]; wallpapers: Wallpaper[]; console?: ConsoleAppearance }
export interface SiteStats { usage_stale?: boolean; usage: {sampled_at:string;day:string;requests:number;today_tokens:number;average_per_million:number|null;price_samples:number;models:{model:string;tokens:number;requests:number}[]}|null;online_users:number|null;in_flight:number|null;sampled_at:string;online_window_seconds:number;refresh_seconds:number }
export interface Category { id: number; name: string; position: number; version?: number }
export interface Partner { id: number; name: string; description: string; url: string; icon: string; thumbnail?: string; tag: string; enabled: boolean; position: number; version: number }
export interface DocumentBody { title: string; summary: string; content: string; category_id: number; position: number }
export interface SiteDocument { id: number; slug: string; draft: DocumentBody; published: DocumentBody | null; version: number; updated_at: string; published_at: string | null }
export interface PublicDocument extends DocumentBody { id: number; slug: string; updated_at: string }
export interface DocumentIndex { page: number; has_more: boolean; categories: Category[]; documents: Omit<PublicDocument, 'content'>[] }
export const defaultAppearance = (): Appearance => ({ console: defaultConsoleAppearance(), version: 0, home_mode: 'immersive', docs_enabled: true, partners_enabled: true, motion_enabled: true, intensity: 1, interval: 5, phrases: ['xigong.cloud', '西宫我们爱你', '我最喜欢的中转站', '好像和西宫在一起'], quotes: [{ en: 'Stay curious.', zh: '别被默认值定义。' }], wallpapers: [70, 25, 45, 50, 25, 50].map((x, i) => ({ id: `scene-${i + 1}`, focus_x: x, focus_y: i === 3 ? 28 : 45 })) })
// 旧服务与部分配置响应在入口补齐，不把预览草稿写进全站状态。
export function normalizeAppearance(value: Appearance): Appearance {
 return { ...value, console: { ...defaultConsoleAppearance(), ...value.console } }
}
const base = '/site-content'
const admin = '/admin/site-content'
export const siteContent = {
 appearance: async () => normalizeAppearance((await apiClient.get<Appearance>(`${base}/appearance`)).data),
 uploadAsset: async (file: File) => { const form = new FormData(); form.append("file", file); return (await apiClient.post<{url:string}>(`${admin}/assets`, form, { headers: { "Content-Type": "multipart/form-data" } })).data },
 resolvePartner: async (url: string, signal?: AbortSignal) => (await apiClient.post<Partial<Partner> & {warnings:string[]}>(`${admin}/partners/resolve`, {url}, {signal,timeout:35000})).data,
 quote: async (signal?:AbortSignal) => (await apiClient.get<{zh:string;en:string;source?:string}|null>(`${base}/quote`,{signal,timeout:6000})).data,
 monitors: async (signal?: AbortSignal) => (await apiClient.get<PublicMonitorList>(`${base}/channel-monitors`, { signal })).data,
 stats: async (signal?: AbortSignal) => (await apiClient.get<SiteStats>(`${base}/stats`, {signal})).data,
 presence: async () => apiClient.post(`${base}/presence`),
 partners: async () => (await apiClient.get<Partner[]>(`${base}/partners`)).data,
 docs: async (q = '', signal?: AbortSignal, page = 1) => (await apiClient.get<DocumentIndex>(`${base}/docs`, { params: { q, page }, signal })).data,
 document: async (slug: string, signal?: AbortSignal) => (await apiClient.get<PublicDocument>(`${base}/docs/${encodeURIComponent(slug)}`, { signal })).data,
 adminAppearance: async () => normalizeAppearance((await apiClient.get<Appearance>(`${admin}/appearance`)).data),
 saveAppearance: async (a: Appearance) => normalizeAppearance((await apiClient.put<Appearance>(`${admin}/appearance`, a)).data),
 adminDocs: async (page = 1, q = '') => (await apiClient.get<{ documents: SiteDocument[]; page: number; has_more: boolean }>(`${admin}/docs`, { params: { page, q } })).data,
 adminDoc: async (id: number) => (await apiClient.get<SiteDocument>(`${admin}/docs/${id}`)).data,
 saveDoc: async (d: SiteDocument) => (await apiClient.request<SiteDocument>({ method: d.id ? 'PUT' : 'POST', url: `${admin}/docs${d.id ? `/${d.id}` : ''}`, data: { slug: d.slug, draft: d.draft, version: d.version } })).data,
 docAction: async (d: SiteDocument, action: 'publish' | 'unpublish' | 'delete') => apiClient.post(`${admin}/docs/${d.id}/${action}`, { version: d.version }),
 categories: async () => (await apiClient.get<Category[]>(`${admin}/categories`)).data,
 saveCategory: async (c: Category) => (await apiClient.request<Category>({ method: c.id ? 'PUT' : 'POST', url: `${admin}/categories${c.id ? `/${c.id}` : ''}`, data: c })).data,
 deleteCategory: async (c: Category) => apiClient.delete(`${admin}/categories/${c.id}`, { data: { version: c.version } }),
 adminPartners: async () => (await apiClient.get<Partner[]>(`${admin}/partners`)).data,
 savePartner: async (p: Partner) => (await apiClient.request<Partner>({ method: p.id ? 'PUT' : 'POST', url: `${admin}/partners${p.id ? `/${p.id}` : ''}`, data: p })).data,
 deletePartner: async (p: Partner) => apiClient.delete(`${admin}/partners/${p.id}`, { data: { version: p.version } }),
}
