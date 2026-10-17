import type { UseFetchOptions } from 'nuxt/app'
import type { MaybeRefOrGetter } from 'vue'

// Accepts a reactive URL so a page can refetch when its query (e.g. `?as=`) changes.
export function useApi<T>(uri: MaybeRefOrGetter<string>, options: UseFetchOptions<T> = {}) {
  return useFetch(() => `/api${toValue(uri)}`, options)
}
