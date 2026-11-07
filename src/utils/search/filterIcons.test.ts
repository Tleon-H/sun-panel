import assert from 'node:assert/strict'
import test from 'node:test'
import { decideSearchConfirm, decideSearchKey, filterIconGroups, moveMatchIndex } from './filterIcons.ts'

const groups = [
  {
    title: '常用',
    items: [
      { title: '示例文档', url: 'https://example.com/docs', description: '说明' },
      { title: '相册', url: 'https://example.com/photos', description: '图片' },
    ],
  },
]

test('search matches title, address, and description without adding groups', () => {
  const byTitle = filterIconGroups(groups, ' 文档 ', true)
  assert.equal(byTitle.active, true)
  assert.equal(byTitle.groups.length, 1)
  assert.equal(byTitle.groups[0].title, '常用')
  assert.deepEqual(byTitle.matches.map(item => item.title), ['示例文档'])

  const byUrl = filterIconGroups(groups, 'PHOTOS', true)
  assert.deepEqual(byUrl.matches.map(item => item.title), ['相册'])

  const byDescription = filterIconGroups(groups, '说明', true)
  assert.deepEqual(byDescription.matches.map(item => item.title), ['示例文档'])
})

test('blank keyword or disabled local search leaves the original groups', () => {
  assert.equal(filterIconGroups(groups, '   ', true).active, false)
  assert.equal(filterIconGroups(groups, '文档', false).groups, groups)
})

test('no match is an empty result and does not invent a group', () => {
  const result = filterIconGroups(groups, '不存在的词', true)
  assert.equal(result.active, true)
  assert.deepEqual(result.groups, [])
  assert.deepEqual(result.matches, [])
})

test('arrow keys wrap the current match and enter opens a local match when one exists', () => {
  assert.equal(moveMatchIndex(-1, 1, 3), 0)
  assert.equal(moveMatchIndex(0, -1, 3), 2)
  assert.equal(moveMatchIndex(2, 1, 3), 0)
  assert.equal(moveMatchIndex(0, 1, 0), -1)
  assert.equal(decideSearchConfirm({ keyword: '文档', localSearchEnabled: true, matchCount: 1 }), 'open-local')
  assert.equal(decideSearchConfirm({ keyword: '没有', localSearchEnabled: true, matchCount: 0 }), 'open-engine')
  assert.equal(decideSearchConfirm({ keyword: '文档', localSearchEnabled: false, matchCount: 2 }), 'open-engine')
})

test('typing, external search, and IME composition are not captured by the search keys', () => {
  const base = { localSearchEnabled: true, matchCount: 2 }
  assert.deepEqual(decideSearchKey({ ...base, key: 'a' }), { type: 'ignore' })
  assert.deepEqual(decideSearchKey({ ...base, key: 'Enter', isComposing: true }), { type: 'ignore' })
  assert.deepEqual(decideSearchKey({ ...base, key: 'Enter', keyCode: 229 }), { type: 'ignore' })
  assert.deepEqual(decideSearchKey({ ...base, key: 'Process' }), { type: 'ignore' })
  assert.deepEqual(decideSearchKey({ ...base, key: 'ArrowDown', localSearchEnabled: false, matchCount: 2 }), { type: 'ignore' })
  assert.deepEqual(decideSearchKey({ ...base, key: 'ArrowUp', matchCount: 0 }), { type: 'ignore' })
  assert.deepEqual(decideSearchKey({ ...base, key: 'ArrowDown' }), { type: 'move', delta: 1 })
  assert.deepEqual(decideSearchKey({ ...base, key: 'Enter' }), { type: 'confirm' })
  assert.deepEqual(decideSearchKey({ ...base, key: 'Escape' }), { type: 'clear' })
  assert.equal(decideSearchConfirm({ keyword: '文档', localSearchEnabled: true, matchCount: 0 }), 'open-engine')
})
