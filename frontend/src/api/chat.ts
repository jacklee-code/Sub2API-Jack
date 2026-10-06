/**
 * Jack chat mode API. Conversations live on the server; model calls go
 * through the gateway with hidden per-group keys and bill like API calls.
 */
import { apiClient, buildApiUrl } from './client'
import { getLocale } from '@/i18n'
import { refreshAuthTokens } from './tokenRefresh'

export type ChatMode = 'chat' | 'image'
export type ChatMessageStatus = 'streaming' | 'complete' | 'error' | 'aborted'
export type ChatAttachmentKind = 'image' | 'pdf' | 'text' | 'generated'

export interface ChatGroup {
  id: number
  name: string
  description: string
  rate_multiplier: number
  subscription_type: string
}

export interface ChatPreference {
  mode: ChatMode
  group_id: number | null
  chat_model: string
  reasoning_effort: string
  image_model: string
  image_aspect: string
  image_count: number
  web_search: boolean
}

export interface ChatConfig {
  enabled: boolean
  groups: ChatGroup[]
  preference: ChatPreference
  reasoning_efforts: string[]
  image_aspects: string[]
  image_sizes?: Record<string, string>
  max_image_count: number
  storage_available: boolean
  limits: {
    max_image_bytes: number
    max_pdf_bytes: number
    max_text_bytes: number
    max_attachments: number
    max_context_tokens: number
  }
}

export interface ChatConversation {
  id: number
  mode: ChatMode
  group_id: number | null
  model: string
  reasoning_effort: string
  image_aspect: string
  image_count: number
  title: string
  web_search: boolean
  unread: boolean
  created_at: string
  updated_at: string
}

export interface ChatAttachment {
  id: number
  message_id?: number
  kind: ChatAttachmentKind
  filename: string
  mime: string
  size: number
  width?: number
  height?: number
  created_at: string
}

export interface ChatCitation {
  url: string
  title?: string
}

export interface ChatMessage {
  id: number
  conversation_id: number
  role: 'user' | 'assistant'
  content: string
  reasoning?: string
  model?: string
  reasoning_effort?: string
  status: ChatMessageStatus
  error?: string
  input_tokens: number
  output_tokens: number
  web_search?: boolean
  citations: ChatCitation[]
  created_at: string
  attachments: ChatAttachment[]
}

export type ConversationPatch = Partial<
  Pick<ChatConversation, 'title' | 'mode' | 'group_id' | 'model' | 'reasoning_effort' | 'image_aspect' | 'image_count' | 'web_search'>
>

export type ChatStreamEvent =
  | { type: 'start'; conversation: ChatConversation; user_message: ChatMessage; assistant_message: ChatMessage; regenerate: boolean; count?: number; aspect?: string; omitted?: number }
  | { type: 'delta'; text: string }
  | { type: 'reasoning'; text: string }
  | { type: 'search'; status: 'searching' | 'done'; query?: string }
  | { type: 'citations'; citations: ChatCitation[] }
  | { type: 'image'; index: number; attachment: ChatAttachment }
  | { type: 'image_error'; index: number; error: string }
  | { type: 'done'; message: ChatMessage }
  | { type: 'error'; message: string }
  | { type: 'snapshot'; message: ChatMessage; searching: boolean; search_queries: string[]; pending_images: number; image_errors: { index: number; error: string }[]; aspect?: string; omitted?: number }

export async function getConfig(): Promise<ChatConfig> {
  return (await apiClient.get<ChatConfig>('/chat/config')).data
}

export async function getModels(groupId: number, mode: ChatMode): Promise<string[]> {
  return (await apiClient.get<string[]>(`/chat/groups/${groupId}/models`, { params: { mode } })).data
}

export async function updatePreference(patch: Partial<Pick<ChatPreference, 'web_search'>>): Promise<ChatPreference> {
  return (await apiClient.put<ChatPreference>('/chat/preferences', patch)).data
}

export async function listConversations(): Promise<ChatConversation[]> {
  return (await apiClient.get<ChatConversation[]>('/chat/conversations')).data
}

export async function createConversation(patch: ConversationPatch = {}): Promise<ChatConversation> {
  return (await apiClient.post<ChatConversation>('/chat/conversations', patch)).data
}

export async function updateConversation(id: number, patch: ConversationPatch): Promise<ChatConversation> {
  return (await apiClient.patch<ChatConversation>(`/chat/conversations/${id}`, patch)).data
}

/** Mark a conversation read, or unread when unread is true. */
export async function markRead(id: number, unread = false): Promise<void> {
  await apiClient.post(`/chat/conversations/${id}/read`, { unread })
}

/** Number of conversations with replies the user has not seen. */
export async function getUnreadCount(): Promise<number> {
  return (await apiClient.get<{ count: number }>('/chat/unread')).data.count
}

export async function deleteConversation(id: number): Promise<void> {
  await apiClient.delete(`/chat/conversations/${id}`)
}

export async function listMessages(id: number): Promise<ChatMessage[]> {
  return (await apiClient.get<ChatMessage[]>(`/chat/conversations/${id}/messages`)).data
}

export async function uploadAttachment(file: File, onProgress?: (fraction: number) => void): Promise<ChatAttachment> {
  const form = new FormData()
  form.append('file', file)
  return (
    await apiClient.post<ChatAttachment>('/chat/attachments', form, {
      // The client defaults to JSON; multipart lets axios add the boundary.
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 0,
      onUploadProgress: (e) => {
        if (onProgress && e.total) onProgress(e.loaded / e.total)
      },
    })
  ).data
}

export async function deleteAttachment(id: number): Promise<void> {
  await apiClient.delete(`/chat/attachments/${id}`)
}

export function attachmentContentPath(id: number, download = false): string {
  return buildApiUrl(`/chat/attachments/${id}/content${download ? '?download=1' : ''}`)
}

function authHeaders(): Record<string, string> {
  const headers: Record<string, string> = { 'Accept-Language': getLocale() }
  const token = localStorage.getItem('auth_token')
  if (token) headers.Authorization = `Bearer ${token}`
  return headers
}

/** fetch with the panel session; refreshes the token once on 401. */
async function authedFetch(url: string, init: RequestInit): Promise<Response> {
  const attempt = () => fetch(url, { ...init, headers: { ...authHeaders(), ...(init.headers as Record<string, string> | undefined) } })
  let res = await attempt()
  if (res.status === 401 && localStorage.getItem('refresh_token')) {
    try {
      await refreshAuthTokens({ failedAccessToken: localStorage.getItem('auth_token') })
      res = await attempt()
    } catch {
      // keep the 401 response
    }
  }
  return res
}

async function errorMessage(res: Response): Promise<string> {
  try {
    const body = await res.json()
    return body?.message || body?.error?.message || res.statusText
  } catch {
    return res.statusText || `HTTP ${res.status}`
  }
}

/** Download an attachment as a Blob (images need the panel session). */
export async function fetchAttachmentBlob(id: number, signal?: AbortSignal): Promise<Blob> {
  const res = await authedFetch(attachmentContentPath(id), { method: 'GET', signal })
  if (!res.ok) throw new Error(await errorMessage(res))
  return res.blob()
}

/** Read an SSE response body and call onEvent for each `data:` event. */
export async function readEventStream(res: Response, onEvent: (event: ChatStreamEvent) => void): Promise<void> {
  const reader = res.body?.getReader()
  if (!reader) return
  const decoder = new TextDecoder()
  let buffer = ''
  const flush = (block: string) => {
    const data = block
      .split('\n')
      .filter((l) => l.startsWith('data:'))
      .map((l) => l.slice(5).trimStart())
      .join('\n')
    if (!data) return
    try {
      onEvent(JSON.parse(data) as ChatStreamEvent)
    } catch {
      // ignore malformed events
    }
  }
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, '\n')
    let idx: number
    while ((idx = buffer.indexOf('\n\n')) >= 0) {
      flush(buffer.slice(0, idx))
      buffer = buffer.slice(idx + 2)
    }
  }
  buffer += decoder.decode()
  if (buffer.trim()) flush(buffer)
}

async function stream(path: string, body: unknown, onEvent: (event: ChatStreamEvent) => void, signal?: AbortSignal): Promise<void> {
  const res = await authedFetch(buildApiUrl(path), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
    body: JSON.stringify(body),
    signal,
  })
  if (!res.ok) throw new Error(await errorMessage(res))
  if (!(res.headers.get('Content-Type') || '').includes('text/event-stream')) {
    throw new Error(await errorMessage(res))
  }
  await readEventStream(res, onEvent)
}

/**
 * Reattach to a conversation's running reply. Resolves false when nothing is
 * running on the server (the reply already finished or ran elsewhere).
 */
export async function resumeStream(conversationId: number, onEvent: (event: ChatStreamEvent) => void, signal?: AbortSignal): Promise<boolean> {
  const res = await authedFetch(buildApiUrl(`/chat/conversations/${conversationId}/stream`), {
    method: 'GET',
    headers: { Accept: 'text/event-stream' },
    signal,
  })
  if (!res.ok) throw new Error(await errorMessage(res))
  if (!(res.headers.get('Content-Type') || '').includes('text/event-stream')) return false
  await readEventStream(res, onEvent)
  return true
}

/** Stop a conversation's running reply; the server saves it as stopped. */
export async function stopRun(conversationId: number): Promise<void> {
  await apiClient.post(`/chat/conversations/${conversationId}/stop`)
}

export interface SendMessageInput {
  text?: string
  attachment_ids?: number[]
  regenerate?: boolean
  web_search?: boolean
}

export function sendMessage(conversationId: number, input: SendMessageInput, onEvent: (event: ChatStreamEvent) => void, signal?: AbortSignal): Promise<void> {
  return stream(`/chat/conversations/${conversationId}/messages`, input, onEvent, signal)
}

export interface SendImagesInput {
  prompt?: string
  attachment_ids?: number[]
  regenerate?: boolean
}

export function sendImages(conversationId: number, input: SendImagesInput, onEvent: (event: ChatStreamEvent) => void, signal?: AbortSignal): Promise<void> {
  return stream(`/chat/conversations/${conversationId}/images`, input, onEvent, signal)
}

export interface ChatAdminSettings {
  enabled: boolean
  system_prompt: string
  max_image_bytes: number
  max_pdf_bytes: number
  max_text_bytes: number
  max_attachments: number
  max_conversations: number
  max_context_tokens: number
}

export async function getAdminSettings(): Promise<{ settings: ChatAdminSettings; storage_available: boolean }> {
  return (await apiClient.get('/admin/chat-settings')).data
}

export async function updateAdminSettings(settings: ChatAdminSettings): Promise<{ settings: ChatAdminSettings; storage_available: boolean }> {
  return (await apiClient.put('/admin/chat-settings', settings)).data
}
