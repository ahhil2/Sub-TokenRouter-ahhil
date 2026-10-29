import { extractApiErrorCode } from '@/utils/apiError'

/**
 * messageErrors.ts maps recognized messaging API error codes to localized
 * explanations. Unknown codes and errors without a recognized reason use the caller-
 * selected load or send failure message, keeping transport details out of the visible
 * conversation interface.
 */

const messageReasons = new Set([
  'SITE_MESSAGE_INVALID',
  'SITE_MESSAGE_FORBIDDEN',
  'SITE_MESSAGE_RECIPIENT',
  'SITE_MESSAGE_NOT_FOUND',
  'SITE_MESSAGE_CONFLICT',
])

export function messageErrorKey(error: unknown, fallback: 'loadFailed' | 'sendFailed') {
  const reason = extractApiErrorCode(error)
  return reason && messageReasons.has(reason) ? `messages.errors.${reason}` : `messages.${fallback}`
}
