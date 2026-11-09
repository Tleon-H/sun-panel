// FormData fields are not enumerable. Object.assign into a plain object, and JSON.stringify, both yield {}.
// Axios would then post JSON, and the server's MultipartForm parse fails with code 1300.
// When data is already FormData, pass that object through and do not set Content-Type.
// The browser adds the multipart boundary.

export function prepareHttpRequest(input: {
  data?: any
  headers?: Record<string, any> | null
  token?: string | null
  lang?: string | null
}): { data: any, headers: Record<string, any> } {
  const raw = typeof input.data === 'function' ? input.data() : input.data
  const form = typeof FormData !== 'undefined' && raw instanceof FormData
  const data = form ? raw : Object.assign(raw ?? {}, {})
  const headers: Record<string, any> = { ...(input.headers ?? {}) }
  if (form) {
    delete headers['Content-Type']
    delete headers['content-type']
  }
  headers.token = input.token
  headers.lang = input.lang
  return { data, headers }
}
