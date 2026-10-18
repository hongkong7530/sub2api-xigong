import { apiClient } from '../client'

export interface GroupCategory {
  id: number
  name: string
  sort_order: number
  created_at: string
  updated_at: string
}

export async function list(): Promise<GroupCategory[]> {
  const { data } = await apiClient.get<GroupCategory[]>('/admin/groups/categories')
  return data || []
}

export async function create(name: string): Promise<GroupCategory> {
  const { data } = await apiClient.post<GroupCategory>('/admin/groups/categories', { name })
  return data
}

export async function update(id: number, name: string): Promise<GroupCategory> {
  const { data } = await apiClient.put<GroupCategory>(`/admin/groups/categories/${id}`, { name })
  return data
}

export async function deleteCategory(id: number): Promise<void> {
  await apiClient.delete(`/admin/groups/categories/${id}`)
}

export const groupCategoriesAPI = {
  list,
  create,
  update,
  delete: deleteCategory
}

export default groupCategoriesAPI
