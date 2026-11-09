import assert from 'node:assert/strict'
import test from 'node:test'
import { forgetAccount, forgetAccountToken, readAccountSessions, rememberAccount, sanitizeAccountSession } from './sessions.ts'

test('a saved account keeps the session fields and drops the password', () => {
  const session = sanitizeAccountSession({
    id: 2,
    username: 'ada@sun.cc',
    name: 'Ada',
    role: 2,
    token: 'session-token',
    password: 'secret-value',
    mail: 'ada@sun.cc',
  })
  assert.deepEqual(session, {
    id: 2,
    username: 'ada@sun.cc',
    name: 'Ada',
    role: 2,
    token: 'session-token',
  })
  assert.equal(JSON.stringify(session).includes('secret-value'), false)
})

test('an entry without a token or with a password only is not saved', () => {
  assert.equal(sanitizeAccountSession({ username: 'ada@sun.cc', password: 'secret-value' }), null)
  assert.equal(sanitizeAccountSession({ id: 1, username: 'ada@sun.cc', token: '' }), null)
  assert.equal(sanitizeAccountSession(null), null)
})

test('remembering the same account replaces its session and keeps the newest first', () => {
  const first = rememberAccount([], { id: 1, username: 'admin@sun.cc', name: 'Admin', role: 1, token: 'admin-a' })
  const second = rememberAccount(first, { id: 2, username: 'ada@sun.cc', name: 'Ada', role: 2, token: 'ada-a', password: 'secret-value' })
  const replaced = rememberAccount(second, { id: 1, username: 'admin@sun.cc', name: 'Admin', role: 1, token: 'admin-b' })
  assert.deepEqual(replaced.map(item => item.token), ['admin-b', 'ada-a'])
  assert.equal(JSON.stringify(replaced).includes('secret-value'), false)
})

test('forgetting an account or an expired token leaves the others', () => {
  const list = rememberAccount(
    rememberAccount([], { id: 1, username: 'admin@sun.cc', name: 'Admin', role: 1, token: 'admin-a' }),
    { id: 2, username: 'ada@sun.cc', name: 'Ada', role: 2, token: 'ada-a' },
  )
  assert.deepEqual(forgetAccount(list, 2).map(item => item.id), [1])
  assert.deepEqual(forgetAccountToken(list, 'admin-a').map(item => item.id), [2])
})

test('stored text that is not a session list is ignored', () => {
  assert.deepEqual(readAccountSessions('{"password":"secret-value"}'), [])
  assert.deepEqual(readAccountSessions('not-json'), [])
  const raw = JSON.stringify([
    { id: 3, username: 'ada@sun.cc', name: 'Ada', role: 9, token: 'ada-a', password: 'secret-value' },
    { id: 3, username: 'duplicate', token: 'other' },
  ])
  const sessions = readAccountSessions(raw)
  assert.equal(sessions.length, 1)
  assert.equal(sessions[0].role, 2)
  assert.equal(JSON.stringify(sessions).includes('secret-value'), false)
})
