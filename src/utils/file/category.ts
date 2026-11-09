export const fileCategories = ['icon', 'wallpaper', 'uncategorized'] as const

export type FileCategory = typeof fileCategories[number]

export interface FileListRequest {
  page: number
  limit: number
  category: '' | FileCategory
  keyword: string
}

const defaultLimit = 12

export function displayFileCategory(value?: string | null): FileCategory {
  if (value === 'icon' || value === 'wallpaper' || value === 'uncategorized')
    return value
  return 'uncategorized'
}

export function pageAfterListChange(page: number, total: number, limit = defaultLimit): number {
  const size = limit > 0 ? Math.floor(limit) : defaultLimit
  const last = Math.max(1, Math.ceil(Math.max(0, total) / size))
  if (!page || page < 1)
    return 1
  return Math.min(Math.floor(page), last)
}

export function fileListRequest(input: {
  page?: number
  limit?: number
  category?: string
  keyword?: string
}): FileListRequest {
  const page = input.page && input.page > 0 ? Math.floor(input.page) : 1
  const limit = input.limit && input.limit > 0 ? Math.min(Math.floor(input.limit), 60) : defaultLimit
  const keyword = (input.keyword ?? '').trim().slice(0, 80)
  const category = input.category === 'icon' || input.category === 'wallpaper' || input.category === 'uncategorized'
    ? input.category
    : ''
  return { page, limit, category, keyword }
}

// publicFileQuery keeps only the gallery filters. An account id in the input is dropped.
export function publicFileQuery(input: {
  page?: number
  limit?: number
  category?: string
  keyword?: string
  userId?: unknown
  user_id?: unknown
}): FileListRequest {
  return fileListRequest({
    page: input.page,
    limit: input.limit,
    category: input.category,
    keyword: input.keyword,
  })
}
