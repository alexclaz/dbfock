<script setup lang="ts">
import type { AIAgentChat, AIChatJob, AISchemaTable, AIScopeConfirmation, DatabaseInfo, TableInfo } from '~/types/database'

const props = defineProps<{ tabId: string; connectionId: string; database?: string; query?: string; width?: number }>()
const emit = defineEmits<{ apply: [sql: string]; status: [tabId: string, status: 'running' | 'complete'] }>()
const api = useApi()
const workspace = useWorkspaceStore()
const { t } = useI18n()

function ensureChat(tabId = props.tabId) {
  const tab = workspace.tabs.find((item) => item.id === tabId)
  if (!tab) return undefined
  tab.aiChat ??= { draft: '', messages: [] }
  tab.aiChat.fastSchemaRetrieval ??= false
  tab.aiChat.databaseScope ??= 'all'
  tab.aiChat.selectedDatabases ??= []
  tab.aiChat.tableScope ??= 'all'
  tab.aiChat.selectedTables ??= []
  return tab.aiChat
}
const chat = computed<AIAgentChat | undefined>(() => ensureChat())
const submitting = ref(false)
const loading = computed(() => {
  const tab = workspace.tabs.find((item) => item.id === props.tabId)
  return submitting.value || tab?.aiStatus === 'running' || Boolean(tab?.aiJobId)
})
const databases = ref<DatabaseInfo[]>([])
const tablesByDatabase = ref<Record<string, TableInfo[]>>({})
const metadataLoading = ref(false)
const metadataError = ref('')
const databaseSearch = ref('')
const tableSearch = ref('')
const databasePickerOpen = ref(false)
const tablePickerOpen = ref(false)
const confirmationPicker = ref<'databases' | 'tables'>()
const schemaScopeCollapsed = ref(true)
let pollTimer: ReturnType<typeof setTimeout> | undefined
const promptInput = ref<HTMLTextAreaElement>()
const hasEditorQuery = computed(() => Boolean(props.query?.trim()))
const includeEditorQuery = computed({
  get: () => Boolean(chat.value?.includeEditorQuery),
  set: (value: boolean) => { if (chat.value) chat.value.includeEditorQuery = value },
})
const fastSchemaRetrieval = computed({
  get: () => Boolean(chat.value?.fastSchemaRetrieval),
  set: (value: boolean) => { if (chat.value) chat.value.fastSchemaRetrieval = value },
})
const selectedDatabases = computed(() => chat.value?.selectedDatabases || [])
const selectedTables = computed(() => chat.value?.selectedTables || [])
const scopeConfirmation = computed<AIScopeConfirmation | undefined>({
  get: () => chat.value?.scopeConfirmation,
  set: (value) => { if (chat.value) chat.value.scopeConfirmation = value },
})
const filteredDatabases = computed(() => {
  const term = databaseSearch.value.trim().toLocaleLowerCase()
  return databases.value.filter((item) => !term || item.name.toLocaleLowerCase().includes(term))
})
const availableTables = computed<AISchemaTable[]>(() => selectedDatabases.value.flatMap((database) => (tablesByDatabase.value[database] || []).map((table) => ({ database, table: table.name }))))
const filteredTables = computed(() => {
  const term = tableSearch.value.trim().toLocaleLowerCase()
  return availableTables.value.filter((item) => !term || `${item.database}.${item.table}`.toLocaleLowerCase().includes(term))
})
const databaseScopeLabel = computed(() => t('aiAgent.databasesSelected', { count: selectedDatabases.value.length }))
const tableScopeLabel = computed(() => t('aiAgent.tablesSelected', { count: selectedTables.value.length }))
const scopeConfirmationPrefix = '__DBFOCK_SCOPE_CONFIRMATION__:'

function setDraft(value: string) { if (chat.value) chat.value.draft = value }
function promptWithEditorQuery(prompt: string) {
  const query = props.query?.trim()
  if (!includeEditorQuery.value || !query) return prompt
  return `${prompt}\n\n${t('aiAgent.editorQueryContext')}\n\n\`\`\`sql\n${query}\n\`\`\``
}
function pasteQuery(sql: string) {
  const query = sql.trim()
  if (!chat.value || !query) return
  const code = `\`\`\`sql\n${query}\n\`\`\``
  chat.value.draft = chat.value.draft.trim() ? `${chat.value.draft.trimEnd()}\n\n${code}` : code
  nextTick(() => promptInput.value?.focus())
}
function clear() {
  const tab = workspace.tabs.find((item) => item.id === props.tabId)
  if (!chat.value || !tab) return
  chat.value.draft = ''
  chat.value.messages = []
  chat.value.scopeConfirmation = undefined
  confirmationPicker.value = undefined
  tab.aiJobId = undefined
  emit('status', props.tabId, 'complete')
}
async function loadDatabases() {
  if (!props.connectionId || databases.value.length || metadataLoading.value) return
  metadataLoading.value = true
  metadataError.value = ''
  try { databases.value = (await api<DatabaseInfo[]>(`/connections/${props.connectionId}/databases`)) ?? [] }
  catch (cause: any) { metadataError.value = cause.message || t('aiAgent.scopeLoadError') }
  finally { metadataLoading.value = false }
}
async function loadTables(database: string) {
  if (tablesByDatabase.value[database]) return
  metadataLoading.value = true
  metadataError.value = ''
  try { tablesByDatabase.value[database] = (await api<TableInfo[]>(`/connections/${props.connectionId}/databases/${encodeURIComponent(database)}/tables`)) ?? [] }
  catch (cause: any) { metadataError.value = cause.message || t('aiAgent.scopeLoadError') }
  finally { metadataLoading.value = false }
}
function setDatabaseScope(scope: 'all' | 'selected') {
  if (!chat.value) return
  chat.value.databaseScope = scope
  if (scope === 'all') {
    chat.value.tableScope = 'all'
    chat.value.selectedDatabases = []
    chat.value.selectedTables = []
    chat.value.scopeConfirmation = undefined
    databasePickerOpen.value = false
    tablePickerOpen.value = false
  } else loadDatabases()
}
function setTableScope(scope: 'all' | 'selected') {
  if (!chat.value) return
  chat.value.tableScope = scope
  if (scope === 'all') {
    chat.value.selectedTables = []
    tablePickerOpen.value = false
    return
  }
  if (chat.value.databaseScope !== 'selected') chat.value.databaseScope = 'selected'
  loadDatabases()
}
function hasDatabase(name: string) { return selectedDatabases.value.includes(name) }
function toggleDatabase(name: string) {
  if (!chat.value) return
  const selected = hasDatabase(name)
  chat.value.selectedDatabases = selected ? selectedDatabases.value.filter((item) => item !== name) : [...selectedDatabases.value, name]
  if (selected) chat.value.selectedTables = selectedTables.value.filter((item) => item.database !== name)
  else loadTables(name)
}
function tableKey(table: AISchemaTable) { return `${table.database}\u0000${table.table}` }
function hasTable(table: AISchemaTable) { return selectedTables.value.some((item) => tableKey(item) === tableKey(table)) }
function toggleTable(table: AISchemaTable) {
  if (!chat.value) return
  chat.value.selectedTables = hasTable(table) ? selectedTables.value.filter((item) => tableKey(item) !== tableKey(table)) : [...selectedTables.value, table]
}
async function loadSelectedTables() {
  for (const database of selectedDatabases.value) await loadTables(database)
}
async function hydrateScopeMetadata() {
  if (chat.value?.databaseScope !== 'selected') return
  await loadDatabases()
  if (chat.value.tableScope === 'selected' || chat.value.scopeConfirmation?.step === 'tables') await loadSelectedTables()
}
function parseScopeConfirmation(message: string): AIScopeConfirmation | undefined {
  if (!message.startsWith(scopeConfirmationPrefix)) return undefined
  try {
    const value = JSON.parse(message.slice(scopeConfirmationPrefix.length))
    if ((value.step !== 'databases' && value.step !== 'tables') || typeof value.prompt !== 'string' || !Array.isArray(value.databases) || !Array.isArray(value.tables)) return undefined
    if (!value.databases.every((database: unknown) => typeof database === 'string') || !value.tables.every((table: unknown) => table && typeof table === 'object' && typeof (table as AISchemaTable).database === 'string' && typeof (table as AISchemaTable).table === 'string')) return undefined
    return value as AIScopeConfirmation
  } catch { return undefined }
}
async function showTablePicker() {
  confirmationPicker.value = 'tables'
  await loadSelectedTables()
}
function showDatabasePicker() {
  confirmationPicker.value = 'databases'
  loadDatabases()
}
function toggleDatabasePicker() {
  databasePickerOpen.value = !databasePickerOpen.value
  if (!databasePickerOpen.value) return
  tablePickerOpen.value = false
  loadDatabases()
}
async function toggleTablePicker() {
  tablePickerOpen.value = !tablePickerOpen.value
  if (!tablePickerOpen.value) return
  databasePickerOpen.value = false
  await loadSelectedTables()
}
function executionTimeMs(job: AIChatJob) {
  const startedAt = Date.parse(job.createdAt)
  const completedAt = Date.parse(job.updatedAt)
  if (!Number.isFinite(startedAt) || !Number.isFinite(completedAt)) return undefined
  return Math.max(0, completedAt - startedAt)
}
function formatExecutionTime(milliseconds: number) {
  if (milliseconds < 1_000) return `${Math.round(milliseconds)} ms`
  return `${(milliseconds / 1_000).toFixed(milliseconds < 10_000 ? 1 : 0)} s`
}
async function syncJob() {
  const tabId = props.tabId
  const targetTab = workspace.tabs.find((item) => item.id === tabId)
  const jobId = targetTab?.aiJobId
  if (!targetTab || !jobId) return
  try {
    const job = await api<AIChatJob>(`/ai/chat/jobs/${jobId}`)
    if (targetTab.aiJobId !== job.id) return
    if (job.status === 'running') { emit('status', tabId, 'running'); pollTimer = setTimeout(syncJob, 1000); return }
    targetTab.aiJobId = undefined
    const targetChat = ensureChat(tabId)
    const confirmation = job.status === 'complete' ? parseScopeConfirmation(job.message || '') : undefined
    if (confirmation && targetChat) {
      confirmationPicker.value = undefined
      targetChat.scopeConfirmation = confirmation
      targetChat.databaseScope = 'selected'
      targetChat.selectedDatabases = confirmation.databases
      if (confirmation.step === 'tables') {
        targetChat.tableScope = 'selected'
        targetChat.selectedTables = confirmation.tables
      }
    } else {
      targetChat?.messages.push({ role: 'assistant', content: job.status === 'complete' ? job.message || '' : t('aiAgent.error', { message: job.error || 'Unknown error' }), executionTimeMs: job.status === 'complete' ? executionTimeMs(job) : undefined })
    }
    emit('status', tabId, 'complete')
  } catch {
    if (targetTab.aiJobId !== jobId) return
    pollTimer = setTimeout(syncJob, 2000)
  }
}
type ScopeInput = { fastSchemaRetrieval: boolean; databaseScope: 'all' | 'selected'; selectedDatabases: string[]; tableScope: 'all' | 'selected'; selectedTables: AISchemaTable[]; scopeConfirmation?: 'databases' | 'tables' }
async function submit(message: string, scope: ScopeInput, addUserMessage: boolean) {
  const text = message.trim()
  const tabId = props.tabId
  const targetTab = workspace.tabs.find((item) => item.id === tabId)
  if (!text || !targetTab || !chat.value || loading.value) return
  if (scope.databaseScope === 'selected' && !scope.selectedDatabases.length) { metadataError.value = t('aiAgent.chooseDatabase'); return }
  if (scope.tableScope === 'selected' && !scope.selectedTables.length) { metadataError.value = t('aiAgent.chooseTable'); return }
  const prompt = addUserMessage ? promptWithEditorQuery(text) : text
  if (addUserMessage) {
    targetTab.aiChat!.messages.push({ role: 'user', content: text })
    targetTab.aiChat!.draft = ''
  }
  targetTab.aiChat!.scopeConfirmation = undefined
  emit('status', tabId, 'running')
  submitting.value = true
  try {
    const history = (addUserMessage ? targetTab.aiChat!.messages.slice(0, -1) : targetTab.aiChat!.messages).map(({ role, content }) => ({ role, content }))
    const job = await api<AIChatJob>('/ai/chat/jobs', { method: 'POST', body: { connectionId: props.connectionId, database: props.database, prompt, history, ...scope } })
    targetTab.aiJobId = job.id
    await syncJob()
  } catch (cause: any) {
    targetTab.aiChat?.messages.push({ role: 'assistant', content: t('aiAgent.error', { message: cause.message }) })
    emit('status', tabId, 'complete')
  } finally {
    submitting.value = false
  }
}
async function send(message = chat.value?.draft || '') {
  await submit(message, { fastSchemaRetrieval: fastSchemaRetrieval.value, databaseScope: chat.value?.databaseScope || 'all', selectedDatabases: selectedDatabases.value, tableScope: chat.value?.tableScope || 'all', selectedTables: selectedTables.value }, true)
}
async function confirmDatabases() {
  const confirmation = scopeConfirmation.value
  if (!confirmation) return
  await submit(confirmation.prompt, { fastSchemaRetrieval: false, databaseScope: 'selected', selectedDatabases: selectedDatabases.value, tableScope: 'all', selectedTables: [], scopeConfirmation: 'databases' }, false)
}
async function confirmTables() {
  const confirmation = scopeConfirmation.value
  if (!confirmation) return
  await submit(confirmation.prompt, { fastSchemaRetrieval: false, databaseScope: 'selected', selectedDatabases: selectedDatabases.value, tableScope: 'selected', selectedTables: selectedTables.value, scopeConfirmation: 'tables' }, false)
}
function sqlFromResponse(text: string) { return text.match(/```sql\s*([\s\S]*?)```/i)?.[1]?.trim() }
function insert(text: string) { const sql = sqlFromResponse(text); if (sql) emit('apply', sql) }
watch(() => props.connectionId, () => { databases.value = []; tablesByDatabase.value = {}; metadataError.value = ''; databasePickerOpen.value = false; tablePickerOpen.value = false; confirmationPicker.value = undefined; if (chat.value) chat.value.scopeConfirmation = undefined; hydrateScopeMetadata() })
defineExpose({ ask: send, pasteQuery })
onMounted(async () => { await syncJob(); await hydrateScopeMetadata() })
onBeforeUnmount(() => clearTimeout(pollTimer))
</script>

<template>
  <aside class="flex min-w-80 shrink-0 flex-col border-l border-line bg-panel" :style="{ width: `${width ?? 50}%` }">
    <div class="flex justify-between border-b border-line p-3"><b class="text-sm">{{ t('aiAgent.title') }}</b><button class="text-xs text-muted" @click="clear">{{ t('aiAgent.clear') }}</button></div>
    <div class="scrollbar flex-1 space-y-3 overflow-auto p-3" :aria-busy="loading">
      <p v-if="!chat?.messages.length && !loading && !scopeConfirmation" class="text-sm text-muted">{{ t('aiAgent.empty') }}</p>
      <div v-for="(message, index) in chat?.messages || []" :key="index" class="rounded p-2 text-sm" :class="message.role === 'user' ? 'ml-4 bg-accent text-white' : 'bg-canvas'"><div class="whitespace-pre-wrap">{{ message.content }}</div><button v-if="message.role === 'assistant' && sqlFromResponse(message.content)" class="mt-2 block text-xs text-accent" @click="insert(message.content)">{{ t('aiAgent.insertSql') }}</button><p v-if="message.role === 'assistant' && message.executionTimeMs !== undefined" class="mt-2 text-right text-[11px] text-muted">{{ t('aiAgent.executionTime', { time: formatExecutionTime(message.executionTimeMs) }) }}</p></div>
      <div v-if="scopeConfirmation" class="scope-confirmation"><p class="text-sm font-medium text-ink">{{ scopeConfirmation.step === 'databases' ? t('aiAgent.confirmDatabases') : t('aiAgent.confirmTables') }}</p><div class="mt-2 flex flex-wrap gap-1"><span v-for="database in scopeConfirmation.step === 'databases' ? selectedDatabases : []" :key="database" class="scope-chip">{{ database }}</span><span v-for="table in scopeConfirmation.step === 'tables' ? selectedTables : []" :key="tableKey(table)" class="scope-chip">{{ table.database }}.{{ table.table }}</span></div><p class="mt-2 text-xs text-muted">{{ scopeConfirmation.step === 'databases' ? t('aiAgent.confirmDatabasesHint') : t('aiAgent.confirmTablesHint') }}</p><div class="mt-3 grid grid-cols-2 gap-2"><button type="button" class="scope-confirm-primary" @click="scopeConfirmation.step === 'databases' ? confirmDatabases() : confirmTables()">{{ scopeConfirmation.step === 'databases' ? t('aiAgent.useDatabases') : t('aiAgent.useTables') }}</button><button type="button" class="scope-confirm-secondary" @click="scopeConfirmation.step === 'databases' ? showDatabasePicker() : showTablePicker()">{{ scopeConfirmation.step === 'databases' ? t('aiAgent.chooseOtherDatabases') : t('aiAgent.chooseOtherTables') }}</button></div><div v-if="confirmationPicker === 'databases'" class="scope-confirm-picker"><input v-model="databaseSearch" type="search" class="scope-search" :placeholder="t('aiAgent.searchDatabases')" autofocus ><p v-if="metadataLoading" class="scope-empty">{{ t('aiAgent.loadingScope') }}</p><label v-for="database in filteredDatabases" :key="database.name" class="scope-check"><input :checked="hasDatabase(database.name)" type="checkbox" @change="toggleDatabase(database.name)" ><span class="truncate">{{ database.name }}</span></label><p v-if="!metadataLoading && !filteredDatabases.length" class="scope-empty">{{ t('aiAgent.noMatches') }}</p></div><div v-else-if="confirmationPicker === 'tables'" class="scope-confirm-picker"><input v-model="tableSearch" type="search" class="scope-search" :placeholder="t('aiAgent.searchTables')" autofocus ><p v-if="metadataLoading" class="scope-empty">{{ t('aiAgent.loadingScope') }}</p><label v-for="table in filteredTables" :key="tableKey(table)" class="scope-check"><input :checked="hasTable(table)" type="checkbox" @change="toggleTable(table)" ><span class="truncate">{{ table.database }}.{{ table.table }}</span></label><p v-if="!metadataLoading && !filteredTables.length" class="scope-empty">{{ t('aiAgent.noMatches') }}</p></div></div>
      <div v-if="loading" class="flex items-center gap-2 rounded bg-canvas p-2 text-sm text-muted" role="status" aria-live="polite"><span class="h-2 w-2 animate-pulse rounded-full bg-accent" aria-hidden="true" /><span>{{ t('aiAgent.thinking') }}</span></div>
    </div>
    <form class="border-t border-line p-3" @submit.prevent="send()">
      <textarea ref="promptInput" :value="chat?.draft || ''" class="h-20 w-full rounded border border-line bg-canvas p-2 text-sm" :placeholder="t('aiAgent.placeholder')" @input="setDraft(($event.target as HTMLTextAreaElement).value)" @keydown.enter.exact.prevent="send()" />
      <p class="mt-1 text-[11px] text-muted">{{ t('aiAgent.enterHint') }}</p>
      <label v-if="hasEditorQuery" class="mt-2 flex cursor-pointer items-center gap-2 text-xs text-muted"><input v-model="includeEditorQuery" type="checkbox" class="accent-accent" >{{ t('aiAgent.includeEditorQuery') }}</label>
      <label class="mt-2 flex cursor-pointer items-center gap-2 text-xs text-muted" :title="t('aiAgent.fastSchemaRetrievalHint')"><input v-model="fastSchemaRetrieval" type="checkbox" class="accent-accent" >{{ t('aiAgent.fastSchemaRetrieval') }}</label>
      <div class="mb-3 mt-3 rounded border border-line bg-canvas p-2">
        <button type="button" class="flex w-full items-center justify-between text-left text-xs font-medium text-ink" :aria-expanded="!schemaScopeCollapsed" aria-controls="schema-scope" @click="schemaScopeCollapsed = !schemaScopeCollapsed"><span>{{ t('aiAgent.schemaScope') }}</span><Icon name="lucide:chevron-down" class="h-3.5 w-3.5 transition-transform" :class="schemaScopeCollapsed ? '' : 'rotate-180'" aria-hidden="true" /></button>
        <div v-show="!schemaScopeCollapsed" id="schema-scope" class="mt-2 space-y-2">
        <div class="space-y-1.5">
          <span class="text-[11px] text-muted">{{ t('aiAgent.databases') }}</span>
          <div class="grid grid-cols-2 gap-1"><button type="button" class="scope-option" :class="chat?.databaseScope === 'all' ? 'scope-option-active' : ''" @click="setDatabaseScope('all')">{{ t('aiAgent.allDatabases') }}</button><button type="button" class="scope-option" :class="chat?.databaseScope === 'selected' ? 'scope-option-active' : ''" @click="setDatabaseScope('selected')">{{ t('aiAgent.selectDatabases') }}</button></div>
          <div v-if="chat?.databaseScope === 'selected'" class="relative"><button type="button" class="scope-picker" :aria-expanded="databasePickerOpen" @click="toggleDatabasePicker"><span class="truncate">{{ databaseScopeLabel }}</span><Icon name="lucide:chevron-down" class="h-3.5 w-3.5" aria-hidden="true" /></button><div v-if="databasePickerOpen" class="scope-menu"><input v-model="databaseSearch" type="search" class="scope-search" :placeholder="t('aiAgent.searchDatabases')" autofocus ><p v-if="metadataLoading" class="scope-empty">{{ t('aiAgent.loadingScope') }}</p><label v-for="database in filteredDatabases" :key="database.name" class="scope-check"><input :checked="hasDatabase(database.name)" type="checkbox" @change="toggleDatabase(database.name)" ><span class="truncate">{{ database.name }}</span></label><p v-if="!metadataLoading && !filteredDatabases.length" class="scope-empty">{{ t('aiAgent.noMatches') }}</p></div></div>
        </div>
        <div class="space-y-1.5">
          <span class="text-[11px] text-muted">{{ t('aiAgent.tables') }}</span>
          <div class="grid grid-cols-2 gap-1"><button type="button" class="scope-option" :class="chat?.tableScope === 'all' ? 'scope-option-active' : ''" @click="setTableScope('all')">{{ t('aiAgent.allTables') }}</button><button type="button" class="scope-option" :class="chat?.tableScope === 'selected' ? 'scope-option-active' : ''" @click="setTableScope('selected')">{{ t('aiAgent.selectTables') }}</button></div>
          <div v-if="chat?.tableScope === 'selected'" class="relative"><button type="button" class="scope-picker" :disabled="!selectedDatabases.length" :aria-expanded="tablePickerOpen" @click="toggleTablePicker"><span class="truncate">{{ selectedDatabases.length ? tableScopeLabel : t('aiAgent.chooseDatabaseFirst') }}</span><Icon name="lucide:chevron-down" class="h-3.5 w-3.5" aria-hidden="true" /></button><div v-if="tablePickerOpen" class="scope-menu"><input v-model="tableSearch" type="search" class="scope-search" :placeholder="t('aiAgent.searchTables')" autofocus ><p v-if="metadataLoading" class="scope-empty">{{ t('aiAgent.loadingScope') }}</p><label v-for="table in filteredTables" :key="tableKey(table)" class="scope-check"><input :checked="hasTable(table)" type="checkbox" @change="toggleTable(table)" ><span class="truncate">{{ table.database }}.{{ table.table }}</span></label><p v-if="!metadataLoading && !filteredTables.length" class="scope-empty">{{ t('aiAgent.noMatches') }}</p></div></div>
        </div>
        <p v-if="metadataError" class="text-xs text-rose-500">{{ metadataError }}</p>
        </div>
      </div>
      <button class="mt-2 w-full rounded bg-accent py-2 text-sm text-white disabled:opacity-50" :disabled="loading">{{ loading ? t('aiAgent.thinking') : t('aiAgent.ask') }}</button>
    </form>
  </aside>
</template>

<style scoped>
.scope-option { @apply rounded border border-line px-2 py-1.5 text-left text-[11px] text-muted hover:bg-panel; }
.scope-option-active { @apply border-accent bg-accent/10 font-medium text-accent; }
.scope-picker { @apply flex w-full items-center justify-between gap-2 rounded border border-line bg-panel px-2 py-1.5 text-left text-xs text-ink disabled:cursor-not-allowed disabled:opacity-50; }
.scope-menu { @apply absolute z-40 mt-1 max-h-52 w-full overflow-auto rounded border border-line bg-panel p-1 shadow-lg; }
.scope-search { @apply mb-1 w-full rounded border border-line bg-canvas px-2 py-1.5 text-xs outline-none focus:border-accent; }
.scope-check { @apply flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-xs text-ink hover:bg-canvas; }
.scope-empty { @apply px-2 py-1.5 text-xs text-muted; }
.scope-confirmation { @apply rounded border border-accent/40 bg-accent/5 p-3; }
.scope-confirm-picker { @apply mt-3 max-h-52 overflow-auto rounded border border-line bg-panel p-1; }
.scope-chip { @apply max-w-full truncate rounded bg-panel px-2 py-1 text-xs text-ink; }
.scope-confirm-primary { @apply rounded bg-accent px-2 py-1.5 text-xs font-medium text-white hover:opacity-90; }
.scope-confirm-secondary { @apply rounded border border-line bg-panel px-2 py-1.5 text-xs text-ink hover:bg-canvas; }
</style>
