import { post } from '@/utils/request'
import { publicFileQuery, type FileCategory, type FileListRequest } from '@/utils/file/category'

export function getList<T>(query?: Partial<FileListRequest>) {
  return post<T>({
    url: '/file/getList',
    data: query ?? {},
  })
}

export function getPublicList<T>(query?: Parameters<typeof publicFileQuery>[0]) {
  return post<T>({
    url: '/file/publicList',
    data: publicFileQuery(query ?? {}),
  })
}

export function setPublic<T>(id: number, visible: boolean) {
  return post<T>({
    url: '/file/setPublic',
    data: { id, public: visible },
  })
}

export function updateCategory<T>(id: number, category: string) {
  return post<T>({
    url: '/file/updateCategory',
    data: { id, category },
  })
}

export function deletes<T>(ids: number[]) {
  return post<T>({
    url: '/file/deletes',
    data: { ids },
  })
}

export interface BatchUploadResult {
  saved?: { fileName: string, imageUrl: string, category: string }[]
  errFiles?: string[]
  succMap?: Record<string, string>
}

// uploadFiles sends one batch. The category is the entry category.
// The form does not include an account id, and the server keeps the files on the logged-in account.
export function uploadFiles(files: File[], category: FileCategory) {
  const form = new FormData()
  for (const file of files)
    form.append('files[]', file)
  form.append('category', category)
  return post<BatchUploadResult>({
    url: '/file/uploadFiles',
    data: form,
  })
}
