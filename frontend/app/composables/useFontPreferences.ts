export const interfaceFontOptions = [
  { value: 'inter', label: 'settings.fontInter' },
  { value: 'system', label: 'settings.fontSystem' },
  { value: 'avenir', label: 'settings.fontAvenir' },
  { value: 'ibm-plex', label: 'settings.fontIbmPlex' },
  { value: 'ibm-plex-mono', label: 'settings.fontIbmPlexMono' },
] as const

export const editorFontOptions = [
  { value: 'source-code-pro', label: 'settings.fontSourceCodePro' },
  { value: 'ibm-plex-mono', label: 'settings.fontIbmPlexMono' },
  { value: 'jetbrains-mono', label: 'settings.fontJetBrainsMono' },
  { value: 'sf-mono', label: 'settings.fontSfMono' },
] as const

export type InterfaceFont = typeof interfaceFontOptions[number]['value']
export type EditorFont = typeof editorFontOptions[number]['value']

const interfaceFontFamilies: Record<InterfaceFont, string> = {
  inter: 'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  system: 'ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  avenir: 'Avenir Next, Avenir, ui-sans-serif, system-ui, sans-serif',
  'ibm-plex': 'IBM Plex Sans, Avenir Next, ui-sans-serif, system-ui, sans-serif',
  'ibm-plex-mono': 'IBM Plex Mono, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
}

const editorFontFamilies: Record<EditorFont, string> = {
  'jetbrains-mono': 'JetBrains Mono, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
  'sf-mono': 'SF Mono, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
  'ibm-plex-mono': 'IBM Plex Mono, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
  'source-code-pro': 'Source Code Pro, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
}

const interfaceStorageKey = 'dbfock.interface-font'
const editorStorageKey = 'dbfock.editor-font'

function isInterfaceFont(value: string | null): value is InterfaceFont {
  return interfaceFontOptions.some((option) => option.value === value)
}

function isEditorFont(value: string | null): value is EditorFont {
  return editorFontOptions.some((option) => option.value === value)
}

export function useFontPreferences() {
  const interfaceFont = useState<InterfaceFont>('interface-font', () => 'inter')
  const editorFont = useState<EditorFont>('editor-font', () => 'source-code-pro')

  function applyFonts() {
    if (!import.meta.client) return
    document.documentElement.style.setProperty('--app-font', interfaceFontFamilies[interfaceFont.value])
    document.documentElement.style.setProperty('--code-font', editorFontFamilies[editorFont.value])
  }

  function setInterfaceFont(value: InterfaceFont) {
    interfaceFont.value = value
    if (!import.meta.client) return
    localStorage.setItem(interfaceStorageKey, value)
    applyFonts()
  }

  function setEditorFont(value: EditorFont) {
    editorFont.value = value
    if (!import.meta.client) return
    localStorage.setItem(editorStorageKey, value)
    applyFonts()
  }

  function restoreFontPreferences() {
    if (!import.meta.client) return
    const savedInterfaceFont = localStorage.getItem(interfaceStorageKey)
    const savedEditorFont = localStorage.getItem(editorStorageKey)
    if (isInterfaceFont(savedInterfaceFont)) interfaceFont.value = savedInterfaceFont
    if (isEditorFont(savedEditorFont)) editorFont.value = savedEditorFont
    applyFonts()
  }

  return { interfaceFont, editorFont, setInterfaceFont, setEditorFont, restoreFontPreferences }
}
