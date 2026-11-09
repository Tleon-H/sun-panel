export const restoreConfirmValue = 'overwrite'

export function buildRestoreForm(file: File) {
  const form = new FormData()
  form.append('file', file)
  form.append('confirm', restoreConfirmValue)
  return form
}
