import type { NitroFetchOptions, NitroFetchRequest } from 'nitropack'

type WithoutPrefix<T, P extends string> = T extends `${P}${infer Rest}` ? Rest : never
export type ApiUri = WithoutPrefix<Extract<NitroFetchRequest, `/api/${string}`>, '/api'> | string

export function $api<T>(request: ApiUri, options?: NitroFetchOptions<ApiUri>) {
  return useRequestFetch()<T, ApiUri>(`/api${request}`, options)
}
