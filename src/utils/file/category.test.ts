import assert from 'node:assert/strict'
import test from 'node:test'
import { displayFileCategory, fileListRequest } from './category.ts'

test('blank and unknown file categories stay uncategorized', () => {
  assert.equal(displayFileCategory(''), 'uncategorized')
  assert.equal(displayFileCategory(null), 'uncategorized')
  assert.equal(displayFileCategory('public'), 'uncategorized')
  assert.equal(displayFileCategory('icon'), 'icon')
  assert.equal(displayFileCategory('wallpaper'), 'wallpaper')
})

test('list requests keep paging, filename search, and only known categories', () => {
  assert.deepEqual(fileListRequest({}), {
    page: 1,
    limit: 12,
    category: '',
    keyword: '',
  })
  assert.deepEqual(fileListRequest({
    page: 2,
    limit: 1000,
    category: 'icon',
    keyword: '  示例壁纸  ',
  }), {
    page: 2,
    limit: 60,
    category: 'icon',
    keyword: '示例壁纸',
  })
  assert.equal(fileListRequest({ category: 'shared', keyword: '<img>' }).category, '')
  assert.equal(fileListRequest({ category: 'uncategorized' }).category, 'uncategorized')
})
