import { describe, expect, it } from 'vitest'
import { readEventStream, type ChatStreamEvent } from '../chat'
import { fitBox } from '@/components/chat/imageSize'
import { renderMarkdown } from '@/components/chat/markdown'

function streamOf(chunks: string[]): Response {
  const encoder = new TextEncoder()
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c))
      controller.close()
    },
  })
  return new Response(body, { headers: { 'Content-Type': 'text/event-stream' } })
}

describe('chat API helpers', () => {
  it('parses SSE events split across chunks', async () => {
    const events: ChatStreamEvent[] = []
    await readEventStream(
      streamOf(['data: {"type":"delta","te', 'xt":"a"}\r\n\r\ndata: {"type":"delta","text":"b"}\n\n', 'data: not json\n\ndata: {"type":"done","message":{}}']),
      (e) => events.push(e)
    )
    expect(events.map((e) => e.type)).toEqual(['delta', 'delta', 'done'])
    expect((events[1] as { text: string }).text).toBe('b')
  })

  it('fits images by ratio into the longest edge', () => {
    expect(fitBox(1024, 576, 320)).toEqual({ width: 320, height: 180 })
    expect(fitBox(576, 1024, 200)).toEqual({ width: 113, height: 200 })
    expect(fitBox(undefined, undefined, 320, '1:1')).toEqual({ width: 320, height: 320 })
  })

  it('renders sanitized markdown with copyable code and safe links', () => {
    const html = renderMarkdown('[x](https://example.test) <img src=x onerror=alert(1)>\n\n```js\nconst a = "<b>"\n```', '复制')
    expect(html).toContain('target="_blank"')
    expect(html).toContain('rel="noopener noreferrer"')
    expect(html).not.toContain('onerror')
    expect(html).toContain('data-chat-copy="1">复制</button>')
    expect(html).toContain('<code>const a = "&lt;b&gt;"</code>')
  })
})
