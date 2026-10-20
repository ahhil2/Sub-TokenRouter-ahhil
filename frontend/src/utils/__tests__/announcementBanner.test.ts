import { describe, expect, it } from 'vitest'
import { renderAnnouncementBanner } from '../announcementBanner'

describe('announcement banner rendering', () => {
  it('preserves bold, line breaks and bounded font sizes', () => {
    const html = renderAnnouncementBanner('欢迎 **API 用户**\n<span style="font-size: 24px">余额 123.45</span>')
    expect(html).toContain('<strong>API 用户</strong>')
    expect(html).toContain('<br>')
    expect(html).toContain('font-size: 24px')
  })

  it('removes scripts, links, images, events and unrelated CSS', () => {
    const html = renderAnnouncementBanner('<script>alert(1)</script><img src="https://example.invalid/x"><a href="javascript:alert(1)">link</a><span onclick="alert(1)" style="font-size:24px;position:fixed;background:url(https://example.invalid/x);font-family:Arial">notice</span>')
    expect(html).toContain('font-size: 24px')
    for (const forbidden of ['script', '<img', '<a ', 'onclick', 'position', 'background', 'font-family', 'https:']) expect(html).not.toContain(forbidden)
  })

  it.each(['0px', '11px', '49px', '200px', '2em', 'calc(100vw)', '24.5px'])('removes unsupported size %s', size => {
    expect(renderAnnouncementBanner(`<span style="font-size:${size}">notice</span>`)).not.toContain('style=')
  })
})
