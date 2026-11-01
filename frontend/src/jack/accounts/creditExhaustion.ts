import type { Account } from '@/types'

// Stable backend error marker; this hold survives restarts and needs recovery.
export function isCreditExhausted(account: Pick<Account, 'status' | 'error_message'>): boolean {
  return account.status === 'error' && account.error_message?.startsWith('[jack_openai_credit_exhausted] ') === true
}
