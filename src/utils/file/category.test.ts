import assert from 'node:assert/strict'
import test from 'node:test'
import { displayFileCategory, fileListRequest, pageAfterListChange, publicFileQuery } from './category.ts'

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
  assert.equal(fileListRequest({ keyword: '图'.repeat(90) }).keyword.length, 80)
})

test('a public gallery query drops any account id', () => {
  const query = publicFileQuery({
    page: 1,
    category: 'icon',
    keyword: '示例',
    userId: 8,
    user_id: 7,
  })
  assert.deepEqual(query, {
    page: 1,
    limit: 12,
    category: 'icon',
    keyword: '示例',
  })
  assert.equal(Object.hasOwn(query, 'userId'), false)
  assert.equal(Object.hasOwn(query, 'user_id'), false)
})

test('deleting the last file on the last page moves back to a page that still has files', () => {
  assert.equal(pageAfterListChange(2, 13, 12), 2)
  assert.equal(pageAfterListChange(2, 12, 12), 1)
  assert.equal(pageAfterListChange(3, 0, 12), 1)
  assert.equal(pageAfterListChange(0, 5, 12), 1)
})
