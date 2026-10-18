import { readonly, shallowRef } from 'vue'

export const RESPONSE_NODE_HEADER = 'x-sub2api-app-node'
const responseNode = shallowRef<string | null>(null)

export function normalizeResponseNode(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const id = value.trim().toLowerCase()
  return /^[a-z0-9][a-z0-9_-]{0,63}$/.test(id) ? id : null
}

/** Most recent API response only; it does not identify another request's worker. */
export function recordResponseNode(value: unknown) {
  const id = normalizeResponseNode(value)
  if (id) responseNode.value = id
}

export function useResponseNode() {
  return { responseNode: readonly(responseNode) }
}
