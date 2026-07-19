export type ToastTone = 'success' | 'error' | 'info'

type Toast = { id: number; message: string; tone: ToastTone }

export function useToast() {
  const toasts = useState<Toast[]>('app-toasts', () => [])

  function dismiss(id: number) { toasts.value = toasts.value.filter((toast) => toast.id !== id) }
  function show(message: string, tone: ToastTone = 'info', duration = 4000) {
    const id = Date.now() + Math.floor(Math.random() * 1000)
    toasts.value.push({ id, message, tone })
    if (import.meta.client) window.setTimeout(() => dismiss(id), duration)
    return id
  }

  return { toasts, dismiss, show, success: (message: string) => show(message, 'success'), error: (message: string) => show(message, 'error'), info: (message: string) => show(message, 'info') }
}
