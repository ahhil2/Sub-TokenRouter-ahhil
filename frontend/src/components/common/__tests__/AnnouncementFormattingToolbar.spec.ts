import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import AnnouncementFormattingToolbar from '../AnnouncementFormattingToolbar.vue'

/**
 * AnnouncementFormattingToolbar.spec.ts exercises announcement formatting through a
 * real textarea selection and emitted model updates. The tests distinguish selected-
 * text emphasis from whole-draft formatting and verify that requested font sizes are
 * constrained to the supported range.
 */

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

const Harness = defineComponent({
  components: { AnnouncementFormattingToolbar },
  setup: () => ({ content: ref('公告 API 123') }),
  template:
    '<div><AnnouncementFormattingToolbar input-id="notice-test" v-model="content" /><textarea id="notice-test" v-model="content" /></div>',
})

describe('AnnouncementFormattingToolbar', () => {
  it('bolds only the selected text', async () => {
    const wrapper = mount(Harness, { attachTo: document.body })
    wrapper.get('textarea').element.setSelectionRange(3, 6)
    await wrapper.get('button').trigger('click')
    expect(wrapper.get('textarea').element.value).toBe('公告 <strong>API</strong> 123')
  })

  it('applies bounded font size to all text with no selection', async () => {
    const wrapper = mount(Harness, { attachTo: document.body })
    wrapper.get('textarea').element.setSelectionRange(0, 0)
    await wrapper.get('input').setValue(99)
    await wrapper.findAll('button')[1].trigger('click')
    expect(wrapper.get('textarea').element.value).toBe(
      '<span style="font-size: 48px">公告 API 123</span>',
    )
    wrapper.get('textarea').element.setSelectionRange(0, 0)
    await wrapper.get('input').setValue(18)
    await wrapper.findAll('button')[1].trigger('click')
    expect(wrapper.get('textarea').element.value).not.toContain('48px')
    expect(wrapper.get('textarea').element.value).toContain('18px')
  })
})
