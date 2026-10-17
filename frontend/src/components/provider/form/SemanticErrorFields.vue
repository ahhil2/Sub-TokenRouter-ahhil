<template>
  <SettingsSection :title="t('admin.providers.semanticError.section')" data-provider-field="semantic-error">
    <SettingToggleRow
      :id="`${uid}-enabled`"
      v-model="enabled"
      :label="t('admin.providers.semanticError.title')"
      :hint="t('admin.providers.semanticError.hint')"
      testid="semantic-error-enabled"
    />
    <Collapse :open="enabled" unmount-on-hide>
      <SettingsSubpanel>
        <div>
          <label :for="`${uid}-texts`" class="input-label">{{ t('admin.providers.semanticError.texts') }}</label>
          <textarea :id="`${uid}-texts`" v-model="texts" class="input" rows="3" data-testid="semantic-error-texts" />
          <p class="input-hint">{{ t('admin.providers.semanticError.textsHint') }}</p>
        </div>
        <div>
          <label :for="`${uid}-cooldown`" class="input-label">{{ t('admin.providers.semanticError.cooldown') }}</label>
          <input :id="`${uid}-cooldown`" v-model.number="cooldown" class="input" type="number" min="1" max="1440" step="1" data-testid="semantic-error-cooldown" />
        </div>
        <p class="input-hint">{{ t('admin.providers.semanticError.action') }}</p>
        <p class="input-hint">{{ t('admin.providers.semanticError.limits') }}</p>
      </SettingsSubpanel>
    </Collapse>
  </SettingsSection>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Collapse from '@/components/common/Collapse.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import SettingsSection from '@/components/common/settings/SettingsSection.vue'
import SettingsSubpanel from '@/components/common/settings/SettingsSubpanel.vue'
import type { SemanticErrorPolicy } from '@/utils/semanticErrorPolicy'

const policy = defineModel<SemanticErrorPolicy>({ required: true })
const { t } = useI18n()
const uid = useId()
const enabled = computed({
  get: () => policy.value.enabled,
  set: value => { policy.value = { ...policy.value, enabled: value } }
})
const texts = computed({
  get: () => policy.value.texts.join('\n'),
  set: value => { policy.value = { ...policy.value, texts: value.split('\n') } }
})
const cooldown = computed({
  get: () => policy.value.cooldown_minutes,
  set: value => { policy.value = { ...policy.value, cooldown_minutes: value } }
})
</script>
