import assert from 'node:assert/strict'
import test from 'node:test'
import { buildRestoreForm, restoreConfirmValue } from './restore.ts'

test('restore sends the backup only together with an explicit overwrite confirmation', () => {
  const file = new File(['zip'], 'leon-panel-backup.zip', { type: 'application/zip' })
  const form = buildRestoreForm(file)
  const entries = [...form.entries()].map(([name, value]) => {
    return [name, typeof value === 'string' ? value : value.name]
  })
  assert.deepEqual(entries, [
    ['file', 'leon-panel-backup.zip'],
    ['confirm', restoreConfirmValue],
  ])
  assert.equal(restoreConfirmValue, 'overwrite')
})
