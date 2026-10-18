import { apiClient } from '../client'

export interface AdminJailbreakTemplate {
  name: string
  description: string
  enabled: boolean
  source: 'custom' | 'builtin'
  system_prompt?: string
  user_prompt?: string
  shadowed?: boolean
}

export interface CreateJailbreakTemplateRequest {
  name: string
  description?: string
  system_prompt?: string
  user_prompt?: string
  enabled?: boolean
}

export interface UpdateJailbreakTemplateRequest {
  description?: string
  system_prompt?: string
  user_prompt?: string
  enabled?: boolean
}

export async function listJailbreakTemplates(): Promise<AdminJailbreakTemplate[]> {
  const { data } = await apiClient.get<AdminJailbreakTemplate[]>('/admin/jailbreak/templates')
  return data
}

export async function createJailbreakTemplate(payload: CreateJailbreakTemplateRequest): Promise<AdminJailbreakTemplate[]> {
  const { data } = await apiClient.post<AdminJailbreakTemplate[]>('/admin/jailbreak/templates', payload)
  return data
}

export async function updateJailbreakTemplate(name: string, payload: UpdateJailbreakTemplateRequest): Promise<AdminJailbreakTemplate[]> {
  const { data } = await apiClient.put<AdminJailbreakTemplate[]>(`/admin/jailbreak/templates/${encodeURIComponent(name)}`, payload)
  return data
}

export async function deleteJailbreakTemplate(name: string): Promise<AdminJailbreakTemplate[]> {
  const { data } = await apiClient.delete<AdminJailbreakTemplate[]>(`/admin/jailbreak/templates/${encodeURIComponent(name)}`)
  return data
}

const jailbreakTemplatesAPI = {
  list: listJailbreakTemplates,
  create: createJailbreakTemplate,
  update: updateJailbreakTemplate,
  remove: deleteJailbreakTemplate
}

export default jailbreakTemplatesAPI
