import { apiClient } from './client'

/**
 * siteMessages.ts provides conversation, recipient, and message operations for
 * account messaging. Thread responses include both participants' read positions,
 * and send requests carry a client identifier used by the server to recognize
 * repeated submissions.
 */

export interface SiteMessage {
  id: number
  user_id: number
  sender_id: number
  is_admin: boolean
  body: string
  created_at: string
}

export interface MessageContact {
  id: number
  username: string
  email: string
}

export interface MessageConversation {
  user_id: number
  username: string
  email: string
  last_message_id: number
  body: string
  is_admin: boolean
  created_at: string
  unread_count: number
}

export interface MessagePage<T> {
  items: T[]
  has_more: boolean
}

export interface MessageThread extends MessagePage<SiteMessage> {
  contact: MessageContact
  read_through_id: number
  peer_read_through_id: number
}

export interface SendMessageRequest {
  body: string
  client_id: string
  recipient_ids?: number[]
  all_users?: boolean
}

export const siteMessagesAPI = {
  async summary() {
    const { data } = await apiClient.get<{ unread_count: number }>('/site-messages/summary')
    return data
  },
  async conversations(search = '', page = 1) {
    const { data } = await apiClient.get<MessagePage<MessageConversation>>(
      '/site-messages/conversations',
      { params: { search, page } },
    )
    return data
  },
  async contacts(search = '', page = 1) {
    const { data } = await apiClient.get<MessagePage<MessageContact>>('/site-messages/contacts', {
      params: { search, page },
    })
    return data
  },
  async thread(userID: number, before = 0) {
    const { data } = await apiClient.get<MessageThread>(`/site-messages/conversations/${userID}`, {
      params: { before },
    })
    return data
  },
  async markRead(userID: number, throughID: number) {
    await apiClient.post(`/site-messages/conversations/${userID}/read`, { through_id: throughID })
  },
  async send(request: SendMessageRequest) {
    const { data } = await apiClient.post<{ recipient_count: number }>('/site-messages', request)
    return data
  },
}
