import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ProviderPriorityCell from '../ProviderPriorityCell.vue'
import type { Provider } from '@/types'

const { update } = vi.hoisted(() => ({ update: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: { providers: { update } },
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const provider = (overrides: Partial<Provider> = {}) => ({
  id: 7,
  name: 'Anthropic provider',
  platform: 'anthropic',
  type: 'apikey',
  proxy_id: null,
  concurrency: 1,
  priority: 3,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...overrides,
}) as Provider

const mountCell = (value = provider()) => mount(ProviderPriorityCell, {
  props: { provider: value },
  global: { stubs: { Icon: true } },
})

beforeEach(() => {
  vi.useFakeTimers()
  update.mockReset().mockImplementation(async (id: number, request: { priority: number }) => provider({ id, priority: request.priority }))
})

afterEach(() => {
  vi.useRealTimers()
})

describe('ProviderPriorityCell', () => {
  it('batches rapid step changes into one priority-only update', async () => {
    const wrapper = mountCell()

    await wrapper.get('[data-testid="provider-priority-increment"]').trigger('click')
    await wrapper.get('[data-testid="provider-priority-increment"]').trigger('click')
    await wrapper.get('[data-testid="provider-priority-decrement"]').trigger('click')

    expect(wrapper.get('[data-testid="provider-priority-value"]').text()).toContain('4')
    expect(update).not.toHaveBeenCalled()

    await vi.runAllTimersAsync()
    await flushPromises()

    expect(update).toHaveBeenCalledTimes(1)
    expect(update).toHaveBeenCalledWith(7, { priority: 4 })
    expect(wrapper.emitted('updated')?.[0]?.[0]).toMatchObject({ id: 7, priority: 4 })
  })

  it('clamps the lower bound to one', async () => {
    const wrapper = mountCell(provider({ priority: 1 }))
    const decrement = wrapper.get('[data-testid="provider-priority-decrement"]')

    expect(decrement.attributes('disabled')).toBeDefined()
    await decrement.trigger('click')
    await vi.runAllTimersAsync()

    expect(update).not.toHaveBeenCalled()
  })

  it('clamps the upper bound to the maximum priority', async () => {
    const wrapper = mountCell(provider({ priority: 2_147_483_647 }))
    const increment = wrapper.get('[data-testid="provider-priority-increment"]')

    expect(increment.attributes('disabled')).toBeDefined()
    await increment.trigger('click')
    await vi.runAllTimersAsync()

    expect(update).not.toHaveBeenCalled()
  })

  it.each(['Enter', 'blur'])('saves the maximum typed priority on %s', async (event) => {
    const wrapper = mountCell()

    await wrapper.get('[data-testid="provider-priority-value"]').trigger('click')
    const input = wrapper.get('[data-testid="provider-priority-input"]')
    await input.setValue('2147483647')
    if (event === 'blur') await input.trigger('blur')
    else await input.trigger('keydown', { key: event })
    await flushPromises()

    expect(update).toHaveBeenCalledWith(7, { priority: 2_147_483_647 })
    expect(wrapper.emitted('error')).toBeUndefined()
  })

  it.each(['0', '-1', '1.5', 'invalid', '2147483648', '9007199254740992'])(
    'rejects invalid typed priority %s before sending a request',
    async (value) => {
      const wrapper = mountCell()

      await wrapper.get('[data-testid="provider-priority-value"]').trigger('click')
      const input = wrapper.get('[data-testid="provider-priority-input"]')
      await input.setValue(value)
      await input.trigger('keydown', { key: 'Enter' })
      await flushPromises()

      expect(update).not.toHaveBeenCalled()
      expect(wrapper.get('[data-testid="provider-priority-value"]').text()).toBe('3')
      expect(wrapper.emitted('error')).toEqual([['admin.providers.priorityInvalid']])
    },
  )

  it('saves typed values and cancels with Escape', async () => {
    const wrapper = mountCell()

    await wrapper.get('[data-testid="provider-priority-value"]').trigger('click')
    const input = wrapper.get('[data-testid="provider-priority-input"]')
    await input.setValue('12')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(update).toHaveBeenCalledWith(7, { priority: 12 })

    update.mockClear()
    await wrapper.get('[data-testid="provider-priority-value"]').trigger('click')
    await wrapper.get('[data-testid="provider-priority-input"]').setValue('50')
    await wrapper.get('[data-testid="provider-priority-input"]').trigger('keydown', { key: 'Escape' })
    await flushPromises()

    expect(update).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="provider-priority-value"]').text()).toContain('3')
  })

  it('reverts and emits an error when saving fails', async () => {
    update.mockRejectedValueOnce(new Error('boom'))
    const wrapper = mountCell()

    await wrapper.get('[data-testid="provider-priority-increment"]').trigger('click')
    await vi.runAllTimersAsync()
    await flushPromises()

    expect(wrapper.get('[data-testid="provider-priority-value"]').text()).toContain('3')
    expect(wrapper.emitted('error')).toHaveLength(1)
    expect(wrapper.emitted('updated')).toBeUndefined()
  })
})
