import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildDeleteRequest,
  clearFileSelection,
  deleteOwnFilesConfirm,
  deleteOwnFilesLimit,
  selectCurrentPage,
  selectionAfterAccountChange,
  selectionAfterDelete,
  selectionAfterQueryChange,
  toggleFileSelection,
  uniqueFileIds,
} from './selection.ts'

test('selection toggles one image and ignores invalid ids', () => {
  assert.deepEqual(toggleFileSelection([], 4, true), [4])
  assert.deepEqual(toggleFileSelection([4], 4, true), [4])
  assert.deepEqual(toggleFileSelection([4], 9, true), [4, 9])
  assert.deepEqual(toggleFileSelection([4, 9], 4, false), [9])
  assert.deepEqual(toggleFileSelection([4], 0, true), [4])
  assert.deepEqual(toggleFileSelection([4, 4, -1], 9, true), [4, 9])
  assert.deepEqual(uniqueFileIds([3, 3, 0, 8]), [3, 8])
})

test('selecting the current page replaces the selection and does not include another page', () => {
  const pageOne = [1, 2, 3]
  const pageTwo = [4, 5]
  const selected = selectCurrentPage(pageTwo)
  assert.deepEqual(selected, pageTwo)
  for (const id of pageOne)
    assert.equal(selected.includes(id), false)
  assert.deepEqual(selectCurrentPage([4, 4, 0, 5]), [4, 5])
  assert.deepEqual(clearFileSelection(), [])
})

test('search, category, page, and account changes drop the previous selection', () => {
  assert.deepEqual(selectionAfterQueryChange(), [])
  assert.deepEqual(selectionAfterAccountChange(7, 8, [1, 2]), [])
  assert.deepEqual(selectionAfterAccountChange(7, 7, [1, 1, 2]), [1, 2])
  assert.deepEqual(selectionAfterAccountChange(undefined, 7, [1]), [])
})

test('cancel does not build a delete request, and confirm sends each id once', () => {
  assert.equal(buildDeleteRequest([1, 2], false), null)
  assert.equal(buildDeleteRequest([], true), null)
  assert.equal(buildDeleteRequest([0, -3], true), null)
  const request = buildDeleteRequest([2, 2, 5], true)
  assert.deepEqual(request, {
    ids: [2, 5],
    confirm: deleteOwnFilesConfirm,
  })
  assert.equal(deleteOwnFilesConfirm, 'delete-own-files')
  const oversized = Array.from({ length: deleteOwnFilesLimit + 1 }, (_, index) => index + 1)
  assert.equal(buildDeleteRequest(oversized, true), null)
})

test('after a delete, removed images and images that are no longer visible are not left selected', () => {
  assert.deepEqual(selectionAfterDelete([1, 2, 3], [1], [2, 3]), [2, 3])
  assert.deepEqual(selectionAfterDelete([1, 2, 3], [1], [3]), [3])
  assert.deepEqual(selectionAfterDelete([1, 2], [1, 2], []), [])
  assert.deepEqual(selectionAfterDelete([8], [], [1, 2]), [])
})
