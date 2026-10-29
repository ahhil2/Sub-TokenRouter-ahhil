import { afterEach, describe, expect, it } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import PelicanPreview from '../PelicanPreview.vue'

/**
 * PelicanPreview.spec.ts checks generated documents inside the drawing preview frame.
 * The component tests inspect sandbox configuration, retained interactive controls,
 * gallery thumbnail pointer behavior, and empty-document handling without invoking an
 * actual model drawing provider.
 */

enableAutoUnmount(afterEach)

describe('PelicanPreview', () => {
  const html =
    '<html><body><svg></svg><button id="pause">暂停</button><script>document.getElementById("pause").addEventListener("click", () => {})</script></body></html>'

  it('renders generated HTML in an isolated iframe with working controls retained', () => {
    const wrapper = mount(PelicanPreview, { props: { html, title: '骑车动画', interactive: true } })
    const frame = wrapper.get('iframe')
    expect(frame.attributes('sandbox')).toBe('allow-scripts')
    expect(frame.attributes('referrerpolicy')).toBe('no-referrer')
    expect(frame.attributes('srcdoc')).toContain('<button id="pause" type="button">')
    expect(frame.attributes('srcdoc')).toContain('addEventListener')
    expect(frame.attributes('tabindex')).toBe('0')
    expect(frame.attributes('style')).toContain('pointer-events: auto')
  })

  it('leaves gallery thumbnail interaction to the card', () => {
    const wrapper = mount(PelicanPreview, { props: { html, title: '骑车动画' } })
    expect(wrapper.get('iframe').attributes('tabindex')).toBe('-1')
    expect(wrapper.get('iframe').attributes('style')).toContain('pointer-events: none')
  })

  it('does not mount an empty document', () => {
    const wrapper = mount(PelicanPreview, { props: { html: '', title: '骑车动画' } })
    expect(wrapper.find('iframe').exists()).toBe(false)
  })
})
