import { describe, expect, it } from 'vitest'
import { preparePelicanHTML, PELICAN_CSP } from '../pelicanHtml'

/**
 * pelicanHtml.test.ts exercises generated-document preparation with animation,
 * navigation, resource, and malformed-size fixtures. Assertions inspect the policy
 * placement, removed attributes, retained drawing controls and layout, and host nonce
 * handling before documents reach the sandboxed frame.
 */

describe('pelican sandbox document', () => {
  it('preserves inline animation and inserts policy before generated code', () => {
    const result = preparePelicanHTML(
      '<html><head><script>document.body.dataset.playing="yes"</script><style>svg{animation:spin 1s infinite}</style></head><body><svg><animate attributeName="x" /></svg></body></html>',
    )
    expect(result).toContain('<script>')
    expect(result).toContain('<animate')
    expect(result.indexOf('Content-Security-Policy')).toBeLessThan(result.indexOf('<script>'))
    expect(result).toContain("connect-src 'none'")
    expect(PELICAN_CSP).toContain("frame-src 'none'")
  })
  it('removes external resources, navigation markup and nested documents', () => {
    const result = preparePelicanHTML(
      '<html><head><meta http-equiv="refresh" content="0;url=https://example.invalid"><base href="https://example.invalid"><link rel="stylesheet" href="https://example.invalid/a"></head><body><iframe srcdoc="secret"></iframe><form action="/admin"><input></form><script src="https://example.invalid/x"></script><img src="https://example.invalid/x" srcset="https://example.invalid/y 2x"><a href="javascript:alert(1)" ping="/secret" target="_top">link</a><svg><use href="#bird"/></svg></body></html>',
    )
    expect(result).not.toContain('example.invalid')
    expect(result).not.toContain('javascript:')
    expect(result).not.toContain('<iframe')
    expect(result).not.toContain('http-equiv="refresh"')
    expect(result).not.toContain('<form')
    expect(result).not.toContain('target=')
    expect(result).toContain('href="#bird"')
  })
  it('rejects empty and oversized documents', () => {
    expect(preparePelicanHTML('')).toBe('')
    expect(preparePelicanHTML('鸟'.repeat(400000))).toBe('')
  })
  it('keeps pause buttons and listener scripts while removing form and navigation attributes', () => {
    const result = preparePelicanHTML(`<html><body>
      <svg id="bird"></svg>
      <button id="pause" type="submit" form="external" formaction="https://example.invalid" formtarget="_top" onclick="alert(1)" aria-label="暂停动画">暂停</button>
      <script>document.getElementById('pause').addEventListener('click', () => { document.body.dataset.paused = 'yes' })</script>
    </body></html>`)
    const parsed = new DOMParser().parseFromString(result, 'text/html')
    const button = parsed.querySelector('#pause')
    expect(button?.tagName).toBe('BUTTON')
    expect(button?.getAttribute('type')).toBe('button')
    expect(button?.getAttribute('aria-label')).toBe('暂停动画')
    for (const attribute of ['form', 'formaction', 'formtarget', 'onclick']) {
      expect(button?.hasAttribute(attribute)).toBe(false)
    }
    expect(parsed.querySelector('script')?.textContent).toContain(
      "getElementById('pause').addEventListener",
    )
    expect(result).not.toContain('example.invalid')
    expect(result).toContain('html{overflow:auto}')
    expect(PELICAN_CSP).toContain("form-action 'none'")
  })
  it('keeps root layout attributes while stripping event handlers', () => {
    const result = preparePelicanHTML(
      '<html lang="zh"><body id="scene" style="display:grid" onload="alert(1)"><svg onload="alert(2)"></svg></body></html>',
    )
    expect(result).toContain('lang="zh"')
    expect(result).toContain('id="scene"')
    expect(result).toContain('style="display:grid"')
    expect(result).not.toContain('onload')
  })
  it('uses the host nonce for inline scripts without allowing external scripts', () => {
    const result = preparePelicanHTML(
      '<html><body><script nonce="untrusted" src="https://example.invalid/script.js">window.animated=true</script></body></html>',
      'host-nonce',
    )
    expect(result).toContain('nonce="host-nonce"')
    expect(result).not.toContain('nonce="untrusted"')
    expect(result).not.toContain('src=')
    expect(result).toContain("script-src 'unsafe-inline'")
    expect(result).not.toContain("script-src 'unsafe-inline' 'nonce-")
  })
})
