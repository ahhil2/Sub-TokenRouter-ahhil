import { defineComponent, h, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCreativePrompt } from '@/composables/useCreativePrompt'
import type { CreativeCapabilities, CreativeExpandedPrompt } from '@/api/creative'

/**
 * useCreativePrompt.spec.ts exercises paid expansion and free restoration with
 * controlled API outcomes. Mounted composables expose request identity,
 * concurrent clicks, failed retries, and delayed responses while the real
 * reactive prompt records whether user edits survive asynchronous completion.
 */

// API and account fixtures isolate expansion calls from unrelated browser
// storage and wallet requests while retaining observable request arguments.
const { expand, refreshUser, workspace } = vi.hoisted(() => ({
  expand: vi.fn(),
  refreshUser: vi.fn(),
  workspace: { id: '11111111-1111-4111-8111-111111111111' },
}))
vi.mock('@/api/creative', () => ({ expandCreativePrompt: expand }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 1 }, refreshUser }) }))
vi.mock('@/utils/creativeLocalStore', () => ({
  getCreativeWorkspaceId: () => workspace.id,
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

/**
 * setupPrompt mounts the production composable around an editable prompt.
 * Each caller unmounts the returned component so cancellation and watch cleanup
 * run through Vue's component lifecycle.
 */
function setupPrompt() {
  const prompt = ref('一只猫')
  const capabilities = ref<CreativeCapabilities>({
    max_prompt_chars: 8000,
    max_asset_bytes: 1,
    max_total_input_bytes: 1,
    max_mask_bytes: 1,
    allowed_mime_types: [],
    prompt_expansion_enabled: true,
  })
  let actions!: ReturnType<typeof useCreativePrompt>
  const wrapper = mount(
    defineComponent({
      /** setup connects the fixture prompt with production request lifecycle handling. */
      setup() {
        actions = useCreativePrompt(prompt, capabilities)
        return () => h('div')
      },
    }),
  )
  return { prompt, capabilities, actions, wrapper }
}

beforeEach(() => {
  vi.clearAllMocks()
  workspace.id = '11111111-1111-4111-8111-111111111111'
  refreshUser.mockResolvedValue({ id: 1 })
})

describe('creative prompt expansion', () => {
  /**
   * Consecutive successful expansions retain the first input for restoration.
   * Undo restores that input locally and sends no additional paid API request.
   */
  it('restores the first prompt for free after repeated expansion', async () => {
    const { prompt, actions, wrapper } = setupPrompt()
    try {
      expand.mockResolvedValueOnce({ prompt: '窗边的一只猫', cost_cny: 0.015 })
      await actions.expand()
      expect(prompt.value).toBe('窗边的一只猫')
      expand.mockResolvedValueOnce({ prompt: '阳光下窗边的一只猫', cost_cny: 0.015 })
      await actions.expand()
      expect(expand.mock.calls[0][2]).not.toBe(expand.mock.calls[1][2])
      actions.restore()
      expect(prompt.value).toBe('一只猫')
      expect(expand).toHaveBeenCalledTimes(2)
      expect(actions.canRestore.value).toBe(false)
    } finally {
      wrapper.unmount()
    }
  })

  /**
   * A dropped response can follow a successful server charge. Retrying unchanged
   * text uses the same operation key and leaves the input intact after failure.
   */
  it('retries an unresolved request with the same billing identity', async () => {
    const { prompt, actions, wrapper } = setupPrompt()
    try {
      expand.mockRejectedValueOnce(new Error('network interrupted'))
      await actions.expand()
      expect(prompt.value).toBe('一只猫')
      expect(actions.canRestore.value).toBe(false)
      expand.mockResolvedValueOnce({ prompt: '窗边的一只猫', cost_cny: 0.015 })
      await actions.expand()
      expect(expand.mock.calls[0][2]).toBe(expand.mock.calls[1][2])
      expect(actions.error.value).toBe('')
    } finally {
      wrapper.unmount()
    }
  })

  /**
   * External prompt edits can arrive while expansion is pending. Even a change
   * back to the submitted text invalidates publication and concurrent clicks.
   */
  it('blocks duplicate clicks and keeps edits made during the request', async () => {
    const { prompt, actions, wrapper } = setupPrompt()
    let resolve!: (value: CreativeExpandedPrompt) => void
    expand.mockReturnValue(
      new Promise<CreativeExpandedPrompt>((done) => {
        resolve = done
      }),
    )
    try {
      const pending = actions.expand()
      await actions.expand()
      expect(expand).toHaveBeenCalledTimes(1)
      prompt.value = '一只狗'
      prompt.value = '一只猫'
      resolve({ prompt: '窗边的一只猫', cost_cny: 0.015 })
      await pending
      expect(prompt.value).toBe('一只猫')
      expect(actions.error.value).toBe('creative.prompt.changed')
      expect(actions.canRestore.value).toBe(false)
    } finally {
      wrapper.unmount()
    }
  })

  /**
   * Clearing local creative data rotates the browser workspace while the composer
   * remains mounted. Restoration must discard the old prompt instead of repopulating
   * the cleared workspace with text from a previous expansion.
   */
  it('discards restoration when the workspace changes', async () => {
    const { prompt, actions, wrapper } = setupPrompt()
    try {
      expand.mockResolvedValueOnce({ prompt: '窗边的一只猫', cost_cny: 0.015 })
      await actions.expand()
      expect(actions.canRestore.value).toBe(true)
      workspace.id = '22222222-2222-4222-8222-222222222222'
      prompt.value = ''
      expect(actions.canRestore.value).toBe(false)
      actions.restore()
      expect(prompt.value).toBe('')
    } finally {
      wrapper.unmount()
    }
  })

  /**
   * Leaving the composer cancels its request and discards late completion.
   * The response cannot mutate a draft belonging to an unmounted component.
   */
  it('aborts on unmount and ignores a late successful response', async () => {
    const { prompt, actions, wrapper } = setupPrompt()
    let resolve!: (value: CreativeExpandedPrompt) => void
    expand.mockReturnValue(
      new Promise<CreativeExpandedPrompt>((done) => {
        resolve = done
      }),
    )
    const pending = actions.expand()
    const signal = expand.mock.calls[0][3] as AbortSignal
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    resolve({ prompt: '窗边的一只猫', cost_cny: 0.015 })
    await pending
    expect(prompt.value).toBe('一只猫')
  })
})
