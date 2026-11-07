import assert from 'node:assert/strict'
import test from 'node:test'
import { fallbackLabel, isRemoteIconType, reduceRemoteIconPhase } from './remoteIcon.ts'

test('text icons are not remote and image or iconify icons are', () => {
  assert.equal(isRemoteIconType(1), false)
  assert.equal(isRemoteIconType(2), true)
  assert.equal(isRemoteIconType(3), true)
})

test('a failed remote icon stays on fallback and does not accept a later success', () => {
  const failed = reduceRemoteIconPhase('loading', 'error')
  assert.equal(failed, 'fallback')
  assert.equal(reduceRemoteIconPhase(failed, 'success'), 'fallback')
  assert.equal(reduceRemoteIconPhase(failed, 'error'), 'fallback')
})

test('a successful load can be shown', () => {
  assert.equal(reduceRemoteIconPhase('loading', 'success'), 'ready')
})

test('fallback keeps a custom text icon and otherwise uses the item title', () => {
  assert.equal(fallbackLabel(2, '图', '示例文档'), '图')
  assert.equal(fallbackLabel(2, '', '示例文档'), '示')
  assert.equal(fallbackLabel(3, 'mdi:not-real', '手册'), '手')
  assert.equal(fallbackLabel(3, '', ''), '?')
})
