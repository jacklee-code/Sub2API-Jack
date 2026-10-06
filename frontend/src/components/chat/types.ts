import type { ChatAttachment, ChatMessage } from '@/api/chat'

/** A message plus the live state shown while it streams. */
export interface UiMessage extends ChatMessage {
  searching?: boolean
  searchQueries?: string[]
  pendingImages?: number
  imageErrors?: { index: number; error: string }[]
  aspect?: string
  /** Earlier messages left out to fit the model context. */
  omitted?: number
}

/** A file in the composer, uploading or ready to send. */
export interface DraftAttachment {
  key: string
  name: string
  progress: number
  error?: string
  attachment?: ChatAttachment
  previewUrl?: string
  /** True when the file already belongs to a message (reused reference). */
  reused?: boolean
}
