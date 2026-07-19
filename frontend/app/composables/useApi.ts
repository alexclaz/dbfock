export function useApi() {
  const config = useRuntimeConfig()
  return async function api<T>(path: string, options: Parameters<typeof $fetch<T>>[1] = {}): Promise<T> {
    try { return await $fetch<T>(path, { baseURL: config.public.apiBase, ...options }) }
    catch (error: any) { throw new Error(error?.data?.error?.message || error?.message || 'Request failed') }
  }
}
