import { forgetAccountToken, readAccountSessions, type AccountSession } from './sessions'
import { ss } from '@/utils/storage'

const STORAGE_KEY = 'LEON_ACCOUNT_SESSIONS'

export function loadAccountSessions(): AccountSession[] {
  const stored = ss.get(STORAGE_KEY)
  if (stored == null)
    return []
  return readAccountSessions(JSON.stringify(stored))
}

export function saveAccountSessions(list: AccountSession[]) {
  ss.set(STORAGE_KEY, readAccountSessions(JSON.stringify(list)))
}

export function forgetStoredAccountToken(token: string | null | undefined) {
  if (!token)
    return
  saveAccountSessions(forgetAccountToken(loadAccountSessions(), token))
}
