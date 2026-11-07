import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '../..')

function source(path: string): string {
  return readFileSync(join(root, path), 'utf8')
}

test('the search box uses the keyboard, address, and saved-engine rules', () => {
  const box = source('src/components/deskModule/SearchBox/index.vue')
  assert.match(box, /decideSearchKey\(/)
  assert.match(box, /buildSearchUrl\(/)
  assert.match(box, /normalizeSearchConfig\(/)
  assert.match(box, /toPersistedSearchConfig\(/)
  assert.match(box, /addCustomEngine\(/)
  assert.match(box, /moveEngine\(/)
  assert.doesNotMatch(box, /v-html/)
  assert.doesNotMatch(box, /innerHTML/)
})

test('the icon component resets through the remote icon controller when its address changes', () => {
  const icon = source('src/components/common/ItemIcon/index.vue')
  assert.match(icon, /createRemoteIconController\(/)
  assert.match(icon, /iconController\.sync\(/)
  assert.match(icon, /onImageEvent\(/)
  assert.match(icon, /data-request/)
})

test('cpu, memory, and disk cards mount and unmount through the shared poll hook', () => {
  for (const path of [
    'src/components/deskModule/SystemMonitor/AppIconSystemMonitor/CPU.vue',
    'src/components/deskModule/SystemMonitor/AppIconSystemMonitor/Memory.vue',
    'src/components/deskModule/SystemMonitor/AppIconSystemMonitor/Disk.vue',
  ]) {
    const card = source(path)
    assert.match(card, /attachMonitorPoll\(/)
    assert.match(card, /onMounted/)
    assert.match(card, /onUnmounted/)
    assert.doesNotMatch(card, /setInterval\(/)
  }
})
