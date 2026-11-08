import assert from 'node:assert/strict'
import test from 'node:test'
import { isZipContentType, readBackupFailure } from './response.ts'

test('a zip response is accepted and an error message stays short', async () => {
  assert.equal(isZipContentType('application/zip'), true)
  assert.equal(isZipContentType('application/json; charset=utf-8'), false)
  const message = await readBackupFailure(new Blob([JSON.stringify({ msg: '当前使用的是 MySQL，这项功能不会导出数据库。' })]))
  assert.match(message, /MySQL/)
  const fallback = await readBackupFailure(new Blob(['not-json']))
  assert.equal(fallback, '备份导出失败')
  const huge = await readBackupFailure(new Blob([JSON.stringify({ msg: '测'.repeat(350) })]))
  assert.equal(huge.length, 300)
  const truncated = await readBackupFailure(new Blob([JSON.stringify({ msg: 'x'.repeat(800) })]))
  assert.equal(truncated, '备份导出失败')
})
