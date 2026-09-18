export class ApiError extends Error {
  constructor(message: string, readonly status: number) { super(message) }
}

function apiError(cause: unknown): ApiError {
  const error = cause as { data?: { error?: { message?: string } }, message?: string, status?: number, statusCode?: number } | null
  return new ApiError(error?.data?.error?.message || error?.message || 'Request failed', error?.status ?? error?.statusCode ?? 0)
}

export function useApi() {
  const config = useRuntimeConfig()
  return async function api<T>(path: string, options: Parameters<typeof $fetch<T>>[1] = {}): Promise<T> {
    try { return await $fetch<T>(path, { baseURL: config.public.apiBase, ...options }) }
    catch (cause: unknown) { throw apiError(cause) }
  }
}
