import { post } from '@/utils/request'
import type { FileListRequest } from '@/utils/file/category'

export function getList<T>(query?: Partial<FileListRequest>) {
  return post<T>({
    url: '/file/getList',
    data: query ?? {},
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
