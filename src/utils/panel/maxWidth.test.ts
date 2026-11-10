import assert from 'node:assert/strict'
import test from 'node:test'
import { contentMaxWidth, maxWidthControlValue, maxWidthFromControl } from './maxWidth.ts'

test('an existing max width stays a number, including zero', () => {
  assert.equal(maxWidthControlValue(960), 960)
  assert.equal(maxWidthFromControl(960), 960)
  assert.equal(contentMaxWidth(960, 'px'), '960px')

  assert.equal(maxWidthControlValue(0), 0)
  assert.equal(maxWidthFromControl(0), 0)
  assert.equal(contentMaxWidth(0, '%'), '0%')
})

test('clearing the max width control removes the number and the page uses 1200', () => {
  assert.equal(maxWidthControlValue(undefined), null)
  assert.equal(maxWidthFromControl(null), undefined)
  assert.equal(contentMaxWidth(undefined, 'px'), '1200px')
  assert.equal(contentMaxWidth(maxWidthFromControl(null), 'px'), '1200px')
})

test('a non-numeric max width is not written into the layout', () => {
  assert.equal(maxWidthControlValue(Number.NaN), null)
  assert.equal(maxWidthFromControl(Number.NaN), undefined)
  assert.equal(contentMaxWidth(Number.NaN, 'px'), '1200px')
  assert.equal(contentMaxWidth(1200, 'px'), '1200px')
})
