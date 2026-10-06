import { fetchAttachmentBlob } from '@/api/chat'

// Chat files need the panel session, so they load as blobs and are shown via
// object URLs. URLs are cached for the page lifetime.
const cache = new Map<number, Promise<string>>()

export function attachmentUrl(id: number): Promise<string> {
  let pending = cache.get(id)
  if (!pending) {
    pending = fetchAttachmentBlob(id).then((blob) => URL.createObjectURL(blob))
    pending.catch(() => cache.delete(id))
    cache.set(id, pending)
  }
  return pending
}

export function releaseAttachmentUrls(): void {
  for (const pending of cache.values()) {
    pending.then((url) => URL.revokeObjectURL(url)).catch(() => undefined)
  }
  cache.clear()
}

export async function downloadAttachment(id: number, filename: string): Promise<void> {
  const url = await attachmentUrl(id)
  const a = document.createElement('a')
  a.href = url
  a.download = filename || 'download'
  document.body.appendChild(a)
  a.click()
  a.remove()
}
