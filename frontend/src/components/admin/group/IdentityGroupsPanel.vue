<template>
  <section class="space-y-4">
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <p v-if="loading" class="text-sm text-gray-500">{{ t('identityGroups.loading') }}</p>
    <div class="grid gap-4 md:grid-cols-3">
      <article v-for="group in groups" :key="group.code" class="card space-y-3 p-4">
        <h2 class="font-semibold">{{ group.name }}</h2>
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ description(group.code) }}</p>
        <p class="text-sm">{{ t('identityGroups.count', { count: group.member_count }) }}</p>
        <div class="flex flex-wrap gap-2">
          <button class="btn btn-primary" @click="openMembers(group.code)">{{ t('identityGroups.members') }}</button>
          <button v-if="group.code === 'ahhilai'" class="btn btn-secondary" @click="openRates">{{ t('identityGroups.rates') }}</button>
        </div>
      </article>
    </div>

    <BaseDialog :show="activeCode !== null" :title="activeCode ? identityName(activeCode) : ''" width="wide" @close="closeMembers">
      <div class="space-y-4">
        <p class="text-sm text-gray-500">{{ t('identityGroups.replace') }}</p>
        <p v-if="memberError" role="alert" class="text-sm text-red-600">{{ memberError }}</p>
        <button class="btn btn-secondary" :disabled="busy" @click="toggleMode">{{ t(allUsers ? 'identityGroups.memberMode' : 'identityGroups.addMode') }}</button>
        <form class="flex gap-2" @submit.prevent="searchMembers">
          <input v-model="search" class="input min-w-0 flex-1" :placeholder="t('identityGroups.search')" :aria-label="t('identityGroups.search')" />
          <button class="btn btn-secondary" :disabled="busy">{{ t('identityGroups.searchButton') }}</button>
        </form>
        <p v-if="busy" class="text-sm" aria-live="polite">{{ t('identityGroups.loading') }}</p>
        <ul class="space-y-2">
          <li v-for="member in members" :key="member.user_id" class="flex items-center gap-3 rounded-control border border-gray-200 p-3 dark:border-dark-600">
            <input v-model="selected" type="checkbox" :value="member.user_id" :disabled="busy" :aria-label="`${t('identityGroups.select')} ${member.email}`" />
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm">#{{ member.user_id }} {{ member.username || member.email }}</p>
              <p class="truncate text-xs text-gray-500">{{ member.email }}</p>
              <span v-for="code in member.identity_groups" :key="code" class="badge badge-primary mr-1">{{ identityName(code) }}</span>
            </div>
          </li>
        </ul>
        <p v-if="!busy && members.length === 0" class="text-sm text-gray-500">{{ t('identityGroups.empty') }}</p>
        <div class="flex flex-wrap items-center gap-2">
          <button class="btn btn-secondary" :disabled="busy || page <= 1" @click="changePage(-1)">{{ t('identityGroups.previous') }}</button>
          <span class="text-sm">{{ t('identityGroups.page', { page, total }) }}</span>
          <button class="btn btn-secondary" :disabled="busy || page * 20 >= total" @click="changePage(1)">{{ t('identityGroups.next') }}</button>
        </div>
      </div>
      <template #footer>
        <span class="mr-auto text-sm">{{ t('identityGroups.selected', { count: selected.length }) }}</span>
        <button class="btn btn-primary" :disabled="busy || selected.length === 0" @click="changeMembers(allUsers ? 'add' : 'remove')">{{ t(allUsers ? 'identityGroups.add' : 'identityGroups.remove') }}</button>
      </template>
    </BaseDialog>

    <BaseDialog :show="showRates" :title="`AHHilAI · ${t('identityGroups.rates')}`" width="wide" @close="showRates = false">
      <div class="space-y-4">
        <p class="text-sm text-gray-500">{{ t('identityGroups.rateHelp') }}</p>
        <p v-if="rateError" role="alert" class="text-sm text-red-600">{{ rateError }}</p>
        <button class="btn btn-secondary" :disabled="rateBusy" @click="clearRates">{{ t('identityGroups.clear') }}</button>
        <div v-for="rate in rates" :key="rate.group_id" class="flex flex-wrap items-center gap-3">
          <label :for="`identity-rate-${rate.group_id}`" class="min-w-0 flex-1 text-sm">{{ rate.name }}<span class="block text-xs text-gray-500">{{ t('identityGroups.defaultRate') }}: {{ rate.default_multiplier }}x</span></label>
          <input :id="`identity-rate-${rate.group_id}`" v-model="rateValues[rate.group_id]" type="number" min="0" step="0.00000001" class="input w-32" :disabled="rateBusy" :placeholder="t('identityGroups.inherit')" />
        </div>
      </div>
      <template #footer><button class="btn btn-primary" :disabled="rateBusy || !ratesLoaded" @click="saveRates">{{ t('identityGroups.save') }}</button></template>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { changeIdentityMembers, getIdentityRates, identityName, listIdentityGroups, listIdentityMembers, updateIdentityRates, type IdentityCode, type IdentityGroup, type IdentityMember, type IdentityRate } from '@/api/identityGroups'

const { t } = useI18n()
const groups = ref<IdentityGroup[]>([])
const loading = ref(false)
const error = ref('')
const activeCode = ref<IdentityCode | null>(null)
const allUsers = ref(false)
const search = ref('')
const page = ref(1)
const total = ref(0)
const members = ref<IdentityMember[]>([])
const selected = ref<number[]>([])
const busy = ref(false)
const memberError = ref('')
const showRates = ref(false)
const rates = ref<IdentityRate[]>([])
const rateValues = ref<Record<number, string | number>>({})
const rateBusy = ref(false)
const ratesLoaded = ref(false)
const rateError = ref('')
let memberRequest = 0
const description = (code: IdentityCode) => code === 'ahhilai' ? t('identityGroups.pricing') : t('identityGroups.bonus', { percent: code === 'bonus_5' ? 5 : 10 })
async function loadGroups() {
  loading.value = true
  error.value = ''
  try { groups.value = await listIdentityGroups() } catch { error.value = t('identityGroups.failed') } finally { loading.value = false }
}
async function loadMembers() {
  if (!activeCode.value) return
  const request = ++memberRequest
  busy.value = true
  selected.value = []
  memberError.value = ''
  try {
    const result = await listIdentityMembers(activeCode.value, search.value, page.value, allUsers.value)
    if (request !== memberRequest) return
    members.value = result.items
    total.value = result.total
  } catch { if (request === memberRequest) memberError.value = t('identityGroups.failed') }
  finally { if (request === memberRequest) busy.value = false }
}
function openMembers(code: IdentityCode) { activeCode.value = code; allUsers.value = false; search.value = ''; page.value = 1; members.value = []; void loadMembers() }
function closeMembers() { activeCode.value = null; memberRequest++; busy.value = false }
function toggleMode() { allUsers.value = !allUsers.value; page.value = 1; void loadMembers() }
function searchMembers() { page.value = 1; void loadMembers() }
function changePage(delta: number) { page.value += delta; void loadMembers() }
async function changeMembers(action: 'add' | 'remove') {
  if (!activeCode.value || busy.value) return
  busy.value = true
  memberError.value = ''
  try { await changeIdentityMembers(activeCode.value, selected.value, action); await loadMembers(); await loadGroups() }
  catch { memberError.value = t('identityGroups.failed') }
  finally { busy.value = false }
}
async function openRates() {
  showRates.value = true
  rateBusy.value = true
  ratesLoaded.value = false
  rateError.value = ''
  try {
    rates.value = await getIdentityRates()
    rateValues.value = Object.fromEntries(rates.value.map(rate => [rate.group_id, rate.rate_multiplier ?? '']))
    ratesLoaded.value = true
  } catch { rateError.value = t('identityGroups.failed') } finally { rateBusy.value = false }
}
function clearRates() { for (const rate of rates.value) rateValues.value[rate.group_id] = '' }
async function saveRates() {
  const values = rates.value.map(rate => ({ group_id: rate.group_id, rate_multiplier: rateValues.value[rate.group_id] === '' ? null : Number(rateValues.value[rate.group_id]) }))
  if (values.some(rate => rate.rate_multiplier !== null && (!Number.isFinite(rate.rate_multiplier) || rate.rate_multiplier < 0))) { rateError.value = t('identityGroups.invalid'); return }
  rateBusy.value = true
  rateError.value = ''
  try { await updateIdentityRates(values); showRates.value = false } catch { rateError.value = t('identityGroups.failed') } finally { rateBusy.value = false }
}
onMounted(loadGroups)
</script>
