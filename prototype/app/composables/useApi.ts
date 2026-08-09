import type { UseFetchOptions } from 'nuxt/app'

export function useApi<T>(uri: string, options: UseFetchOptions<T> = {}) {
  return useFetch(`/api${uri}`, options)
}
