import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ChatView from '../ChatView.vue'
import type { ChatStreamEvent } from '@/api/chat'

const mocks = vi.hoisted(() => ({
  getConfig: vi.fn(),
  listConversations: vi.fn(),
  listMessages: vi.fn(),
  getModels: vi.fn(),
  createConversation: vi.fn(),
  updateConversation: vi.fn(),
  deleteConversation: vi.fn(),
  sendMessage: vi.fn(),
  sendImages: vi.fn(),
  updatePreference: vi.fn(),
  uploadAttachment: vi.fn(),
  deleteAttachment: vi.fn(),
  fetchAttachmentBlob: vi.fn(),
  resumeStream: vi.fn(),
  stopRun: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))
vi.mock('@/api/chat', () => mocks)
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: mocks.showError, showSuccess: mocks.showSuccess }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key) }),
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))

const config = {
  enabled: true,
  groups: [
    { id: 2, name: 'Newest', description: '', rate_multiplier: 1, subscription_type: 'standard' },
    { id: 1, name: 'Older', description: '', rate_multiplier: 1, subscription_type: 'standard' },
  ],
  preference: { mode: 'chat', group_id: 2, chat_model: 'gpt-5', reasoning_effort: '', image_model: '', image_aspect: '1:1', image_count: 1, web_search: false },
  reasoning_efforts: ['', 'low', 'high'],
  image_aspects: ['1:1', '16:9'],
  max_image_count: 4,
  storage_available: true,
  limits: { max_image_bytes: 20 << 20, max_pdf_bytes: 32 << 20, max_text_bytes: 2 << 20, max_attachments: 10 },
}

function conversation(overrides: Record<string, unknown> = {}) {
  return { id: 5, mode: 'chat', group_id: 2, model: 'gpt-5', reasoning_effort: '', image_aspect: '1:1', image_count: 1, title: '', created_at: '', updated_at: '', ...overrides }
}

function message(overrides: Record<string, unknown> = {}) {
  return { id: 1, conversation_id: 5, role: 'user', content: 'hi', status: 'complete', input_tokens: 0, output_tokens: 0, citations: [], created_at: '', attachments: [], ...overrides }
}

function render() {
  return mount(ChatView, {
    attachTo: document.body,
    global: {
      stubs: {
        Icon: true,
        Teleport: true,
        ConfirmDialog: { props: ['show'], emits: ['confirm', 'cancel'], template: '<div v-if="show" data-confirm><button data-ok @click="$emit(\'confirm\')" /></div>' },
      },
    },
  })
}

beforeEach(() => {
  vi.resetAllMocks()
  mocks.getConfig.mockResolvedValue(structuredClone(config))
  mocks.listConversations.mockResolvedValue([])
  mocks.listMessages.mockResolvedValue([])
  mocks.getModels.mockImplementation(async (_group: number, mode: string) => (mode === 'image' ? ['gpt-image-2'] : ['gpt-5', 'gpt-5-mini']))
  mocks.updateConversation.mockImplementation(async (id: number, patch: Record<string, unknown>) => conversation({ id, ...patch }))
  mocks.updatePreference.mockResolvedValue({})
})

describe('Chat mode', () => {
  it('defaults to the preferred group and streams a reply into a new conversation', async () => {
    mocks.createConversation.mockResolvedValue(conversation())
    mocks.sendMessage.mockImplementation(async (_id: number, _input: unknown, onEvent: (e: ChatStreamEvent) => void) => {
      onEvent({ type: 'start', conversation: conversation({ title: 'Hello there' }) as never, user_message: message({ id: 10, content: 'Hello there' }) as never, assistant_message: message({ id: 11, role: 'assistant', content: '', status: 'streaming' }) as never, regenerate: false })
      onEvent({ type: 'search', status: 'done', query: 'weather' })
      onEvent({ type: 'delta', text: '**Hi** <script>alert(1)</script>' })
      onEvent({ type: 'citations', citations: [{ url: 'https://example.test/x', title: 'Example' }] })
      onEvent({ type: 'done', message: message({ id: 11, role: 'assistant', content: '**Hi** <script>alert(1)</script>', status: 'complete', model: 'gpt-5', citations: [{ url: 'https://example.test/x', title: 'Example' }] }) as never })
    })
    const wrapper = render()
    await flushPromises()

    expect(wrapper.get('[data-testid="chat-settings"]').text()).toContain('gpt-5')
    expect(mocks.getModels).toHaveBeenCalledWith(2, 'chat')

    await wrapper.get('button[aria-pressed="false"]').trigger('click')
    await wrapper.get('textarea').setValue('Hello there')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(mocks.createConversation).toHaveBeenCalledWith(expect.objectContaining({ mode: 'chat', group_id: 2, model: 'gpt-5' }))
    expect(mocks.sendMessage).toHaveBeenCalledWith(5, expect.objectContaining({ text: 'Hello there', regenerate: false, web_search: true }), expect.any(Function), expect.any(AbortSignal))
    expect(mocks.updatePreference).toHaveBeenCalledWith({ web_search: true })
    const html = wrapper.html()
    expect(html).toContain('<strong>Hi</strong>')
    expect(html).not.toContain('<script>')
    expect(wrapper.text()).toContain('chat.searched')
    expect(wrapper.text()).toContain('Example')
    expect(wrapper.text()).toContain('Hello there')
    expect(wrapper.text()).toContain('chat.regenerate')
    wrapper.unmount()
  })

  it('keeps IME composition from sending', async () => {
    mocks.createConversation.mockResolvedValue(conversation())
    const wrapper = render()
    await flushPromises()
    await wrapper.get('textarea').setValue('你好')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', isComposing: true })
    await flushPromises()
    expect(mocks.createConversation).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('restores the draft when the request fails before the reply starts', async () => {
    mocks.createConversation.mockResolvedValue(conversation())
    mocks.sendMessage.mockRejectedValue(new Error('Insufficient balance'))
    const wrapper = render()
    await flushPromises()
    await wrapper.get('textarea').setValue('try')
    await wrapper.get('button[aria-label="chat.send"]').trigger('click')
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledWith('Insufficient balance')
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('try')
    wrapper.unmount()
  })

  it('loads the latest conversation, renames and deletes it', async () => {
    mocks.listConversations.mockResolvedValue([conversation({ title: 'First' })])
    mocks.listMessages.mockResolvedValue([message(), message({ id: 2, role: 'assistant', content: 'answer', model: 'gpt-5' })])
    mocks.deleteConversation.mockResolvedValue(undefined)
    const wrapper = render()
    await flushPromises()
    expect(mocks.listMessages).toHaveBeenCalledWith(5)
    expect(wrapper.text()).toContain('answer')

    await wrapper.get('button[aria-label="chat.rename"]').trigger('click')
    const input = wrapper.get('input[aria-label="chat.rename"]')
    await input.setValue('Renamed')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(mocks.updateConversation).toHaveBeenCalledWith(5, { title: 'Renamed' })

    await wrapper.get('button[aria-label="chat.delete"]').trigger('click')
    await wrapper.get('[data-confirm] [data-ok]').trigger('click')
    await flushPromises()
    expect(mocks.deleteConversation).toHaveBeenCalledWith(5)
    expect(wrapper.text()).toContain('chat.welcome')
    wrapper.unmount()
  })

  it('switches to image mode, offers only image models and shows generated images', async () => {
    mocks.createConversation.mockResolvedValue(conversation({ mode: 'image', model: 'gpt-image-2', image_aspect: '16:9', image_count: 2 }))
    mocks.fetchAttachmentBlob.mockResolvedValue(new Blob(['x'], { type: 'image/png' }))
    const created = URL.createObjectURL
    URL.createObjectURL = vi.fn(() => 'blob:test')
    mocks.sendImages.mockImplementation(async (_id: number, _input: unknown, onEvent: (e: ChatStreamEvent) => void) => {
      onEvent({ type: 'start', conversation: conversation({ mode: 'image' }) as never, user_message: message({ id: 20, content: 'a cat' }) as never, assistant_message: message({ id: 21, role: 'assistant', content: '', status: 'streaming' }) as never, regenerate: false, count: 2, aspect: '16:9' })
      onEvent({ type: 'image', index: 0, attachment: { id: 30, kind: 'generated', filename: 'image-1.png', mime: 'image/png', size: 1, width: 1024, height: 576, created_at: '' } })
      onEvent({ type: 'image_error', index: 1, error: 'busy' })
      onEvent({ type: 'done', message: message({ id: 21, role: 'assistant', content: '', status: 'complete', error: 'busy', attachments: [{ id: 30, kind: 'generated', filename: 'image-1.png', mime: 'image/png', size: 1, width: 1024, height: 576, created_at: '' }] }) as never })
    })
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="chat-mode-image"]').trigger('click')
    await flushPromises()
    expect(mocks.getModels).toHaveBeenLastCalledWith(2, 'image')
    await wrapper.get('[data-testid="chat-settings"]').trigger('click')
    expect(wrapper.find('[data-testid="chat-effort-range"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('chat.resolutionHint')

    await wrapper.get('[data-testid="chat-aspect-16:9"]').trigger('click')
    await wrapper.get('[data-testid="chat-count-2"]').trigger('click')
    await wrapper.get('textarea').setValue('a cat')
    await wrapper.get('button[aria-label="chat.send"]').trigger('click')
    await flushPromises()

    expect(mocks.createConversation).toHaveBeenCalledWith(expect.objectContaining({ mode: 'image', model: 'gpt-image-2', image_aspect: '16:9', image_count: 2 }))
    expect(mocks.sendImages).toHaveBeenCalledWith(5, expect.objectContaining({ prompt: 'a cat' }), expect.any(Function), expect.any(AbortSignal))
    const thumb = wrapper.get('button[title="image-1.png"]')
    // Two images in one reply share a 200px longest edge, keeping 16:9.
    const box = thumb.element.parentElement!.getAttribute('style') || ''
    expect(box).toContain('width: 200px')
    expect(box).toContain('aspect-ratio: 200 / 113')
    expect(wrapper.text()).toContain('chat.imageFailed')

    await thumb.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('chat.download')
    await wrapper.findAll('button').find((b) => b.text() === 'chat.useAsReference')!.trigger('click')
    await flushPromises()
    expect(wrapper.find('img[src="blob:test"]').exists()).toBe(true)
    URL.createObjectURL = created
    wrapper.unmount()
  })

  it('resumes a reply that kept generating while the page was away', async () => {
    const streaming = message({ id: 2, role: 'assistant', content: 'Hel', status: 'streaming', model: 'gpt-5' })
    mocks.listConversations.mockResolvedValue([conversation({ title: 'Running' })])
    mocks.listMessages.mockResolvedValue([message(), streaming])
    mocks.resumeStream.mockImplementation(async (_id: number, onEvent: (e: ChatStreamEvent) => void) => {
      onEvent({ type: 'snapshot', message: { ...streaming, content: 'Hello wor' } as never, searching: false, search_queries: [], pending_images: 0, image_errors: [] })
      onEvent({ type: 'delta', text: 'ld' })
      onEvent({ type: 'done', message: { ...streaming, content: 'Hello world', status: 'complete' } as never })
      return true
    })
    const wrapper = render()
    await flushPromises()
    expect(mocks.resumeStream).toHaveBeenCalledWith(5, expect.any(Function), expect.any(AbortSignal))
    expect(wrapper.text()).toContain('Hello world')
    expect(wrapper.text()).not.toContain('Hello worldld')
    expect(mocks.stopRun).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('leaving the page only detaches; the stop button stops the run', async () => {
    mocks.createConversation.mockResolvedValue(conversation())
    let signal: AbortSignal | undefined
    mocks.sendMessage.mockImplementation((_id: number, _input: unknown, onEvent: (e: ChatStreamEvent) => void, s: AbortSignal) => {
      signal = s
      onEvent({ type: 'start', conversation: conversation() as never, user_message: message({ id: 10 }) as never, assistant_message: message({ id: 11, role: 'assistant', content: '', status: 'streaming' }) as never, regenerate: false })
      return new Promise((_resolve, reject) => s.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError'))))
    })
    mocks.stopRun.mockResolvedValue(undefined)
    const wrapper = render()
    await flushPromises()
    await wrapper.get('textarea').setValue('long task')
    await wrapper.get('button[aria-label="chat.send"]').trigger('click')
    await flushPromises()

    await wrapper.get('button[aria-label="chat.stop"]').trigger('click')
    await flushPromises()
    expect(mocks.stopRun).toHaveBeenCalledWith(5)

    wrapper.unmount()
    expect(signal?.aborted).toBe(true)
    expect(mocks.stopRun).toHaveBeenCalledTimes(1)
  })

  it('changes model, effort and group from the composer pill', async () => {
    mocks.listConversations.mockResolvedValue([conversation()])
    const wrapper = render()
    await flushPromises()
    await wrapper.get('[data-testid="chat-settings"]').trigger('click')
    const range = wrapper.get('[data-testid="chat-effort-range"]')
    await range.setValue('2')
    await flushPromises()
    expect(mocks.updateConversation).toHaveBeenCalledWith(5, { reasoning_effort: 'high' })
    expect(wrapper.get('[data-testid="chat-effort-label"]').text()).toBe('chat.efforts.high')

    await wrapper.get('[data-testid="chat-open-models"]').trigger('click')
    await wrapper.get('[data-testid="chat-model-gpt-5-mini"]').trigger('click')
    await flushPromises()
    expect(mocks.updateConversation).toHaveBeenCalledWith(5, { model: 'gpt-5-mini' })

    await wrapper.get('[data-testid="chat-open-models"]').trigger('click')
    await wrapper.get('[data-testid="chat-group-1"]').trigger('click')
    await flushPromises()
    expect(mocks.updateConversation).toHaveBeenCalledWith(5, { group_id: 1 })
    expect(mocks.getModels).toHaveBeenCalledWith(1, 'chat')
    wrapper.unmount()
  })

  it('explains when chat is turned off', async () => {
    mocks.getConfig.mockResolvedValue({ ...structuredClone(config), enabled: false })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('chat.disabled')
    wrapper.unmount()
  })
})
