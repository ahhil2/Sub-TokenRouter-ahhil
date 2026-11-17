import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

import TeamView from '../TeamView.vue'

// TeamView.spec.ts checks team key creation and restarting the onboarding guide
// from owner tabs. The production page and creation dialog receive controlled API
// responses, while shell and selector substitutes retain form events, request
// ownership, and the parent list refresh after credential persistence.

// Team, group, credential, notification, and clipboard spies observe ownership
// scope and publication order without persisting any actual credentials.
const {
  currentTeam,
  getMembers,
  getInvitations,
  getTeamKeys,
  startTeamGuide,
  getGroups,
  createKey,
  showError,
  showSuccess,
  copyToClipboard,
} = vi.hoisted(() => ({
  currentTeam: vi.fn(),
  getMembers: vi.fn(),
  getInvitations: vi.fn(),
  getTeamKeys: vi.fn(),
  startTeamGuide: vi.fn(),
  getGroups: vi.fn(),
  createKey: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn(),
}))

vi.mock('@/api/team', () => ({
  teamAPI: {
    current: currentTeam,
    members: getMembers,
    invitations: getInvitations,
    keys: getTeamKeys,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}))

vi.mock('@/api/groups', () => ({ userGroupsAPI: { getAvailable: getGroups } }))
vi.mock('@/api/keys', () => ({ keysAPI: { createWithPayload: createKey } }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard }) }))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({ startTeamGuide }),
}))

vi.mock('@/composables/useBalanceDisplay', () => ({
  useBalanceDisplay: () => ({
    balanceUnitSymbol: '$',
    hasCustomBalanceIcon: false,
  }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn() }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

// owner and teamContext describe an active team with unlimited member windows, enabling
// owner tabs and their guide anchors for both scenarios.
const owner = {
  id: 1,
  team_id: 1,
  user_id: 1,
  email: 'owner@example.com',
  username: 'owner',
  role: 'owner',
  daily_limit_usd: 0,
  weekly_limit_usd: 0,
  monthly_limit_usd: 0,
  daily_usage_usd: 0,
  weekly_usage_usd: 0,
  monthly_usage_usd: 0,
  joined_at: '2026-07-29T00:00:00Z',
  last_active_at: null,
}

const teamContext = {
  team: {
    id: 1,
    name: 'Test team',
    status: 'active',
    member_limit: 10,
    default_daily_limit_usd: 0,
    default_weekly_limit_usd: 0,
    default_monthly_limit_usd: 0,
    member_count: 0,
    created_at: '2026-07-29T00:00:00Z',
    updated_at: '2026-07-29T00:00:00Z',
  },
  membership: owner,
  owner,
}

// Layout stubs render their slots so the mounted production team page owns the entry actions exercised by these tests.
const AppLayoutStub = {
  template: '<div><slot /></div>',
}

// BaseDialogStub retains visible content, footer actions, and the header close
// event while avoiding portal and transition mechanics in form behavior tests.
const BaseDialogStub = {
  props: ['show'],
  emits: ['close'],
  template:
    '<section v-if="show"><button data-testid="dialog-close" @click="$emit(\'close\')">close</button><slot /><slot name="footer" /></section>',
}
// SelectStub turns numeric group choices into actual control changes so the
// production dialog receives the same value type as the shared selector.
const SelectStub = {
  props: ['modelValue', 'options', 'disabled'],
  emits: ['update:modelValue'],
  template: `<select :value="modelValue ?? ''" :disabled="disabled" @change="$emit('update:modelValue', Number($event.target.value))">
    <option value=""></option><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
  </select>`,
}

// mountTeamView renders the production team page and creation dialog with
// slot-preserving shells and a controlled selector. It settles initial context
// and list requests before returning the wrapper for tab navigation.
const mountTeamView = async () => {
  const wrapper = mount(TeamView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        ConfirmDialog: true,
        Icon: true,
        TeamInvitationDialog: true,
        TotpStepUpDialog: true,
        BalanceAmount: true,
        BalanceIcon: true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

// openKeyCreator navigates through the visible owner tab and creation button.
// It settles ready group responses and leaves deliberately deferred lookups
// pending for response-order tests.
const openKeyCreator = async (wrapper: VueWrapper) => {
  await wrapper
    .findAll('nav button')
    .find((button) => button.text() === 'team.keys')!
    .trigger('click')
  await wrapper.get('[data-testid="create-team-key"]').trigger('click')
  await flushPromises()
}

describe('TeamView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentTeam.mockReset().mockResolvedValue(teamContext)
    getMembers.mockReset().mockResolvedValue([owner])
    getInvitations.mockReset().mockResolvedValue([])
    getTeamKeys.mockReset().mockResolvedValue([])
    getGroups.mockReset().mockResolvedValue([{ id: 17, name: 'Team group', description: null }])
    createKey.mockReset().mockResolvedValue({ id: 81, key: 'sk-team-created', name: 'Team client' })
  })

  // An owner starts onboarding while viewing keys or settings. The parameterized test
  // protects restoration of overview anchors and the owner/team context passed to the guide
  // controller.
  it.each(['team.keys', 'team.settings'])('从%s页签启动时先恢复概览页', async (tabLabel) => {
    const wrapper = await mountTeamView()
    const tab = wrapper.findAll('nav button').find((button) => button.text() === tabLabel)
    expect(tab).toBeDefined()

    await tab!.trigger('click')
    expect(wrapper.find('[data-tour="team-members"]').exists()).toBe(false)

    const guideButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('team.guideButton'))
    expect(guideButton).toBeDefined()
    await guideButton!.trigger('click')

    expect(wrapper.find('[data-tour="team-members"]').exists()).toBe(true)
    expect(startTeamGuide).toHaveBeenCalledWith({ isOwner: true, hasTeam: true })
    wrapper.unmount()
  })

  // The owner creates a credential directly from the team key tab.
  // Team scope, automatic billing, secret copying, list refresh, and a clean next draft are observable.
  it('creates and copies a team key from the team panel and refreshes the list', async () => {
    const wrapper = await mountTeamView()
    try {
      expect(getGroups).not.toHaveBeenCalled()
      await openKeyCreator(wrapper)
      expect(getGroups).toHaveBeenCalledWith('team', undefined, false)
      await wrapper.get('#team-key-name').setValue('  Team client  ')
      await wrapper.get('#team-key-group').setValue('17')
      getTeamKeys.mockResolvedValue([
        {
          id: 81,
          user_id: 1,
          user_email: owner.email,
          name: 'Team client',
          masked_key: 'sk-team-***',
          status: 'active',
          team_owner_disabled: false,
          group_id: 17,
          group_name: 'Team group',
          last_used_at: null,
          created_at: '2026-11-17T12:00:00Z',
        },
      ])
      await wrapper.get('#team-key-create-form').trigger('submit')
      await flushPromises()

      expect(createKey).toHaveBeenCalledExactlyOnceWith({
        name: 'Team client',
        scope: 'team',
        group_id: 17,
        billing_mode: 'auto',
      })
      expect(getTeamKeys).toHaveBeenCalledTimes(2)
      expect(wrapper.text()).toContain('sk-team-***')
      expect(wrapper.get('[data-testid="created-team-key"]').text()).toBe('sk-team-created')
      await wrapper
        .findAll('button')
        .find((button) => button.text() === 'common.copy')!
        .trigger('click')
      expect(copyToClipboard).toHaveBeenCalledWith('sk-team-created')

      await wrapper.get('[data-testid="dialog-close"]').trigger('click')
      expect(wrapper.find('[data-testid="created-team-key"]').exists()).toBe(false)
      await wrapper.get('[data-testid="create-team-key"]').trigger('click')
      await flushPromises()
      expect((wrapper.get('#team-key-name').element as HTMLInputElement).value).toBe('')
      expect((wrapper.get('#team-key-group').element as HTMLSelectElement).value).toBe('')
    } finally {
      wrapper.unmount()
    }
  })

  // Creation can finish successfully before the masked list reload fails.
  // The secret remains available, the error identifies retrieval, and the create form stays replaced.
  it('retains the created secret when refreshing the team list fails', async () => {
    const wrapper = await mountTeamView()
    try {
      await openKeyCreator(wrapper)
      await wrapper.get('#team-key-name').setValue('Team client')
      await wrapper.get('#team-key-group').setValue('17')
      getTeamKeys.mockRejectedValueOnce(new Error('list unavailable'))
      await wrapper.get('#team-key-create-form').trigger('submit')
      await flushPromises()
      expect(wrapper.get('[data-testid="created-team-key"]').text()).toBe('sk-team-created')
      expect(wrapper.find('#team-key-create-form').exists()).toBe(false)
      expect(showError).toHaveBeenCalledWith('team.keysRefreshFailed')
      expect(createKey).toHaveBeenCalledTimes(1)
    } finally {
      wrapper.unmount()
    }
  })

  // A pending write receives another submit event and a dismissal request.
  // Both are held until failure arrives, after which the original draft supports a retry.
  it('prevents duplicate creation and preserves the draft after a failed write', async () => {
    let rejectCreation!: (reason: Error) => void
    createKey.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectCreation = reject
        }),
    )
    const wrapper = await mountTeamView()
    try {
      await openKeyCreator(wrapper)
      await wrapper.get('#team-key-name').setValue('Team client')
      await wrapper.get('#team-key-group').setValue('17')
      await wrapper.get('#team-key-create-form').trigger('submit')
      await wrapper.get('#team-key-create-form').trigger('submit')
      await wrapper.get('[data-testid="dialog-close"]').trigger('click')
      expect(wrapper.find('#team-key-create-form').exists()).toBe(true)
      expect(wrapper.get('[form="team-key-create-form"]').attributes('disabled')).toBeDefined()
      expect(createKey).toHaveBeenCalledTimes(1)
      rejectCreation(new Error('quota exceeded'))
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toBe('quota exceeded')
      expect((wrapper.get('#team-key-name').element as HTMLInputElement).value).toBe('Team client')
      expect((wrapper.get('#team-key-group').element as HTMLSelectElement).value).toBe('17')
      expect(getTeamKeys).toHaveBeenCalledTimes(1)

      await wrapper.get('#team-key-create-form').trigger('submit')
      await flushPromises()
      expect(createKey).toHaveBeenCalledTimes(2)
      expect(wrapper.find('[data-testid="created-team-key"]').exists()).toBe(true)
    } finally {
      wrapper.unmount()
    }
  })

  // A failed group lookup leaves no authorized selection to submit.
  // Retrying loads team choices, while missing names and missing groups still prevent writes.
  it('retries unavailable groups and validates the name and group before creation', async () => {
    getGroups.mockRejectedValueOnce(new Error('groups unavailable'))
    const wrapper = await mountTeamView()
    try {
      await openKeyCreator(wrapper)
      expect(wrapper.get('[role="alert"]').text()).toBe('groups unavailable')
      await wrapper.get('#team-key-create-form').trigger('submit')
      expect(createKey).not.toHaveBeenCalled()
      await wrapper
        .findAll('button')
        .find((button) => button.text() === 'common.retry')!
        .trigger('click')
      await flushPromises()
      expect(getGroups).toHaveBeenCalledTimes(2)
      await wrapper.get('#team-key-create-form').trigger('submit')
      expect(wrapper.get('[role="alert"]').text()).toBe('team.keyNameRequired')
      await wrapper.get('#team-key-name').setValue('Team client')
      await wrapper.get('#team-key-create-form').trigger('submit')
      expect(wrapper.get('[role="alert"]').text()).toBe('keys.groupRequired')
      expect(createKey).not.toHaveBeenCalled()
      await wrapper.get('#team-key-group').setValue('17')
      await wrapper.get('#team-key-create-form').trigger('submit')
      await flushPromises()
      expect(wrapper.find('[data-testid="created-team-key"]').exists()).toBe(true)
    } finally {
      wrapper.unmount()
    }
  })

  // The first opening's group response arrives after the dialog has reopened.
  // It must leave the newer opening's available groups and selected group intact.
  it('ignores group responses from a previously closed dialog', async () => {
    let finishEarlier!: (value: unknown) => void
    getGroups.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishEarlier = resolve
        }),
    )
    const wrapper = await mountTeamView()
    try {
      await openKeyCreator(wrapper)
      await wrapper.get('[data-testid="dialog-close"]').trigger('click')
      await wrapper.get('[data-testid="create-team-key"]').trigger('click')
      await flushPromises()
      await wrapper.get('#team-key-group').setValue('17')
      finishEarlier([{ id: 99, name: 'Obsolete group' }])
      await flushPromises()
      expect(wrapper.get('#team-key-group').text()).toContain('Team group')
      expect(wrapper.get('#team-key-group').text()).not.toContain('Obsolete group')
      expect((wrapper.get('#team-key-group').element as HTMLSelectElement).value).toBe('17')
    } finally {
      wrapper.unmount()
    }
  })

  // The server returns no groups available to the team's payer.
  // The form explains the empty selection and cannot submit a key without an authorized group.
  it('keeps creation unavailable when the team has no available groups', async () => {
    getGroups.mockResolvedValue([])
    const wrapper = await mountTeamView()
    try {
      await openKeyCreator(wrapper)
      await wrapper.get('#team-key-name').setValue('Team client')
      expect(wrapper.text()).toContain('common.noGroupsAvailable')
      expect(wrapper.get('[form="team-key-create-form"]').attributes('disabled')).toBeDefined()
      await wrapper.get('#team-key-create-form').trigger('submit')
      expect(createKey).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  })

  // A suspended team's owner can still inspect the management list.
  // Creation remains disabled and opening attempts do not fetch groups or submit credentials.
  it('disables creation while the team is suspended', async () => {
    currentTeam.mockResolvedValue({
      ...teamContext,
      team: { ...teamContext.team, status: 'suspended' },
    })
    const wrapper = await mountTeamView()
    try {
      await wrapper
        .findAll('nav button')
        .find((button) => button.text() === 'team.keys')!
        .trigger('click')
      expect(wrapper.get('[data-testid="create-team-key"]').attributes('disabled')).toBeDefined()
      await wrapper.get('[data-testid="create-team-key"]').trigger('click')
      expect(wrapper.find('#team-key-create-form').exists()).toBe(false)
      expect(getGroups).not.toHaveBeenCalled()
      expect(createKey).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  })

  // Ordinary members do not have the owner's management tab.
  // The new direct-create entry stays within that tab and does not request owner-only key lists.
  it('keeps the team management creation entry scoped to owners', async () => {
    currentTeam.mockResolvedValue({ ...teamContext, membership: { ...owner, role: 'member' } })
    const wrapper = await mountTeamView()
    try {
      expect(wrapper.findAll('nav button').map((button) => button.text())).not.toContain(
        'team.keys',
      )
      expect(wrapper.find('[data-testid="create-team-key"]').exists()).toBe(false)
      expect(getTeamKeys).not.toHaveBeenCalled()
      expect(getGroups).not.toHaveBeenCalled()
    } finally {
      wrapper.unmount()
    }
  })
})
