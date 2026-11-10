export const deleteOwnFilesConfirm = 'delete-own-files'

export const deleteOwnFilesLimit = 100

export interface FileDeleteFailure {
  id: number
  reason: string
}

export interface FileDeleteResult {
  deleted?: number[]
  failed?: FileDeleteFailure[]
}

export interface FileDeleteRequest {
  ids: number[]
  confirm: string
}

export function uniqueFileIds(ids: number[]): number[] {
  const seen = new Set<number>()
  const result: number[] = []
  for (const id of ids) {
    if (!Number.isInteger(id) || id <= 0 || seen.has(id))
      continue
    seen.add(id)
    result.push(id)
  }
  return result
}

export function toggleFileSelection(selected: number[], id: number, checked: boolean): number[] {
  const current = uniqueFileIds(selected)
  if (!Number.isInteger(id) || id <= 0)
    return current
  if (!checked)
    return current.filter(item => item !== id)
  if (current.includes(id))
    return current
  return [...current, id]
}

// selectCurrentPage replaces the selection with the ids on this page.
// It does not keep ids from any other page.
export function selectCurrentPage(pageIds: number[]): number[] {
  return uniqueFileIds(pageIds)
}

export function clearFileSelection(): number[] {
  return []
}

export function selectionAfterQueryChange(): number[] {
  return []
}

export function selectionAfterAccountChange(previousId: number | null | undefined, nextId: number | null | undefined, selected: number[]): number[] {
  if (previousId !== nextId)
    return []
  return uniqueFileIds(selected)
}

// buildDeleteRequest returns null unless the user has confirmed.
// A null result must not be sent to the server.
export function buildDeleteRequest(ids: number[], confirmed: boolean): FileDeleteRequest | null {
  if (!confirmed)
    return null
  const unique = uniqueFileIds(ids)
  if (unique.length === 0 || unique.length > deleteOwnFilesLimit)
    return null
  return { ids: unique, confirm: deleteOwnFilesConfirm }
}

export function selectionAfterDelete(selected: number[], deleted: number[], visibleIds: number[]): number[] {
  const gone = new Set(deleted)
  const visible = new Set(visibleIds)
  return uniqueFileIds(selected).filter(id => !gone.has(id) && visible.has(id))
}
