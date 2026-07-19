export const textScaleOptions = [
  { value: 0.8, label: 'settings.fontSizeExtraSmall' },
  { value: 0.9, label: 'settings.fontSizeSmall' },
  { value: 1, label: 'settings.fontSizeNormal' },
  { value: 1.25, label: 'settings.fontSizeLarge' },
  { value: 1.5, label: 'settings.fontSizeExtraLarge' },
] as const

const storageKey = 'dbfock.text-scale'

export function useTextScale() {
  const textScale = useState<number>('text-scale', () => 1)

  function applyTextScale() {
    if (!import.meta.client) return
    document.documentElement.style.fontSize = `${Math.round(textScale.value * 100)}%`
    document.documentElement.style.setProperty('--ide-editor-font-size', `${13 * textScale.value}px`)
    document.documentElement.style.setProperty('--ide-editor-line-height', `${22 * textScale.value}px`)
    localStorage.setItem(storageKey, String(textScale.value))
  }

  function setTextScale(value: number) {
    textScale.value = value
    applyTextScale()
  }

  function adjustTextScale(amount: number) {
    const currentIndex = textScaleOptions.reduce((closest, option, index) => Math.abs(option.value - textScale.value) < Math.abs(textScaleOptions[closest]!.value - textScale.value) ? index : closest, 0)
    const nextIndex = Math.min(textScaleOptions.length - 1, Math.max(0, currentIndex + Math.sign(amount)))
    setTextScale(textScaleOptions[nextIndex]!.value)
  }

  function restoreTextScale() {
    if (!import.meta.client) return
    const saved = Number(localStorage.getItem(storageKey))
    if (saved >= 0.8 && saved <= 1.5) textScale.value = saved
    applyTextScale()
  }

  return { textScale, setTextScale, adjustTextScale, restoreTextScale }
}
