import { apiClient } from './client'

export interface ApiKeyFolder {
  id: number
  user_id: number
  name: string
  sort_order: number
  key_count: number
  created_at: string
  updated_at: string
}

export async function list(): Promise<ApiKeyFolder[]> {
  const { data } = await apiClient.get<ApiKeyFolder[]>('/keys/folders')
  return data || []
}

export async function create(name: string): Promise<ApiKeyFolder> {
  const { data } = await apiClient.post<ApiKeyFolder>('/keys/folders', { name })
  return data
}

export async function update(id: number, name: string): Promise<ApiKeyFolder> {
  const { data } = await apiClient.put<ApiKeyFolder>(`/keys/folders/${id}`, { name })
  return data
}

export async function deleteFolder(id: number): Promise<void> {
  await apiClient.delete(`/keys/folders/${id}`)
}

export const keyFoldersAPI = {
  list,
  create,
  update,
  delete: deleteFolder
}

export default keyFoldersAPI
