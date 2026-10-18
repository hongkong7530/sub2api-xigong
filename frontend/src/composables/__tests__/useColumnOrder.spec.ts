import { computed, effectScope, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { normalizeColumnOrder, useColumnOrder } from '../useColumnOrder'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.restoreAllMocks() })

const setup = (key = 'test-column-order') => {
  const columns = ref(['model', 'group', 'created_at'].map(key => ({ key, label: key, sortable: key === 'created_at' })))
  const storageKey = ref(key)
  const scope = effectScope()
  scopes.push(scope)
  const state = scope.run(() => useColumnOrder(computed(() => columns.value), storageKey))!
  return { ...state, columns, storageKey }
}

describe('column order', () => {
  it('filters stale/non-string/duplicate preferences and appends new columns', () => {
    expect(normalizeColumnOrder(['created_at', 'gone', 42, 'created_at'], ['model', 'group', 'created_at'])).toEqual(['created_at', 'model', 'group'])
    expect(normalizeColumnOrder({ model: 0 }, ['model', 'group'])).toEqual(['model', 'group'])
  })
  it('loads saved order while retaining the actual column metadata', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockReturnValue('["created_at","model"]')
    const state = setup()
    expect(state.orderedColumns.value.map(c => c.key)).toEqual(['created_at', 'model', 'group'])
    expect(state.orderedColumns.value[0].sortable).toBe(true)
  })
  it('moves in both directions and persists without rewriting hidden preferences', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockReturnValue(null)
    const save = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {})
    const state = setup()
    state.moveColumn('created_at', 'model')
    expect(state.orderedColumns.value.map(c => c.key)).toEqual(['created_at', 'model', 'group'])
    state.shiftColumn('created_at', 1)
    expect(state.orderedColumns.value.map(c => c.key)).toEqual(['model', 'created_at', 'group'])
    expect(save).toHaveBeenLastCalledWith('test-column-order', '["model","created_at","group"]')
  })
  it('ignores invalid/boundary moves and resets only the order', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockReturnValue(null)
    const state = setup()
    state.shiftColumn('model', -1)
    state.moveColumn('missing', 'model')
    expect(state.orderedColumns.value.map(c => c.key)).toEqual(['model', 'group', 'created_at'])
    state.moveColumn('model', 'created_at')
    state.resetColumnOrder()
    expect(state.orderedColumns.value.map(c => c.key)).toEqual(['model', 'group', 'created_at'])
  })
  it('handles corrupt or unavailable browser storage without breaking the table', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockReturnValue('{broken')
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked') })
    const state = setup()
    expect(() => state.moveColumn('group', 'model')).not.toThrow()
    expect(state.orderedColumns.value[0].key).toBe('group')
  })
  it('keeps added columns and reactive labels while removing unavailable keys', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockReturnValue('["created_at","model"]')
    const state = setup()
    state.columns.value = [{ key: 'model', label: '模型', sortable: false }, { key: 'cost', label: '费用', sortable: false }]
    await nextTick()
    expect(state.orderedColumns.value.map(c => c.key)).toEqual(['model', 'cost'])
    expect(state.orderedColumns.value[0].label).toBe('模型')
  })
  it('keeps separate view preference keys isolated', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(key => key === 'admin' ? '["group","model"]' : '["created_at"]')
    const state = setup('admin')
    expect(state.orderedColumns.value[0].key).toBe('group')
    state.storageKey.value = 'user'
    await nextTick()
    expect(state.orderedColumns.value[0].key).toBe('created_at')
  })
})
