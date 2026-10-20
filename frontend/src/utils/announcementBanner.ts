import DOMPurify from 'dompurify'
import { marked } from 'marked'

export function renderAnnouncementBanner(content: string): string {
  const fragment = DOMPurify.sanitize(marked.parse(content, { async: false, breaks: true }), {
    ALLOWED_TAGS: ['p', 'br', 'strong', 'b', 'em', 'span', 'ul', 'ol', 'li'],
    ALLOWED_ATTR: ['style'],
    RETURN_DOM_FRAGMENT: true,
  })
  fragment.querySelectorAll('[style]').forEach(element => {
    const fontSize = (element as HTMLElement).style.fontSize
    element.removeAttribute('style')
    if (/^(1[2-9]|[23]\d|4[0-8])px$/.test(fontSize)) {
      (element as HTMLElement).style.fontSize = fontSize
    }
  })
  const container = document.createElement('div')
  container.append(fragment)
  return container.innerHTML
}
