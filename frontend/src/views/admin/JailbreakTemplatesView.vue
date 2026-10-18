<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="min-w-0 flex-1">
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.jailbreakTemplates.title') }}
            </h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.jailbreakTemplates.description') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center justify-end gap-2">
            <button
              class="btn btn-secondary"
              :disabled="loading"
              :title="t('common.refresh')"
              @click="loadTemplates"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="openCreateFromScratch">
              <Icon name="plus" size="md" class="mr-1" />
              {{ t('admin.jailbreakTemplates.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <div class="flex-1 overflow-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-900/40">
              <tr>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t('admin.jailbreakTemplates.name') }}
                </th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t('admin.jailbreakTemplates.descriptionLabel') }}
                </th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t('admin.jailbreakTemplates.source') }}
                </th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t('admin.jailbreakTemplates.status') }}
                </th>
                <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t('common.actions') }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr v-if="!loading && templates.length === 0">
                <td colspan="5" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.jailbreakTemplates.empty') }}
                </td>
              </tr>
              <tr
                v-for="tpl in templates"
                :key="`${tpl.source}:${tpl.name}`"
                class="transition-colors hover:bg-gray-50 dark:hover:bg-dark-800/60"
              >
                <td class="px-4 py-3">
                  <div class="flex flex-wrap items-center gap-2">
                    <code class="font-mono text-sm text-gray-900 dark:text-gray-100">{{ tpl.name }}</code>
                    <span
                      v-if="overridesBuiltin(tpl)"
                      class="inline-flex items-center rounded-md bg-purple-100 px-1.5 py-0.5 text-[11px] font-medium text-purple-700 dark:bg-purple-900/30 dark:text-purple-400"
                    >
                      {{ t('admin.jailbreakTemplates.overridesBuiltin') }}
                    </span>
                    <span
                      v-else-if="tpl.source === 'builtin' && tpl.shadowed"
                      class="inline-flex items-center rounded-md bg-gray-100 px-1.5 py-0.5 text-[11px] font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-400"
                    >
                      {{ t('admin.jailbreakTemplates.shadowedByCustom') }}
                    </span>
                  </div>
                </td>
                <td class="max-w-md px-4 py-3 text-sm text-gray-600 dark:text-gray-300">
                  <span class="line-clamp-2">{{ tpl.description || '—' }}</span>
                </td>
                <td class="px-4 py-3">
                  <span
                    :class="[
                      'inline-flex items-center rounded-md px-1.5 py-0.5 text-[11px] font-medium',
                      tpl.source === 'custom'
                        ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                        : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
                    ]"
                  >
                    {{ tpl.source === 'custom' ? t('admin.jailbreakTemplates.sourceCustom') : t('admin.jailbreakTemplates.sourceBuiltin') }}
                  </span>
                </td>
                <td class="px-4 py-3">
                  <span
                    :class="[
                      'inline-flex items-center rounded-md px-1.5 py-0.5 text-[11px] font-medium',
                      tpl.enabled
                        ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                        : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
                    ]"
                  >
                    {{ tpl.enabled ? t('admin.jailbreakTemplates.enabled') : t('admin.jailbreakTemplates.disabled') }}
                  </span>
                </td>
                <td class="px-4 py-3 text-right">
                  <div class="inline-flex items-center gap-3">
                    <button
                      v-if="tpl.source === 'custom'"
                      class="text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                      @click="openEdit(tpl)"
                    >
                      {{ t('admin.jailbreakTemplates.edit') }}
                    </button>
                    <button
                      v-else
                      class="text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                      @click="openCreateFromBuiltin(tpl)"
                    >
                      {{ t('admin.jailbreakTemplates.overrideBuiltin') }}
                    </button>
                    <button
                      v-if="tpl.source === 'custom'"
                      class="text-sm font-medium text-gray-600 hover:text-gray-700 dark:text-gray-300"
                      @click="toggleEnabled(tpl)"
                    >
                      {{ tpl.enabled ? t('admin.jailbreakTemplates.disable') : t('admin.jailbreakTemplates.enable') }}
                    </button>
                    <button
                      v-if="tpl.source === 'custom'"
                      class="text-sm font-medium text-red-600 hover:text-red-700 dark:text-red-400"
                      @click="askDelete(tpl)"
                    >
                      {{ t('admin.jailbreakTemplates.delete') }}
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </TablePageLayout>

    <BaseDialog :show="showDialog" :title="dialogTitle" @close="closeDialog">
      <form id="jailbreak-template-form" class="space-y-4" @submit.prevent="saveTemplate">
        <div>
          <label class="input-label">{{ t('admin.jailbreakTemplates.name') }}</label>
          <input
            v-model="form.name"
            type="text"
            class="input"
            :disabled="editingName !== null"
            :placeholder="t('admin.jailbreakTemplates.namePlaceholder')"
          />
          <p class="input-hint">{{ t('admin.jailbreakTemplates.nameHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.jailbreakTemplates.descriptionLabel') }}</label>
          <input
            v-model="form.description"
            type="text"
            class="input"
            :placeholder="t('admin.jailbreakTemplates.descriptionPlaceholder')"
          />
        </div>
        <div class="flex items-center justify-between">
          <label class="input-label mb-0">{{ t('admin.jailbreakTemplates.enabledLabel') }}</label>
          <Toggle v-model="form.enabled" />
        </div>
        <TextArea
          v-model="form.system_prompt"
          :label="t('admin.jailbreakTemplates.systemPrompt')"
          :placeholder="t('admin.jailbreakTemplates.systemPromptPlaceholder')"
          :hint="t('admin.jailbreakTemplates.systemPromptHint')"
          :rows="12"
        />
        <TextArea
          v-model="form.user_prompt"
          :label="t('admin.jailbreakTemplates.userPrompt')"
          :placeholder="t('admin.jailbreakTemplates.userPromptPlaceholder')"
          :hint="t('admin.jailbreakTemplates.userPromptHint')"
          :rows="4"
        />
        <p class="input-hint">{{ t('admin.jailbreakTemplates.maxHint') }}</p>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeDialog">{{ t('common.cancel') }}</button>
          <button
            form="jailbreak-template-form"
            type="submit"
            class="btn btn-primary"
            :disabled="saving"
          >
            {{ saving ? t('admin.jailbreakTemplates.saving') : t('admin.jailbreakTemplates.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.jailbreakTemplates.delete')"
      :message="t('admin.jailbreakTemplates.deleteConfirm', { name: deleting?.name })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api'
import type { AdminJailbreakTemplate } from '@/api/admin/jailbreakTemplates'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import TextArea from '@/components/common/TextArea.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const templates = ref<AdminJailbreakTemplate[]>([])
const loading = ref(false)
const saving = ref(false)
const showDialog = ref(false)
const showDeleteDialog = ref(false)
const editingName = ref<string | null>(null)
const deleting = ref<AdminJailbreakTemplate | null>(null)
const form = ref({ name: '', description: '', enabled: true, system_prompt: '', user_prompt: '' })
const builtinNames = computed(() => new Set(templates.value.filter(item => item.source === 'builtin').map(item => item.name)))
const dialogTitle = computed(() => editingName.value ? t('admin.jailbreakTemplates.editTitle') : t('admin.jailbreakTemplates.createTitle'))
const overridesBuiltin = (tpl: AdminJailbreakTemplate): boolean => tpl.source === 'custom' && builtinNames.value.has(tpl.name)

const loadTemplates = async () => {
  loading.value = true
  try { templates.value = await adminAPI.jailbreakTemplates.list() }
  catch (error: any) { appStore.showError(error?.message || t('admin.jailbreakTemplates.loadFailed')) }
  finally { loading.value = false }
}
const openCreateFromScratch = () => { editingName.value = null; form.value = { name: '', description: '', enabled: true, system_prompt: '', user_prompt: '' }; showDialog.value = true }
const openCreateFromBuiltin = (tpl: AdminJailbreakTemplate) => { editingName.value = null; form.value = { name: tpl.name, description: tpl.description || '', enabled: true, system_prompt: tpl.system_prompt || '', user_prompt: tpl.user_prompt || '' }; showDialog.value = true }
const openEdit = (tpl: AdminJailbreakTemplate) => { editingName.value = tpl.name; form.value = { name: tpl.name, description: tpl.description || '', enabled: tpl.enabled, system_prompt: tpl.system_prompt || '', user_prompt: tpl.user_prompt || '' }; showDialog.value = true }
const closeDialog = () => { showDialog.value = false }
const saveTemplate = async () => {
  const name = form.value.name.trim()
  if (!/^[A-Za-z0-9_-]{1,50}$/.test(name)) { appStore.showError(t('admin.jailbreakTemplates.nameInvalid')); return }
  if (!form.value.system_prompt.trim() && !form.value.user_prompt.trim()) { appStore.showError(t('admin.jailbreakTemplates.promptsRequired')); return }
  saving.value = true
  try {
    if (editingName.value) {
      templates.value = await adminAPI.jailbreakTemplates.update(editingName.value, { description: form.value.description, system_prompt: form.value.system_prompt, user_prompt: form.value.user_prompt, enabled: form.value.enabled })
      appStore.showSuccess(t('admin.jailbreakTemplates.updated'))
    } else {
      templates.value = await adminAPI.jailbreakTemplates.create({ name, description: form.value.description, system_prompt: form.value.system_prompt, user_prompt: form.value.user_prompt, enabled: form.value.enabled })
      appStore.showSuccess(t('admin.jailbreakTemplates.created'))
    }
    showDialog.value = false
  } catch (error: any) { appStore.showError(error?.message || t('admin.jailbreakTemplates.saveFailed')) }
  finally { saving.value = false }
}
const toggleEnabled = async (tpl: AdminJailbreakTemplate) => {
  try { templates.value = await adminAPI.jailbreakTemplates.update(tpl.name, { enabled: !tpl.enabled }); appStore.showSuccess(t('admin.jailbreakTemplates.updated')) }
  catch (error: any) { appStore.showError(error?.message || t('admin.jailbreakTemplates.saveFailed')) }
}
const askDelete = (tpl: AdminJailbreakTemplate) => { deleting.value = tpl; showDeleteDialog.value = true }
const confirmDelete = async () => {
  if (!deleting.value) return
  try { templates.value = await adminAPI.jailbreakTemplates.remove(deleting.value.name); showDeleteDialog.value = false; appStore.showSuccess(t('admin.jailbreakTemplates.deleted')) }
  catch (error: any) { appStore.showError(error?.message || t('admin.jailbreakTemplates.saveFailed')) }
}
onMounted(loadTemplates)
</script>
