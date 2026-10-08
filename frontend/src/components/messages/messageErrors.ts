import { extractApiErrorCode } from '@/utils/apiError'

const messageReasons = new Set(['SITE_MESSAGE_INVALID', 'SITE_MESSAGE_FORBIDDEN', 'SITE_MESSAGE_RECIPIENT', 'SITE_MESSAGE_NOT_FOUND', 'SITE_MESSAGE_CONFLICT'])

export function messageErrorKey(error: unknown, fallback: 'loadFailed' | 'sendFailed') {
  const reason = extractApiErrorCode(error)
  return reason && messageReasons.has(reason) ? `messages.errors.${reason}` : `messages.${fallback}`
}
