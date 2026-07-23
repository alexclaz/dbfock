<script setup lang="ts">
import '@fontsource/source-code-pro/400.css'
import '@fontsource/source-code-pro/700.css'
import '@fontsource/ibm-plex-sans/400.css'
import '@fontsource/ibm-plex-sans/600.css'
import '@fontsource/ibm-plex-sans/700.css'
import '@fontsource/ibm-plex-mono/400.css'
import '@fontsource/ibm-plex-mono/700.css'

type ThemePreference = 'dbfock-light' | 'dbfock-dark' | 'github-light' | 'github-dark' | 'one-dark' | 'dracula' | 'cobalt2' | 'claude-code' | 'supabase' | 'monokai' | 'vscode-light' | 'vscode-dark'

const theme = useState<ThemePreference>('theme-preference', () => 'vscode-dark')
const { restoreLocale } = useI18n()
const { restoreFontPreferences } = useFontPreferences()

function confirmLeaving(event: BeforeUnloadEvent) {
  // Browsers intentionally provide the dialog text to prevent abusive custom prompts.
  event.preventDefault()
  event.returnValue = true
}

function applyTheme() {
  if (!import.meta.client) return
  const dark = theme.value !== 'dbfock-light' && theme.value !== 'github-light' && theme.value !== 'vscode-light'
  document.documentElement.dataset.theme = theme.value
  document.documentElement.classList.toggle('dark', dark)
}

useHead({
  titleTemplate: (title) => title ? `${title} · DBfock` : 'DBfock',
  link: [
    { rel: 'icon', type: 'image/x-icon', href: '/branding/favicon/favicon.ico' },
    { rel: 'icon', type: 'image/png', sizes: '16x16', href: '/branding/favicon/favicon-16x16.png' },
    { rel: 'icon', type: 'image/png', sizes: '32x32', href: '/branding/favicon/favicon-32x32.png' },
    { rel: 'apple-touch-icon', sizes: '180x180', href: '/branding/favicon/apple-touch-icon.png' },
    { rel: 'manifest', href: '/branding/favicon/site.webmanifest' },
  ],
})
watch(theme, () => { if (import.meta.client) { localStorage.setItem('dbfock.theme', theme.value); applyTheme() } })
onMounted(() => {
  restoreLocale()
  restoreFontPreferences()
  const saved = localStorage.getItem('dbfock.theme') ?? localStorage.getItem('theme-mode')
  if (saved === 'codex') theme.value = 'supabase'
  else if (saved === 'dbfock-light' || saved === 'dbfock-dark' || saved === 'github-light' || saved === 'github-dark' || saved === 'one-dark' || saved === 'dracula' || saved === 'cobalt2' || saved === 'claude-code' || saved === 'supabase' || saved === 'monokai' || saved === 'vscode-light' || saved === 'vscode-dark') theme.value = saved
  else if (saved === 'light') theme.value = 'github-light'
  else if (saved === 'dark') theme.value = 'vscode-dark'
  else if (saved === 'auto' || saved === 'system') theme.value = 'vscode-dark'
  window.addEventListener('beforeunload', confirmLeaving)
  applyTheme()
})
onBeforeUnmount(() => window.removeEventListener('beforeunload', confirmLeaving))
</script>

<template><NuxtLayout><NuxtPage /></NuxtLayout><AppToast /></template>
