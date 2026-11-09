import { post } from '@/utils/request'
import { publicFileQuery, type FileListRequest } from '@/utils/file/category'

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
