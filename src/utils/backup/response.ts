export function isZipContentType(value: string | undefined): boolean {
  return (value ?? '').toLowerCase().includes('application/zip')
}

export async function readBackupFailure(blob: Blob): Promise<string> {
  const text = (await blob.text()).slice(0, 500)
  try {
    const body = JSON.parse(text) as { msg?: unknown }
    if (typeof body.msg === 'string' && body.msg.trim() !== '')
      return body.msg.slice(0, 300)
  }
  catch {
    // 内容不是一段短的错误说明时，不把压缩包内容显示出来。
  }
  return '备份导出失败'
}
