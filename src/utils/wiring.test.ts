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

test('image uploads and the file manager use the category metadata', () => {
  const manager = source('src/components/apps/UploadFileManager/index.vue')
  assert.match(manager, /ownFileQuery\(/)
  assert.match(manager, /uploadFiles\(/)
  assert.match(manager, /uploadCategoryForEntry\('other'\)/)
  assert.match(manager, /multiple/)
  assert.match(manager, /updateCategory\(/)
  assert.match(manager, /displayFileCategory\(/)
  assert.doesNotMatch(manager, /v-html/)
  assert.match(source('src/components/apps/Style/index.vue'), /category: 'wallpaper'/)
  assert.match(source('src/views/home/components/EditItem/IconEditor.vue'), /category: 'icon'/)
  assert.match(source('service/api/api_v1/panel/itemIcon.go'), /FileCategoryIcon/)
})

test('the own gallery lists only the current account and stays beside the public gallery', () => {
  const own = source('src/components/apps/OwnGallery/index.vue')
  const gallery = source('src/components/apps/PublicGallery/index.vue')
  assert.match(own, /getList/)
  assert.match(own, /ownFileQuery\(/)
  assert.doesNotMatch(own, /getPublicList/)
  assert.doesNotMatch(own, /setPublic\(/)
  assert.doesNotMatch(own, /v-html/)
  assert.match(gallery, /getPublicList/)
  assert.match(gallery, /publicFileQuery\(/)
  assert.match(source('src/views/home/components/EditItem/IconEditor.vue'), /open-own-icons/)
  assert.match(source('src/views/home/components/EditItem/IconEditor.vue'), /open-public-icons/)
  assert.match(source('src/views/home/components/EditItem/IconEditor.vue'), /locked-category="icon"/)
  assert.match(source('src/components/apps/Style/index.vue'), /open-own-wallpapers/)
  assert.match(source('src/components/apps/Style/index.vue'), /open-public-wallpapers/)
  assert.match(source('src/components/apps/Style/index.vue'), /locked-category="wallpaper"/)
  assert.match(source('service/api/api_v1/system/file.go'), /allowedImageExt/)
})

test('the public gallery is separate from private files and only an administrator can mark one', () => {
  const gallery = source('src/components/apps/PublicGallery/index.vue')
  const manager = source('src/components/apps/UploadFileManager/index.vue')
  assert.match(gallery, /getPublicList/)
  assert.match(gallery, /publicFileQuery\(/)
  assert.doesNotMatch(gallery, /v-html/)
  assert.match(manager, /setPublic\(/)
  assert.match(manager, /isAdmin/)
  assert.match(source('src/views/home/components/EditItem/IconEditor.vue'), /locked-category="icon"/)
  assert.match(source('src/components/apps/Style/index.vue'), /locked-category="wallpaper"/)
  assert.match(source('service/api/api_v1/system/file.go'), /PublicFileListScope/)
  assert.match(source('service/models/file.go'), /SetOwnPublic/)
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
