import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import NewMessageDialog from '../NewMessageDialog.vue'
import MessageComposer from '../MessageComposer.vue'

/**
 * NewMessageDialog.spec.ts checks recipient selection and administrator message
 * submission through mocked API calls. Broadcasts require a separate confirmation,
 * while failed selected-recipient sends retain their draft and reuse the submission
 * identifier when retried.
 */

const api = vi.hoisted(() => ({ contacts: vi.fn(), send: vi.fn() }))
vi.mock('@/api/siteMessages', () => ({ siteMessagesAPI: api }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

beforeEach(() => {
  vi.resetAllMocks()
  vi.stubGlobal('crypto', { randomUUID: () => '019bb611-0872-7000-8000-000000000001' })
  api.contacts.mockResolvedValue({
    items: [{ id: 2, username: 'Alice', email: 'alice@example.invalid' }],
    has_more: false,
  })
  api.send.mockResolvedValue({ recipient_count: 1 })
})
afterEach(() => vi.unstubAllGlobals())

function mountDialog() {
  return shallowMount(NewMessageDialog, {
    global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, Icon: true } },
  })
}

describe('NewMessageDialog', () => {
  it('requires a separate confirmation before broadcasting', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.findAll('input[type="checkbox"]')[0].setValue(true)
    wrapper.getComponent(MessageComposer).vm.$emit('update:modelValue', 'Broadcast 😊')
    await flushPromises()
    wrapper.getComponent(MessageComposer).vm.$emit('submit')
    await flushPromises()
    expect(api.send).not.toHaveBeenCalled()
    const confirm = wrapper
      .findAll('button')
      .find((button) => button.text() === 'messages.confirmSendAll')
    expect(confirm).toBeDefined()
    await confirm!.trigger('click')
    await flushPromises()
    expect(api.send).toHaveBeenCalledWith(
      expect.objectContaining({ all_users: true, recipient_ids: [], body: 'Broadcast 😊' }),
    )
    expect(wrapper.emitted('sent')?.[0]?.[0]).toBe(1)
    wrapper.unmount()
  })

  it('keeps the draft and idempotency key when a selected-recipient send is retried', async () => {
    api.send.mockRejectedValueOnce(new Error('network timeout'))
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.findAll('input[type="checkbox"]')[1].setValue(true)
    wrapper.getComponent(MessageComposer).vm.$emit('update:modelValue', 'Hello Alice 👍')
    await flushPromises()
    wrapper.getComponent(MessageComposer).vm.$emit('submit')
    await flushPromises()
    expect(wrapper.getComponent(MessageComposer).props('modelValue')).toBe('Hello Alice 👍')
    expect(wrapper.text()).toContain('messages.sendFailed')
    wrapper.getComponent(MessageComposer).vm.$emit('submit')
    await flushPromises()
    expect(api.send).toHaveBeenCalledTimes(2)
    expect(api.send.mock.calls[0][0]).toEqual(api.send.mock.calls[1][0])
    expect(api.send.mock.calls[0][0].recipient_ids).toEqual([2])
    wrapper.unmount()
  })
})
