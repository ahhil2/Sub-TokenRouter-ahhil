import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MessageComposer from '../MessageComposer.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: { emoji?: string }) => params?.emoji ? `${key} ${params.emoji}` : key }) }))

describe('MessageComposer', () => {
  it('inserts emoji at the selection and restores the cursor', async () => {
    const wrapper = mount(MessageComposer, { props: { modelValue: 'Hello world' }, global: { stubs: { Icon: true } } })
    const textarea = wrapper.get('textarea').element as HTMLTextAreaElement
    textarea.setSelectionRange(6, 11)
    await wrapper.get('button[type="button"]').trigger('click')
    await wrapper.findAll('button').find(button => button.attributes('aria-label') === 'messages.insertEmoji 😊')!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['Hello 😊'])
    expect(textarea.selectionStart).toBe(8)
    wrapper.unmount()
  })

  it('blocks empty, oversized and pending sends, and accepts Unicode at the limit', async () => {
    const wrapper = mount(MessageComposer, { props: { modelValue: '   ' }, global: { stubs: { Icon: true } } })
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('submit')).toBeUndefined()
    await wrapper.setProps({ modelValue: '😊'.repeat(4001) })
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', ctrlKey: true })
    expect(wrapper.emitted('submit')).toBeUndefined()
    await wrapper.setProps({ modelValue: '😊'.repeat(4000), sending: true })
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('submit')).toBeUndefined()
    await wrapper.setProps({ sending: false })
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', ctrlKey: true })
    expect(wrapper.emitted('submit')).toHaveLength(1)
    wrapper.unmount()
  })
})
