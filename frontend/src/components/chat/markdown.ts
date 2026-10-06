import { Marked, type Tokens } from 'marked'
import DOMPurify from 'dompurify'

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

let copyLabel = 'Copy'

const chatMarked = new Marked({
  gfm: true,
  breaks: true,
  renderer: {
    code({ text, lang }: Tokens.Code): string {
      const language = (lang || '').trim().split(/\s+/)[0] || ''
      return (
        '<div class="chat-code">' +
        '<div class="chat-code-bar">' +
        `<span>${escapeHtml(language || 'text')}</span>` +
        `<button type="button" class="chat-code-copy" data-chat-copy="1">${escapeHtml(copyLabel)}</button>` +
        '</div>' +
        `<pre><code>${escapeHtml(text)}</code></pre>` +
        '</div>'
      )
    },
    link({ href, title, tokens }: Tokens.Link): string {
      const inner = this.parser.parseInline(tokens)
      const titleAttr = title ? ` title="${escapeHtml(title)}"` : ''
      return `<a href="${escapeHtml(href)}"${titleAttr} target="_blank" rel="noopener noreferrer">${inner}</a>`
    },
  },
})

/** Render model output as sanitized HTML. */
export function renderMarkdown(source: string, label = 'Copy'): string {
  if (!source) return ''
  copyLabel = label
  const html = chatMarked.parse(source, { async: false }) as string
  return DOMPurify.sanitize(html, { ADD_ATTR: ['target'], FORBID_TAGS: ['style', 'form', 'input'] })
}

/** Copy the code block next to a clicked copy button; returns true if handled. */
export async function handleCodeCopyClick(event: MouseEvent, copiedLabel: string, label: string): Promise<boolean> {
  const target = event.target as HTMLElement | null
  const button = target?.closest?.('[data-chat-copy]') as HTMLElement | null
  if (!button) return false
  const code = button.closest('.chat-code')?.querySelector('code')?.textContent || ''
  try {
    await navigator.clipboard.writeText(code)
    button.textContent = copiedLabel
    window.setTimeout(() => {
      button.textContent = label
    }, 1500)
  } catch {
    // clipboard unavailable
  }
  return true
}
