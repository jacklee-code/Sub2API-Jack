import { describe, expect, it, vi } from 'vitest'

const post = vi.hoisted(() => vi.fn())
vi.mock('../client', () => ({ apiClient: { post }, buildApiUrl: (p: string) => `/api/v1${p}` }))

import { uploadAttachment } from '../chat'

describe('chat attachment upload', () => {
  it('sends the file as multipart form data, not JSON', async () => {
    post.mockResolvedValue({ data: { id: 1 } })
    const file = new File(['png'], 'IMG_9153.PNG', { type: 'image/png' })
    await uploadAttachment(file)
    const [url, body, config] = post.mock.calls[0]
    expect(url).toBe('/chat/attachments')
    expect(body).toBeInstanceOf(FormData)
    expect((body as FormData).get('file')).toBeInstanceOf(File)
    expect(config.headers['Content-Type']).toBe('multipart/form-data')
  })
})
