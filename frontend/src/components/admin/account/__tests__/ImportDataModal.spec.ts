/// <reference path="../../../../vite-env.d.ts" />
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ImportDataModal from '../ImportDataModal.vue'

const { importData, showError } = vi.hoisted(() => ({
  importData: vi.fn(), showError: vi.fn(),
}))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { importData } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showWarning: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

function modal() {
  return mount(ImportDataModal, {
    props: { show: true },
    global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } },
  })
}

async function selectFile(wrapper: ReturnType<typeof modal>, name: string, text: string) {
  const input = wrapper.get('input[type="file"]')
  const file = new File([text], name)
  Object.defineProperty(file, 'text', { value: async () => text })
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
}

beforeEach(() => { vi.clearAllMocks() })

describe('standard account imports', () => {
  it('does not allow YAML to enter the unrelated standard backup importer', async () => {
    const wrapper = modal()
    await selectFile(wrapper, 'accounts.yaml', 'accounts: []')
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')
    expect(importData).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
