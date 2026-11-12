import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import type { useFeaturesSettings } from './useFeaturesSettings'
import type { SettingsForm } from './settingsState'
import type { CreativeModelSetting } from '@/api/admin/settings'

/**
 * featuresSettingsValidation.ts validates creative worker capacity and image and text models for
 * the bulk settings coordinator. Worker validation publishes an accepted whole count to
 * the draft, while model normalization returns a separate list; null results report errors
 * that stop request construction at the corresponding validation phase.
 */

/**
 * createFeaturesSettingsValidation binds worker-count checks and creative selection errors
 * to the settings page. Create it during setup and use its returned count and model list
 * in the feature serializer only after both validation steps succeed.
 */
export function createFeaturesSettingsValidation(
  form: Pick<
    SettingsForm,
    'creative_worker_count' | 'creative_prompt_group_id' | 'creative_prompt_model'
  >,
  actions: Pick<ReturnType<typeof useFeaturesSettings>, 'normalizeCreativeModelSettingsForSave'>,
) {
  // Validation feedback uses the page locale and toast store, while controller
  // actions operate on the same draft captured by this factory.
  const { t } = useI18n()
  const appStore = useAppStore()
  const { normalizeCreativeModelSettingsForSave } = actions

  /**
   * normalizeWorkerCount floors the worker input and validates a positive safe
   * integer. Accepted input replaces the draft count and is returned for request
   * serialization; invalid input reports the worker-count error and returns null.
   */
  function normalizeWorkerCount(): number | null {
    // Worker capacity is stored as a positive whole count; floor the control
    // value before validating and reflecting that count back into the editable draft.
    const normalizedCreativeWorkerCount = Math.floor(Number(form.creative_worker_count))

    if (
      !Number.isSafeInteger(normalizedCreativeWorkerCount) ||
      normalizedCreativeWorkerCount <= 0
    ) {
      appStore.showError(t('admin.settings.features.creative.workerCountInvalid'))
      return null
    }

    form.creative_worker_count = normalizedCreativeWorkerCount
    return normalizedCreativeWorkerCount
  }

  /**
   * normalizeModelSettings validates the optional prompt expansion route and asks
   * the feature controller to normalize image models and operations. Invalid text
   * or image selections report their corresponding error and return null; valid
   * image rows are returned for serialization while both drafts remain editable.
   */
  function normalizeModelSettings(): CreativeModelSetting[] | null {
    const groupID = Number(form.creative_prompt_group_id) || 0
    const model = form.creative_prompt_model?.trim() ?? ''
    if (
      !Number.isSafeInteger(groupID) ||
      groupID < 0 ||
      (groupID > 0 && (!model || Array.from(model).length > 200 || /[*?]/.test(model)))
    ) {
      appStore.showError(t('admin.settings.features.creative.promptInvalid'))
      return null
    }

    const normalizedCreativeModelSettings = normalizeCreativeModelSettingsForSave()

    if (!normalizedCreativeModelSettings) {
      appStore.showError(t('admin.settings.features.creative.modelSettings.validationError'))
      return null
    }
    return normalizedCreativeModelSettings
  }
  return { normalizeWorkerCount, normalizeModelSettings }
}
