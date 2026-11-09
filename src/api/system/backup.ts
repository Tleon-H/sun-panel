import service from '@/utils/request/axios'
import { post } from '@/utils/request'
import { useAuthStore } from '@/store'
import { buildRestoreForm } from '@/utils/backup/restore'
import { isZipContentType, readBackupFailure } from '@/utils/backup/response'

export async function downloadBackup() {
  const token = useAuthStore().token
  const response = await service.post('/backup/export', {}, {
    responseType: 'blob',
    timeout: 120000,
    headers: {
      token: token ?? '',
    },
  })
  const headerType = String(response.headers?.['content-type'] ?? '')
  const blob = response.data as Blob
  if (!isZipContentType(headerType) && !isZipContentType(blob.type))
    throw new Error(await readBackupFailure(blob))

  const stamp = new Date().toISOString().replace(/[:.]/g, '-')
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `leon-panel-backup-${stamp}.zip`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

export async function restoreBackup(file: File) {
  const result = await post({
    url: '/backup/restore',
    data: buildRestoreForm(file),
  })
  if (result.code !== 0)
    throw new Error(result.msg || '恢复失败')
}
