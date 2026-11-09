export const accountSessionLimit = 8

export interface AccountSession {
  id: number
  username: string
  name: string
  role: number
  token: string
}

// Keep only the fields this browser needs to switch. Passwords and other profile fields are dropped.
export function sanitizeAccountSession(input: unknown): AccountSession | null {
  if (!input || typeof input !== 'object')
    return null
  const raw = input as Record<string, unknown>
  const id = Number(raw.id)
  const username = typeof raw.username === 'string' ? raw.username.trim() : ''
  const name = typeof raw.name === 'string' ? raw.name.trim() : ''
  const token = typeof raw.token === 'string' ? raw.token.trim() : ''
  const role = Number(raw.role)
  if (!Number.isInteger(id) || id <= 0 || username === '' || token === '')
    return null
  if (username.length > 80 || name.length > 80 || token.length > 200)
    return null
  return {
    id,
    username,
    name: name || username,
    role: role === 1 ? 1 : 2,
    token,
  }
}

export function readAccountSessions(raw: string | null): AccountSession[] {
  if (!raw)
    return []
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed))
      return []
    const seen = new Set<number>()
    const sessions: AccountSession[] = []
    for (const item of parsed) {
      const session = sanitizeAccountSession(item)
      if (!session || seen.has(session.id))
        continue
      seen.add(session.id)
      sessions.push(session)
      if (sessions.length >= accountSessionLimit)
        break
    }
    return sessions
  }
  catch {
    return []
  }
}

export function rememberAccount(list: AccountSession[], input: unknown): AccountSession[] {
  const session = sanitizeAccountSession(input)
  if (!session)
    return list.slice(0, accountSessionLimit)
  return [session, ...list.filter(item => item.id !== session.id)].slice(0, accountSessionLimit)
}

export function forgetAccount(list: AccountSession[], id: number): AccountSession[] {
  return list.filter(item => item.id !== id)
}

export function forgetAccountToken(list: AccountSession[], token: string | null | undefined): AccountSession[] {
  if (!token)
    return list
  return list.filter(item => item.token !== token)
}
