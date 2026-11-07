import assert from 'node:assert/strict'
import test from 'node:test'
import {
  addCustomEngine,
  buildSearchUrl,
  defaultSearchConfig,
  moveEngine,
  normalizeSearchConfig,
  removeCustomEngine,
  toPersistedSearchConfig,
  updateCustomEngine,
} from './engines.ts'

const example = 'https://example.com/search?q=%s'
const baidu = 'https://www.baidu.com/s?wd=%s'

test('an empty or old configuration keeps Google, Baidu, and Bing and the saved choice', () => {
  const empty = normalizeSearchConfig(null)
  assert.deepEqual(empty.engines.map(engine => engine.id), ['google', 'baidu', 'bing'])
  assert.equal(empty.currentId, 'google')

  const old = normalizeSearchConfig({
    currentSearchEngine: { title: 'Baidu', url: baidu, iconSrc: '/assets/baidu.svg' },
    searchEngineList: [],
    newWindowOpen: true,
  })
  assert.equal(old.currentId, 'baidu')
  assert.equal(old.newWindowOpen, true)
  assert.equal(old.engines.length, 3)

  const blankList = normalizeSearchConfig({ engines: [], currentSearchEngine: { title: 'Baidu', url: baidu } })
  assert.equal(blankList.currentId, 'baidu')
  assert.deepEqual(blankList.engines.map(engine => engine.id), ['google', 'baidu', 'bing'])
})

test('custom engines can be added, edited, reordered, removed, and restored from the saved object', () => {
  const added = addCustomEngine(defaultSearchConfig(), '示例搜索', example, 'custom-example')
  assert.equal(added.ok, true)
  if (!added.ok)
    return
  const edited = updateCustomEngine(added.config, 'custom-example', '示例搜索二', 'https://example.com/find?q=%s')
  assert.equal(edited.ok, true)
  if (!edited.ok)
    return
  const moved = moveEngine(edited.config, edited.config.engines.length - 1, -1)
  assert.equal(moved.engines.at(-2)?.id, 'custom-example')
  const saved = toPersistedSearchConfig(moved)
  const restored = normalizeSearchConfig(saved)
  assert.equal(restored.engines.at(-2)?.title, '示例搜索二')
  assert.equal(restored.engines.at(-2)?.url, 'https://example.com/find?q=%s')
  assert.equal(restored.currentId, 'google')

  const removed = removeCustomEngine(restored, 'custom-example')
  assert.equal(removed.ok, true)
  if (removed.ok)
    assert.equal(removed.config.engines.some(engine => engine.id === 'custom-example'), false)
  assert.equal(removeCustomEngine(defaultSearchConfig(), 'google').ok, false)
})

test('an old custom list is kept and an illegal template is dropped', () => {
  const restored = normalizeSearchConfig({
    currentSearchEngine: { title: 'Google', url: 'https://www.google.com/search?q=%s', iconSrc: '' },
    searchEngineList: [
      { title: '示例搜索', url: example, iconSrc: 'https://example.com/icon.svg' },
      { title: '坏模板', url: 'javascript:alert(1)', iconSrc: '' },
    ],
  })
  assert.deepEqual(restored.engines.map(engine => engine.url).slice(3), [example])
  assert.equal(addCustomEngine(restored, '<b>名称</b>', example).ok, false)
  assert.equal(addCustomEngine(restored, '重复', example).ok, false)
})

test('the query is URL-encoded and dangerous protocols are rejected', () => {
  assert.equal(buildSearchUrl(example, 'a b&中'), 'https://example.com/search?q=a%20b%26%E4%B8%AD')
  assert.equal(buildSearchUrl('https://example.com/?q=%s&alt=%s', 'a b'), 'https://example.com/?q=a%20b&alt=a%20b')
  assert.equal(buildSearchUrl('https://example.com/search?q=', 'a b'), 'https://example.com/search?q=a%20b')
  assert.equal(buildSearchUrl('https://example.com/search', ''), 'https://example.com/search')
  assert.equal(buildSearchUrl(example, '<script>'), 'https://example.com/search?q=%3Cscript%3E')
  for (const template of [
    'javascript:alert(1)',
    'JavaScript:alert(1)',
    'data:text/html,hello',
    'vbscript:msgbox(1)',
    'file:///tmp/example',
    'blob:https://example.com/11111111-1111-1111-1111-111111111111',
    'https://example.com/search?q=<script>',
    'https://example.com/search? q=%s',
  ])
    assert.equal(buildSearchUrl(template, '词'), null)
})
