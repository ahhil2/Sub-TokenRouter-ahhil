/**
 * pelicanHtml.ts prepares model-generated drawing documents for the sandboxed preview
 * frame. Empty or oversized input produces an empty document. Parsed content loses
 * nested documents, form controls, navigation attributes, and unsupported resource
 * references before being inserted into a fresh HTML document.
 *
 * The generated document begins with a content security policy that blocks network
 * connections and external resources while permitting inline drawing code and styles.
 * Inline scripts receive the host nonce when one is available. Selected root layout
 * attributes survive reconstruction so drawings keep their intended layout; the
 * preview component supplies iframe sandboxing around the resulting document.
 */

export const PELICAN_CSP =
  "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; media-src data: blob:; font-src data:; connect-src 'none'; frame-src 'none'; worker-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'"

export function preparePelicanHTML(
  html: string,
  nonce = globalThis.document.querySelector<HTMLScriptElement>('script[nonce]')?.nonce || '',
): string {
  if (!html || new TextEncoder().encode(html).byteLength > 1024 * 1024) return ''
  const template = globalThis.document.createElement('template')
  template.innerHTML = html
  const fragment = template.content
  fragment
    .querySelectorAll(
      'meta, base, link, iframe, frame, frameset, object, embed, portal, form, input, textarea, select',
    )
    .forEach((element) => element.remove())
  for (const element of fragment.querySelectorAll('*')) {
    for (const attribute of [...element.attributes]) {
      const name = attribute.name.toLowerCase()
      if (
        name.startsWith('on') ||
        [
          'srcdoc',
          'srcset',
          'action',
          'form',
          'formaction',
          'formtarget',
          'target',
          'ping',
          'download',
          'autofocus',
        ].includes(name)
      ) {
        element.removeAttribute(attribute.name)
      }
      if (['href', 'xlink:href', 'src', 'poster', 'data', 'background'].includes(name)) {
        const value = attribute.value.trim()
        const image =
          element.tagName.toLowerCase() === 'img' || element.tagName.toLowerCase() === 'image'
        if (!(
          value.startsWith('#') ||
          (image && /^data:image\/(png|jpeg|gif|webp);base64,/i.test(value))
        ))
          element.removeAttribute(attribute.name)
      }
    }
  }
  fragment.querySelectorAll('button').forEach((button) => button.setAttribute('type', 'button'))
  fragment.querySelectorAll('script').forEach((script) => {
    script.removeAttribute('nonce')
    if (nonce) script.setAttribute('nonce', nonce)
  })
  const document = globalThis.document.implementation.createHTMLDocument('')
  const policy = document.createElement('meta')
  policy.httpEquiv = 'Content-Security-Policy'
  policy.content = PELICAN_CSP
  document.head.prepend(policy)
  const style = document.createElement('style')
  style.textContent = 'html{overflow:auto}body{margin:0}svg{max-width:100%}'
  document.head.append(style)
  for (const tag of ['html', 'body'] as const) {
    const opening = html.match(new RegExp(`<${tag}\\b((?:[^>"']|"[^"]*"|'[^']*')*)>`, 'i'))
    if (!opening) continue
    const attributes = globalThis.document.createElement('template')
    attributes.innerHTML = `<div${opening[1]}></div>`
    const source = attributes.content.firstElementChild
    const target = tag === 'html' ? document.documentElement : document.body
    for (const name of ['class', 'style', 'id', 'lang', 'dir']) {
      if (source?.hasAttribute(name)) target.setAttribute(name, source.getAttribute(name)!)
    }
  }
  document.body.append(fragment)
  return '<!doctype html>\n' + document.documentElement.outerHTML
}
