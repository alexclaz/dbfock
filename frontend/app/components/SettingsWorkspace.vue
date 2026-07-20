<script setup lang="ts">
import type { Locale } from '~/composables/useI18n'
import { DBeaverNoMySQLConnectionsError, parseDBeaverProject, type DBeaverMySQLConnection } from '~/utils/dbeaverProject'

type Provider = 'openai' | 'openrouter' | 'anthropic' | 'ollama'
type ThemePreference = 'github-light' | 'github-dark' | 'one-dark' | 'dracula' | 'cobalt2' | 'claude-code' | 'codex' | 'monokai' | 'vscode-light' | 'vscode-dark'
type SettingsSection = 'appearance' | 'shortcuts' | 'connections' | 'ai' | 'audit' | 'backup' | 'about'
type ProviderOption = { value: Provider; label: string; defaultModel: string; baseUrl: string; apiKeyHint: string }
type AISettings = { configured: boolean; provider?: Provider; model?: string; baseUrl?: string; hasApiKey?: boolean }
type AIAuditLog = { id: string; runId: string; question: string; stage: string; provider: string; model: string; request: string; response: string; error: string; createdAt: string }
type AIAuditRun = { id: string; question: string; createdAt: string; logs: AIAuditLog[] }
type BackupSettings = { configured: boolean; endpoint?: string; bucket?: string; region?: string; hasAccessKey?: boolean; hasSecret?: boolean }
type BackupItem = { key: string; createdAt: string; size: number }
type ConnectionExport = { version: number; connections: unknown[] }
type ConnectionImportResult = { imported: number }
type ConnectionImport = { name: string; driver: 'mysql'; host: string; port: number; username: string; initialDatabase: string; color: string; environment: 'development'; sslEnabled: boolean; timeoutSeconds: number }

const providerOptions: ProviderOption[] = [
  { value: 'openai', label: 'OpenAI', defaultModel: 'gpt-5.4', baseUrl: 'https://api.openai.com/v1', apiKeyHint: 'sk-…' },
  { value: 'openrouter', label: 'OpenRouter', defaultModel: 'openai/gpt-5-mini', baseUrl: 'https://openrouter.ai/api/v1', apiKeyHint: 'sk-or-v1-…' },
  { value: 'anthropic', label: 'Anthropic', defaultModel: 'claude-sonnet-4-5', baseUrl: 'https://api.anthropic.com/v1', apiKeyHint: 'sk-ant-…' },
  { value: 'ollama', label: 'Ollama', defaultModel: 'llama3.2', baseUrl: 'http://localhost:11434', apiKeyHint: '' },
]
const appVersion = '0.5.2'
const license = 'MIT'
const githubUrl = 'https://github.com/alexclaz/dbfock'

const props = defineProps<{ section?: SettingsSection }>()
const emit = defineEmits<{ 'ai-configured': [] }>()
const api = useApi()
const workspace = useWorkspaceStore()
const { locale, setLocale, t, locales } = useI18n()
const { error: notifyError, success: notifySuccess } = useToast()
const activeSection = ref<SettingsSection>(props.section || 'appearance')
const theme = useState<ThemePreference>('theme-preference', () => 'vscode-dark')
const { textScale, setTextScale } = useTextScale()
const shortcuts = [
  { label: 'settings.shortcutGlobalSearch', keys: '⌘K / Ctrl+K' },
  { label: 'settings.shortcutSave', keys: '⌘S / Ctrl+S' },
  { label: 'settings.shortcutCloseTab', keys: '⌘W / Ctrl+W' },
  { label: 'settings.shortcutPreviousTab', keys: '⌘9 / Ctrl+9' },
  { label: 'settings.shortcutNextTab', keys: '⌘0 / Ctrl+0' },
  { label: 'settings.shortcutIncreaseText', keys: '⌘+ / Ctrl+' },
  { label: 'settings.shortcutDecreaseText', keys: '⌘- / Ctrl-' },
  { label: 'settings.shortcutRunBlock', keys: '⌘↵ / Ctrl+Enter' },
  { label: 'settings.shortcutRunNewResult', keys: '⌘\\ / Ctrl+\\' },
  { label: 'settings.shortcutCommentBlock', keys: '⌘/ / Ctrl+/' },
] as const
const themeOptions: { value: ThemePreference; label: string; description: string; preview: string[] }[] = [
  { value: 'vscode-dark', label: 'theme.vscodeDark', description: 'theme.vscodeDarkDescription', preview: ['#1e1e1e', '#252526', '#007acc'] },
  { value: 'vscode-light', label: 'theme.vscodeLight', description: 'theme.vscodeLightDescription', preview: ['#ffffff', '#f3f3f3', '#0078d4'] },
  { value: 'github-light', label: 'theme.githubLight', description: 'theme.githubLightDescription', preview: ['#ffffff', '#f6f8fa', '#0969da'] },
  { value: 'github-dark', label: 'theme.githubDark', description: 'theme.githubDarkDescription', preview: ['#0d1117', '#161b22', '#2f81f7'] },
  { value: 'one-dark', label: 'theme.oneDark', description: 'theme.oneDarkDescription', preview: ['#21252b', '#282c34', '#61afef'] },
  { value: 'dracula', label: 'theme.dracula', description: 'theme.draculaDescription', preview: ['#282a36', '#44475a', '#bd93f9'] },
  { value: 'cobalt2', label: 'theme.cobalt2', description: 'theme.cobalt2Description', preview: ['#193549', '#1d3b51', '#ffc600'] },
  { value: 'claude-code', label: 'theme.claudeCode', description: 'theme.claudeCodeDescription', preview: ['#1c1b1a', '#272522', '#d97757'] },
  { value: 'codex', label: 'theme.codex', description: 'theme.codexDescription', preview: ['#111111', '#1a1a1a', '#10a37f'] },
  { value: 'monokai', label: 'theme.monokai', description: 'theme.monokaiDescription', preview: ['#272822', '#3e3d32', '#a6e22e'] },
]
const form = reactive({ provider: 'openai' as Provider, model: 'gpt-5.4', baseUrl: 'https://api.openai.com/v1', apiKey: '' })
const models = ref<string[]>([])
const error = ref('')
const modelsError = ref('')
const loadingModels = ref(false)
const saving = ref(false)
const hasSavedKey = ref(false)
const auditLogs = ref<AIAuditLog[]>([])
const loadingAudit = ref(false)
const auditError = ref('')
const expandedAuditRuns = reactive(new Set<string>())
const backupForm = reactive({ endpoint: '', bucket: '', region: '', accessKey: '', secret: '' })
const hasSavedBackupAccessKey = ref(false)
const hasSavedBackupSecret = ref(false)
const backupError = ref('')
const backupSuccess = ref('')
const savingBackup = ref(false)
const runningBackup = ref(false)
const restoringBackup = ref(false)
const deletingBackupKey = ref('')
const backups = ref<BackupItem[]>([])
const selectedBackupKey = ref('')
const importInput = ref<HTMLInputElement>()
const dbeaverImportInput = ref<HTMLInputElement>()
const connectionTransferError = ref('')
const connectionTransferSuccess = ref('')
const importingConnections = ref(false)
const dbeaverConnections = ref<DBeaverMySQLConnection[]>([])
const dbeaverDefaultUsername = ref('')
const dbeaverImportError = ref('')
const dbeaverImportSuccess = ref('')
const parsingDBeaverProject = ref(false)
const importingDBeaverConnections = ref(false)
let loadTimer: ReturnType<typeof setTimeout> | undefined

const auditRuns = computed<AIAuditRun[]>(() => {
  const runs = new Map<string, AIAuditRun>()
  for (const log of auditLogs.value) {
    const id = log.runId || log.id
    const run = runs.get(id)
    if (run) run.logs.push(log)
    else runs.set(id, { id, question: log.question, createdAt: log.createdAt, logs: [log] })
  }
  return [...runs.values()].map((run) => ({ ...run, logs: [...run.logs].sort((a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()) }))
})

const selectedProvider = computed(() => providerOptions.find(({ value }) => value === form.provider)!)
const apiKeyRequired = computed(() => form.provider !== 'ollama')
const appliedTextScaleIndex = computed(() => textScaleOptions.reduce((closest, option, index) => Math.abs(option.value - textScale.value) < Math.abs(textScaleOptions[closest]!.value - textScale.value) ? index : closest, 0))
const pendingTextScaleIndex = ref(appliedTextScaleIndex.value)
const textScaleIndex = computed({
  get: () => pendingTextScaleIndex.value,
  set: (index: number) => { pendingTextScaleIndex.value = index },
})

function setProvider(provider: Provider) { const option = providerOptions.find(({ value }) => value === provider)!; form.baseUrl = option.baseUrl; form.model = option.defaultModel; form.apiKey = ''; hasSavedKey.value = false; models.value = []; modelsError.value = ''; scheduleModelLoad() }
function commitTextScale(event: Event) { if ((event.target as HTMLInputElement).id === 'font-size') setTextScale(textScaleOptions[pendingTextScaleIndex.value]!.value) }
function scheduleModelLoad() { if (loadTimer) clearTimeout(loadTimer); models.value = []; modelsError.value = ''; if (apiKeyRequired.value && !form.apiKey.trim()) return; loadTimer = setTimeout(loadModels, 450) }
async function loadModels() {
  if (apiKeyRequired.value && !form.apiKey.trim()) return
  loadingModels.value = true; modelsError.value = ''
  try { const response = await api<{ models: string[] }>('/ai/models', { method: 'POST', body: form }); models.value = response.models; if (models.value.length && !models.value.includes(form.model)) form.model = models.value[0]!; if (!models.value.length) modelsError.value = t('ai.noModels') }
  catch (cause: any) { modelsError.value = cause.message || t('ai.loadModelsError') }
  finally { loadingModels.value = false }
}
async function loadSettings() {
  error.value = ''
  try { const settings = await api<AISettings>('/ai/settings'); if (settings.configured === false) return; if (!settings.provider || !settings.model || !settings.baseUrl) throw new Error(t('ai.incompleteSettings')); form.provider = settings.provider; form.model = settings.model; form.baseUrl = settings.baseUrl; form.apiKey = ''; hasSavedKey.value = Boolean(settings.hasApiKey) }
  catch (cause: any) { error.value = cause.message || t('ai.incompleteSettings') }
}
async function save() { error.value = ''; saving.value = true; try { await api('/ai/settings', { method: 'PUT', body: form }); emit('ai-configured'); notifySuccess(t('common.save')) } catch (cause: any) { error.value = cause.message } finally { saving.value = false } }
async function loadAuditLogs() {
  loadingAudit.value = true; auditError.value = ''
  try { auditLogs.value = (await api<AIAuditLog[]>('/ai/audit-logs?limit=100')) ?? [] }
  catch (cause: any) { auditError.value = cause.message || t('audit.loadError') }
  finally { loadingAudit.value = false }
}
async function loadBackupSettings() {
  backupError.value = ''
  try {
    const settings = await api<BackupSettings>('/backup/settings')
    if (!settings.configured) return
    backupForm.endpoint = settings.endpoint || ''; backupForm.bucket = settings.bucket || ''; backupForm.region = settings.region || ''; backupForm.accessKey = ''; backupForm.secret = ''
    hasSavedBackupAccessKey.value = Boolean(settings.hasAccessKey); hasSavedBackupSecret.value = Boolean(settings.hasSecret)
    await loadBackups()
  } catch (cause: any) { backupError.value = cause.message || t('backup.loadError') }
}
async function saveBackupSettings() {
  backupError.value = ''; backupSuccess.value = ''; savingBackup.value = true
  try { await api('/backup/settings', { method: 'PUT', body: backupForm }); await loadBackupSettings(); backupSuccess.value = t('backup.saved') }
  catch (cause: any) { backupError.value = cause.message || t('backup.saveError') }
  finally { savingBackup.value = false }
}
async function createBackup() {
  backupError.value = ''; backupSuccess.value = ''; runningBackup.value = true
  try { await api('/backup/create', { method: 'POST' }); await loadBackups(); backupSuccess.value = t('backup.created') }
  catch (cause: any) { backupError.value = cause.message || t('backup.createError') }
  finally { runningBackup.value = false }
}
async function restoreBackup() {
  if (!selectedBackupKey.value) { backupError.value = 'Escolha um backup para obter.'; return }
  if (!window.confirm(t('backup.restoreConfirm'))) return
  backupError.value = ''; backupSuccess.value = ''; restoringBackup.value = true
  try { await api('/backup/restore', { method: 'POST', body: { key: selectedBackupKey.value } }); await Promise.all([workspace.refreshConnections(), workspace.reloadWorkspaceQueries()]); backupSuccess.value = t('backup.restored') }
  catch (cause: any) { backupError.value = cause.message || t('backup.restoreError') }
  finally { restoringBackup.value = false }
}
async function loadBackups() {
  backups.value = (await api<BackupItem[]>('/backup/items')) ?? []
  if (!backups.value.some((backup) => backup.key === selectedBackupKey.value)) selectedBackupKey.value = backups.value[0]?.key || ''
}
async function deleteBackup(key: string) {
  if (!window.confirm('Excluir este backup do S3?')) return
  backupError.value = ''; backupSuccess.value = ''; deletingBackupKey.value = key
  try { await api('/backup/items', { method: 'DELETE', body: { key } }); await loadBackups() }
  catch (cause: any) { backupError.value = cause.message || 'Não foi possível excluir o backup.' }
  finally { deletingBackupKey.value = '' }
}
function selectSection(section: SettingsSection) { activeSection.value = section; if (section === 'audit') loadAuditLogs(); if (section === 'backup') loadBackupSettings() }
function formatDate(value: string) { return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value)) }
function stageLabel(stage: string) { return t(`audit.stage.${stage}`) }
function toggleAuditRun(runId: string) { if (expandedAuditRuns.has(runId)) expandedAuditRuns.delete(runId); else expandedAuditRuns.add(runId) }
watch(error, (message) => { if (message) { notifyError(message); error.value = '' } })
async function exportConnections() {
  connectionTransferError.value = ''; connectionTransferSuccess.value = ''
  try {
    const exported = await api<ConnectionExport>('/connections/export')
    const file = new Blob([JSON.stringify(exported, null, 2)], { type: 'application/json' })
    const link = document.createElement('a')
    link.href = URL.createObjectURL(file)
    link.download = `dbfock-connections-${new Date().toISOString().slice(0, 10)}.json`
    link.click()
    URL.revokeObjectURL(link.href)
    connectionTransferSuccess.value = t('connections.exported', { count: exported.connections.length })
  } catch (cause: any) { connectionTransferError.value = cause.message || t('connections.transferError') }
}
function chooseConnectionImport() { importInput.value?.click() }
async function importConnections(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  connectionTransferError.value = ''; connectionTransferSuccess.value = ''; importingConnections.value = true
  try {
    const exported = JSON.parse(await file.text()) as ConnectionExport
    if (exported.version !== 1 || !Array.isArray(exported.connections)) throw new Error(t('connections.invalidFile'))
    const result = await api<ConnectionImportResult>('/connections/import', { method: 'POST', body: exported })
    await workspace.refreshConnections()
    connectionTransferSuccess.value = t('connections.imported', { count: result.imported })
  } catch (cause: any) { connectionTransferError.value = cause.message || t('connections.transferError') }
  finally { importingConnections.value = false; input.value = '' }
}
const dbeaverConnectionsWithoutUsername = computed(() => dbeaverConnections.value.filter((connection) => !connection.username))
function chooseDBeaverImport() { dbeaverImportInput.value?.click() }
function clearDBeaverImport() { dbeaverConnections.value = []; dbeaverDefaultUsername.value = ''; dbeaverImportError.value = ''; dbeaverImportSuccess.value = '' }
async function readDBeaverProject(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  clearDBeaverImport(); parsingDBeaverProject.value = true
  try {
    dbeaverConnections.value = await parseDBeaverProject(file)
    dbeaverDefaultUsername.value = dbeaverConnections.value.find((connection) => connection.username)?.username || ''
  } catch (cause) { dbeaverImportError.value = cause instanceof DBeaverNoMySQLConnectionsError ? t('connections.dbeaverNoMySQL') : t('connections.dbeaverInvalidProject') }
  finally { parsingDBeaverProject.value = false; input.value = '' }
}
async function importDBeaverConnections() {
  const defaultUsername = dbeaverDefaultUsername.value.trim()
  if (dbeaverConnectionsWithoutUsername.value.length && !defaultUsername) { dbeaverImportError.value = t('connections.dbeaverUsernameRequired'); return }
  dbeaverImportError.value = ''; dbeaverImportSuccess.value = ''; importingDBeaverConnections.value = true
  try {
    const connections: ConnectionImport[] = dbeaverConnections.value.map((connection) => ({ name: connection.name, driver: 'mysql', host: connection.host, port: connection.port, username: connection.username || defaultUsername, initialDatabase: connection.initialDatabase, color: '#3B82F6', environment: 'development', sslEnabled: connection.sslEnabled, timeoutSeconds: 30 }))
    const result = await api<ConnectionImportResult>('/connections/import', { method: 'POST', body: { version: 1, connections } })
    await workspace.refreshConnections(); dbeaverImportSuccess.value = t('connections.imported', { count: result.imported }); dbeaverConnections.value = []
  } catch (cause: any) { dbeaverImportError.value = cause.message || t('connections.transferError') }
  finally { importingDBeaverConnections.value = false }
}

onMounted(() => { loadSettings(); if (activeSection.value === 'audit') loadAuditLogs(); if (activeSection.value === 'backup') loadBackupSettings() })
onUnmounted(() => { if (loadTimer) clearTimeout(loadTimer) })
watch(() => props.section, (section) => { if (section) selectSection(section) })
watch(appliedTextScaleIndex, (index) => { pendingTextScaleIndex.value = index })
</script>

<template>
  <section class="scrollbar h-full overflow-auto">
    <div class="flex min-h-full w-full flex-col px-5 py-6 lg:px-7">
      <header class="border-b border-line pb-5"><h2 class="text-xl font-semibold">{{ t('settings.title') }}</h2><p class="mt-1 text-sm text-muted">{{ t('settings.description') }}</p></header>
      <div class="mt-6 flex min-h-0 flex-1 flex-col gap-6 md:flex-row">
        <nav class="flex shrink-0 gap-1 border-b border-line pb-4 md:w-48 md:flex-col md:border-b-0 md:border-r md:pb-0 md:pr-5">
          <button type="button" class="settings-nav flex items-center gap-2" :class="activeSection === 'appearance' ? 'settings-nav-active' : ''" @click="selectSection('appearance')"><Icon name="lucide:palette" class="h-4 w-4" aria-hidden="true" />{{ t('settings.appearance') }}</button>
          <button type="button" class="settings-nav flex items-center gap-2" :class="activeSection === 'shortcuts' ? 'settings-nav-active' : ''" @click="selectSection('shortcuts')"><Icon name="lucide:keyboard" class="h-4 w-4" aria-hidden="true" />{{ t('settings.shortcuts') }}</button>
          <button type="button" class="settings-nav flex items-center gap-2" :class="activeSection === 'connections' ? 'settings-nav-active' : ''" @click="selectSection('connections')"><Icon name="lucide:database" class="h-4 w-4" aria-hidden="true" />{{ t('settings.connections') }}</button>
          <button type="button" class="settings-nav flex items-center gap-2" :class="activeSection === 'ai' ? 'settings-nav-active' : ''" @click="selectSection('ai')"><Icon name="lucide:sparkles" class="h-4 w-4" aria-hidden="true" />{{ t('settings.aiAgent') }}</button>
          <button type="button" class="settings-nav flex items-center gap-2" :class="activeSection === 'audit' ? 'settings-nav-active' : ''" @click="selectSection('audit')"><Icon name="lucide:scroll-text" class="h-4 w-4" aria-hidden="true" />{{ t('settings.aiAudit') }}</button>
          <button type="button" class="settings-nav flex items-center gap-2" :class="activeSection === 'backup' ? 'settings-nav-active' : ''" @click="selectSection('backup')"><Icon name="lucide:cloud" class="h-4 w-4" aria-hidden="true" />{{ t('settings.backup') }}</button>
          <button type="button" class="settings-nav" :class="activeSection === 'about' ? 'settings-nav-active' : ''" @click="selectSection('about')">ⓘ {{ t('settings.about') }}</button>
        </nav>

        <div class="min-w-0 flex-1 pb-8" @change="commitTextScale">
          <div v-if="activeSection === 'appearance'" class="max-w-xl"><h3 class="text-base font-semibold">{{ t('settings.appearance') }}</h3><p class="mt-1 text-sm text-muted">{{ t('theme.description') }}</p><label class="mt-6 grid max-w-sm gap-1.5 text-sm font-medium">{{ t('settings.language') }}<AppSelect :model-value="locale" :options="locales.map((option) => ({ value: option.value, label: t(option.label) }))" @change="setLocale($event as Locale)" /><span class="text-xs font-normal text-muted">{{ t('settings.languageDescription') }}</span></label><div class="mt-6 max-w-sm"><div class="flex items-baseline justify-between gap-3"><label for="font-size" class="text-sm font-medium">{{ t('settings.fontSize') }}</label><span class="text-xs text-muted">{{ t(textScaleOptions[textScaleIndex]!.label) }}</span></div><input id="font-size" v-model.number="textScaleIndex" class="font-size-slider mt-3 w-full" type="range" min="0" :max="textScaleOptions.length - 1" step="1" :aria-valuetext="t(textScaleOptions[textScaleIndex]!.label)" /><div class="mt-1 grid grid-cols-5 text-center text-[10px] text-muted"><span v-for="option in textScaleOptions" :key="option.value">{{ t(option.label) }}</span></div></div><div class="mt-6 grid gap-2"><button v-for="option in themeOptions" :key="option.value" type="button" class="flex items-center gap-3 rounded-lg border p-3 text-left" :class="theme === option.value ? 'border-accent bg-accent/10' : 'border-line hover:bg-canvas'" @click="theme = option.value"><span class="flex h-9 w-12 shrink-0 overflow-hidden rounded border border-black/10"><i v-for="color in option.preview" :key="color" class="h-full flex-1" :style="{ backgroundColor: color }" /></span><span class="min-w-0 flex-1"><span class="block text-sm font-medium">{{ t(option.label) }}</span><span class="block text-xs text-muted">{{ t(option.description) }}</span></span><Icon v-if="theme === option.value" name="lucide:check" class="h-4 w-4 text-accent" aria-hidden="true" /></button></div></div>

          <section v-else-if="activeSection === 'shortcuts'" class="max-w-3xl"><h3 class="text-base font-semibold">{{ t('settings.shortcuts') }}</h3><p class="mt-1 text-sm text-muted">{{ t('settings.shortcutsDescription') }}</p><dl class="mt-5 overflow-hidden rounded-lg border border-line bg-panel"><div v-for="shortcut in shortcuts" :key="shortcut.label" class="flex items-center justify-between gap-4 border-b border-line px-4 py-3 last:border-b-0"><dt class="text-sm">{{ t(shortcut.label) }}</dt><dd><kbd class="rounded border border-line bg-canvas px-2 py-1 font-mono text-xs text-ink">{{ shortcut.keys }}</kbd></dd></div></dl></section>

          <div v-else-if="activeSection === 'connections'" class="max-w-xl"><h3 class="text-base font-semibold">{{ t('settings.connections') }}</h3><p class="mt-1 text-sm text-muted">{{ t('connections.description') }}</p><div class="mt-6 rounded-lg border border-line bg-panel p-4"><h4 class="text-sm font-medium">{{ t('connections.export') }}</h4><p class="mt-1 text-sm text-muted">{{ t('connections.exportDescription') }}</p><button type="button" class="mt-4 rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas" @click="exportConnections">{{ t('connections.export') }}</button></div><div class="mt-4 rounded-lg border border-line bg-panel p-4"><h4 class="text-sm font-medium">{{ t('connections.import') }}</h4><p class="mt-1 text-sm text-muted">{{ t('connections.importDescription') }}</p><p class="mt-3 rounded-md bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">{{ t('connections.passwordNotice') }}</p><input ref="importInput" class="sr-only" type="file" accept="application/json,.json" @change="importConnections" /><button type="button" class="mt-4 rounded-md bg-accent px-3 py-2 text-sm text-white disabled:opacity-50" :disabled="importingConnections" @click="chooseConnectionImport">{{ importingConnections ? t('connections.importing') : t('connections.import') }}</button></div><div class="mt-4 rounded-lg border border-line bg-panel p-4"><h4 class="text-sm font-medium">{{ t('connections.dbeaverImport') }}</h4><p class="mt-1 text-sm text-muted">{{ t('connections.dbeaverDescription') }}</p><details class="mt-3 rounded-md bg-canvas px-3 py-2 text-sm text-muted"><summary class="cursor-pointer font-medium text-ink">{{ t('connections.dbeaverHowToExport') }}</summary><ol class="mt-2 list-decimal space-y-1 pl-5"><li>{{ t('connections.dbeaverExportStep1') }}</li><li>{{ t('connections.dbeaverExportStep2') }}</li><li>{{ t('connections.dbeaverExportStep3') }}</li></ol></details><p class="mt-3 rounded-md bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">{{ t('connections.passwordNotice') }}</p><input ref="dbeaverImportInput" class="sr-only" type="file" accept=".dbp,application/zip,application/x-zip-compressed" @change="readDBeaverProject" /><button type="button" class="mt-4 rounded-md bg-accent px-3 py-2 text-sm text-white disabled:opacity-50" :disabled="parsingDBeaverProject || importingDBeaverConnections" @click="chooseDBeaverImport">{{ parsingDBeaverProject ? t('connections.dbeaverReading') : t('connections.dbeaverChooseFile') }}</button><div v-if="dbeaverConnections.length" class="mt-4 rounded-md border border-line bg-canvas p-3"><p class="text-sm font-medium">{{ t('connections.dbeaverFound', { count: dbeaverConnections.length }) }}</p><p class="mt-1 text-xs text-muted">{{ t('connections.dbeaverOnlyMySQL') }}</p><label v-if="dbeaverConnectionsWithoutUsername.length" class="mt-3 grid gap-1.5 text-sm font-medium">{{ t('connections.dbeaverDefaultUsername') }}<input v-model="dbeaverDefaultUsername" class="field" autocomplete="username" /><span class="text-xs font-normal text-muted">{{ t('connections.dbeaverDefaultUsernameDescription', { count: dbeaverConnectionsWithoutUsername.length }) }}</span></label><ul class="mt-3 max-h-32 divide-y divide-line overflow-auto rounded border border-line text-sm"><li v-for="connection in dbeaverConnections" :key="`${connection.name}-${connection.host}-${connection.port}`" class="flex items-center justify-between gap-3 px-3 py-2"><span class="truncate">{{ connection.name }}</span><span class="shrink-0 font-mono text-xs text-muted">{{ connection.host }}:{{ connection.port }}</span></li></ul><div class="mt-3 flex justify-end gap-2"><button type="button" class="rounded-md px-3 py-2 text-sm hover:bg-panel" :disabled="importingDBeaverConnections" @click="clearDBeaverImport">{{ t('connection.cancel') }}</button><button type="button" class="rounded-md bg-accent px-3 py-2 text-sm text-white disabled:opacity-50" :disabled="importingDBeaverConnections" @click="importDBeaverConnections">{{ importingDBeaverConnections ? t('connections.importing') : t('connections.dbeaverImport') }}</button></div></div><p v-if="dbeaverImportError" class="mt-3 text-sm text-rose-500">{{ dbeaverImportError }}</p><p v-else-if="dbeaverImportSuccess" class="mt-3 text-sm text-emerald-600">{{ dbeaverImportSuccess }}</p></div><p v-if="connectionTransferError" class="mt-4 text-sm text-rose-500">{{ connectionTransferError }}</p><p v-else-if="connectionTransferSuccess" class="mt-4 text-sm text-emerald-600">{{ connectionTransferSuccess }}</p></div>

          <form v-else-if="activeSection === 'ai'" class="max-w-xl" @submit.prevent="save"><h3 class="text-base font-semibold">{{ t('settings.aiAgent') }}</h3><p class="mt-1 text-sm text-muted">{{ t('ai.description') }}</p><div class="mt-6 grid gap-3"><label class="grid gap-1.5 text-sm font-medium">{{ t('ai.provider') }}<AppSelect :model-value="form.provider" :options="providerOptions" @change="setProvider($event as Provider)" /></label><label class="grid gap-1.5 text-sm font-medium">{{ t('ai.baseUrl') }}<input v-model="form.baseUrl" class="field" placeholder="https://api.example.com/v1" @input="scheduleModelLoad" /></label><label class="grid gap-1.5 text-sm font-medium">{{ t('ai.apiKey') }} <span v-if="!apiKeyRequired" class="font-normal text-muted">{{ t('ai.optional') }}</span><input v-model="form.apiKey" class="field" type="password" autocomplete="off" :placeholder="hasSavedKey ? t('ai.savedKey') : apiKeyRequired ? selectedProvider.apiKeyHint : t('ai.noApiKey')" @input="scheduleModelLoad" /></label><label class="grid gap-1.5 text-sm font-medium">{{ t('ai.model') }}<input v-model="form.model" class="field" list="ai-models" :placeholder="t('ai.modelPlaceholder')" /><datalist id="ai-models"><option v-for="model in models" :key="model" :value="model" /></datalist><span v-if="loadingModels" class="text-xs font-normal text-muted">{{ t('ai.loadingModels') }}</span><span v-else-if="models.length" class="text-xs font-normal text-emerald-600">{{ t('ai.modelsAvailable', { count: models.length }) }}</span><span v-else-if="modelsError" class="text-xs font-normal text-rose-500">{{ modelsError }}</span><span v-else class="text-xs font-normal text-muted">{{ apiKeyRequired ? t('ai.modelsAfterKey') : t('ai.modelsOllama') }}</span></label></div><div class="mt-6 flex justify-end border-t border-line pt-4"><button class="rounded bg-accent px-3 py-2 text-white disabled:opacity-50" :disabled="saving">{{ saving ? t('common.saving') : t('common.save') }}</button></div></form>

          <form v-else-if="activeSection === 'backup'" class="max-w-xl" @submit.prevent="saveBackupSettings">
            <h3 class="text-base font-semibold">{{ t('settings.backup') }}</h3>
            <p class="mt-1 text-sm text-muted">{{ t('backup.description') }}</p>
            <div class="mt-6 grid gap-3">
              <label class="grid gap-1.5 text-sm font-medium">{{ t('backup.endpoint') }}<input v-model="backupForm.endpoint" class="field" type="url" placeholder="https://s3.us-east-1.amazonaws.com" /></label>
              <label class="grid gap-1.5 text-sm font-medium">{{ t('backup.bucket') }}<input v-model="backupForm.bucket" class="field" autocomplete="off" placeholder="my-dbfock-backups" /></label>
              <label class="grid gap-1.5 text-sm font-medium">{{ t('backup.region') }}<input v-model="backupForm.region" class="field" autocomplete="off" placeholder="us-east-1" /></label>
              <label class="grid gap-1.5 text-sm font-medium">{{ t('backup.accessKey') }}<input v-model="backupForm.accessKey" class="field" type="password" autocomplete="off" :placeholder="hasSavedBackupAccessKey ? t('backup.savedAccessKey') : 'AKIA…'" /></label>
              <label class="grid gap-1.5 text-sm font-medium">{{ t('backup.secret') }}<input v-model="backupForm.secret" class="field" type="password" autocomplete="off" :placeholder="hasSavedBackupSecret ? t('backup.savedSecret') : '…'" /></label>
            </div>
            <p class="mt-4 rounded-md bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-300">{{ t('backup.credentialsNotice') }}</p>
            <p class="mt-3 text-sm text-muted">Cada backup recebe data e hora. São mantidos no máximo 5 backups; ao criar o sexto, o mais antigo é excluído.</p>
            <section class="mt-5 rounded-lg border border-line bg-panel p-4">
              <div class="flex items-center justify-between gap-3"><h4 class="text-sm font-medium">Backups disponíveis</h4><button type="button" class="text-xs text-accent hover:underline" :disabled="runningBackup || restoringBackup || Boolean(deletingBackupKey)" @click="loadBackups">Atualizar</button></div>
              <div v-if="backups.length" class="mt-3 divide-y divide-line overflow-hidden rounded-md border border-line">
                <label v-for="backup in backups" :key="backup.key" class="flex cursor-pointer items-center gap-3 bg-canvas px-3 py-2.5 hover:bg-panel">
                  <input v-model="selectedBackupKey" type="radio" name="backup" :value="backup.key" />
                  <span class="min-w-0 flex-1"><span class="block text-sm font-medium">{{ formatDate(backup.createdAt) }}</span><span class="block truncate font-mono text-xs text-muted">{{ backup.key }} · {{ Math.ceil(backup.size / 1024) }} KB</span></span>
                  <button type="button" class="rounded px-2 py-1 text-xs text-rose-500 hover:bg-rose-500/10 disabled:opacity-50" :disabled="Boolean(deletingBackupKey)" @click.prevent="deleteBackup(backup.key)">{{ deletingBackupKey === backup.key ? 'Excluindo…' : 'Excluir' }}</button>
                </label>
              </div>
              <p v-else class="mt-3 text-sm text-muted">Nenhum backup disponível neste bucket.</p>
            </section>
            <p v-if="backupError" class="mt-3 text-sm text-rose-500">{{ backupError }}</p>
            <p v-else-if="backupSuccess" class="mt-3 text-sm text-emerald-600">{{ backupSuccess }}</p>
            <div class="mt-6 flex flex-wrap justify-end gap-2 border-t border-line pt-4">
              <button class="rounded border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="savingBackup || runningBackup || restoringBackup">{{ savingBackup ? t('common.saving') : t('common.save') }}</button>
              <button type="button" class="rounded bg-accent px-3 py-2 text-sm text-white disabled:opacity-50" :disabled="savingBackup || runningBackup || restoringBackup" @click="createBackup">{{ runningBackup ? t('backup.creating') : t('backup.create') }}</button>
              <button type="button" class="rounded border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="savingBackup || runningBackup || restoringBackup || !selectedBackupKey" @click="restoreBackup">{{ restoringBackup ? t('backup.restoring') : t('backup.restore') }}</button>
            </div>
          </form>

          <section v-else-if="activeSection === 'about'" class="max-w-xl"><h3 class="text-base font-semibold">{{ t('settings.about') }}</h3><p class="mt-1 text-sm text-muted">{{ t('about.description') }}</p><dl class="mt-6 overflow-hidden rounded-lg border border-line bg-panel"><div class="flex items-center justify-between gap-4 border-b border-line px-4 py-3"><dt class="text-sm text-muted">{{ t('about.version') }}</dt><dd class="font-mono text-sm font-medium">{{ appVersion }}</dd></div><div class="flex items-center justify-between gap-4 border-b border-line px-4 py-3"><dt class="text-sm text-muted">{{ t('about.license') }}</dt><dd class="font-mono text-sm font-medium">{{ license }}</dd></div><div class="flex items-center justify-between gap-4 px-4 py-3"><dt class="text-sm text-muted">{{ t('about.repository') }}</dt><dd><a class="text-sm font-medium text-accent hover:underline" :href="githubUrl" target="_blank" rel="noreferrer">alexclaz/dbfock</a></dd></div></dl></section>

          <div v-else>
            <div class="flex items-start justify-between gap-4"><div><h3 class="text-base font-semibold">{{ t('settings.aiAudit') }}</h3><p class="mt-1 text-sm text-muted">{{ t('audit.description') }}</p></div><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="loadingAudit" @click="loadAuditLogs">{{ loadingAudit ? t('audit.loading') : t('audit.refresh') }}</button></div>
            <p v-if="auditError" class="mt-4 text-sm text-rose-500">{{ auditError }}</p>
            <p v-else-if="!loadingAudit && !auditRuns.length" class="mt-8 text-sm text-muted">{{ t('audit.empty') }}</p>
            <div v-else class="mt-5 space-y-4">
              <article v-for="run in auditRuns" :key="run.id" class="overflow-hidden rounded-lg border border-line bg-panel">
                <button type="button" class="flex w-full flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 text-left hover:bg-canvas" :aria-expanded="expandedAuditRuns.has(run.id)" @click="toggleAuditRun(run.id)">
                  <Icon :name="expandedAuditRuns.has(run.id) ? 'lucide:chevron-down' : 'lucide:chevron-right'" class="h-4 w-4 shrink-0 text-muted" aria-hidden="true" />
                  <div class="min-w-0 flex-1"><span class="text-xs font-semibold uppercase tracking-wide text-muted">{{ t('audit.question') }}</span><p class="mt-0.5 break-words text-sm font-medium">{{ run.question || t('audit.legacyQuestion') }}</p></div>
                  <span class="text-xs text-muted">{{ t('audit.steps', { count: run.logs.length }) }} · {{ formatDate(run.createdAt) }}</span>
                </button>
                <div v-if="expandedAuditRuns.has(run.id)" class="divide-y divide-line border-t border-line">
                  <section v-for="log in run.logs" :key="log.id">
                    <header class="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 text-sm"><strong>{{ stageLabel(log.stage) }}</strong><span class="text-muted">{{ log.provider }} · {{ log.model }}</span><time class="ml-auto text-xs text-muted">{{ formatDate(log.createdAt) }}</time></header>
                    <div class="grid divide-y divide-line lg:grid-cols-2 lg:divide-x lg:divide-y-0"><section class="p-4"><h4 class="text-xs font-semibold uppercase tracking-wide text-muted">{{ t('audit.request') }}</h4><pre class="audit-code mt-2">{{ log.request }}</pre></section><section class="p-4"><h4 class="text-xs font-semibold uppercase tracking-wide text-muted">{{ log.error ? t('audit.error') : t('audit.response') }}</h4><pre class="audit-code mt-2" :class="log.error ? 'text-rose-500' : ''">{{ log.error || log.response }}</pre></section></div>
                  </section>
                </div>
              </article>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.field { @apply h-10 rounded-md border border-line bg-canvas px-3 text-sm font-normal; }
.settings-nav { @apply rounded-md px-3 py-2 text-left text-sm text-muted hover:bg-canvas hover:text-ink; }
.settings-nav-active { @apply bg-accent/10 font-medium text-accent hover:bg-accent/10 hover:text-accent; }
.audit-code { @apply max-h-72 overflow-auto whitespace-pre-wrap break-words rounded bg-canvas p-3 text-xs leading-5 text-ink; }
.font-size-slider { accent-color: rgb(var(--accent)); }
</style>
