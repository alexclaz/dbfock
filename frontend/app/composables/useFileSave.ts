type DesktopFilesBinding = { SaveTextFile?: (defaultName: string, contents: string) => Promise<string> }
type WailsBindings = { main?: { DesktopFiles?: DesktopFilesBinding } }

function desktopFiles() {
  if (!import.meta.client) return undefined
  const bindings = (window as Window & { go?: WailsBindings }).go
  const files = bindings?.main?.DesktopFiles
  return typeof files?.SaveTextFile === 'function' ? files : undefined
}

export function useFileSave() {
  const isDesktop = Boolean(desktopFiles())

  /**
   * Writes contents to disk. On the desktop build a native save dialog asks for
   * the destination; in the browser it falls back to a download.
   * Returns false when the user cancels the dialog.
   */
  async function saveTextFile(fileName: string, contents: string, mimeType: string) {
    const files = desktopFiles()
    if (files?.SaveTextFile) {
      const path = await files.SaveTextFile(fileName, contents)
      return Boolean(path)
    }

    const link = document.createElement('a')
    link.href = URL.createObjectURL(new Blob([contents], { type: mimeType }))
    link.download = fileName
    link.click()
    URL.revokeObjectURL(link.href)
    return true
  }

  return { isDesktop, saveTextFile }
}
