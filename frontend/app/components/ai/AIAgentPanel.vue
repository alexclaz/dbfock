<script setup lang="ts">
import type { AIAgentChat, AIChatJob, AISchemaTable, DatabaseInfo, TableInfo } from '~/types/database'

const props = defineProps<{ tabId: string; connectionId: string; database?: string; query?: string; width?: number }>()
const emit = defineEmits<{ apply: [sql: string]; status: [tabId: string, status: 'running' | 'complete'] }>()
const api = useApi()
const workspace = useWorkspaceStore()
const { t } = useI18n()

function ensureChat(tabId = props.tabId) {
  const tab = workspace.tabs.find((item) => item.id === tabId)
  if (!tab) return undefined
  tab.aiChat ??= { draft: '', messages: [] }
  tab.aiChat.databaseScope ??= 'all'
  tab.aiChat.selectedDatabases ??= []
  tab.aiChat.tableScope ??= 'all'
  tab.aiChat.selectedTables ??= []
  return tab.aiChat
}
const chat = computed<AIAgentChat | undefined>(() => ensureChat())
const loading = computed(() => {
  const tab = workspace.tabs.find((item) => item.id === props.tabId)
  return tab?.aiStatus === 'running' || Boolean(tab?.aiJobId)
})
const databases = ref<DatabaseInfo[]>([])
const tablesByDatabase = ref<Record<string, TableInfo[]>>({})
const metadataLoading = ref(false)
const metadataError = ref('')
const databaseSearch = ref('')
const tableSearch = ref('')
const databasePickerOpen = ref(false)
const tablePickerOpen = ref(false)
const schemaScopeCollapsed = ref(true)
let pollTimer: ReturnType<typeof setTimeout> | undefined
const promptInput = ref<HTMLTextAreaElement>()
const hasEditorQuery = computed(() => Boolean(props.query?.trim()))
const includeEditorQuery = computed({
  get: () => Boolean(chat.value?.includeEditorQuery),
  set: (value: boolean) => { if (chat.value) chat.value.includeEditorQuery = value },
})
const selectedDatabases = computed(() => chat.value?.selectedDatabases || [])
const selectedTables = computed(() => chat.value?.selectedTables || [])
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
    targetChat?.messages.push({ role: 'assistant', content: job.status === 'complete' ? job.message || '' : t('aiAgent.error', { message: job.error || 'Unknown error' }), executionTimeMs: job.status === 'complete' ? executionTimeMs(job) : undefined })
    emit('status', tabId, 'complete')
  } catch {
    if (targetTab.aiJobId !== jobId) return
    pollTimer = setTimeout(syncJob, 2000)
  }
}
async function send(message = chat.value?.draft || '') {
  const text = message.trim()
  const tabId = props.tabId
  const targetTab = workspace.tabs.find((item) => item.id === tabId)
  if (!text || !targetTab || !chat.value) return
  if (chat.value.databaseScope === 'selected' && !selectedDatabases.value.length) { metadataError.value = t('aiAgent.chooseDatabase'); return }
  if (chat.value.tableScope === 'selected' && !selectedTables.value.length) { metadataError.value = t('aiAgent.chooseTable'); return }
  const prompt = promptWithEditorQuery(text)
  targetTab.aiChat!.messages.push({ role: 'user', content: text })
  targetTab.aiChat!.draft = ''
  emit('status', tabId, 'running')
  try {
    const history = targetTab.aiChat!.messages.slice(0, -1).map(({ role, content }) => ({ role, content }))
    const job = await api<AIChatJob>('/ai/chat/jobs', { method: 'POST', body: { connectionId: props.connectionId, database: props.database, prompt, history, databaseScope: chat.value.databaseScope, selectedDatabases: selectedDatabases.value, tableScope: chat.value.tableScope, selectedTables: selectedTables.value } })
    targetTab.aiJobId = job.id
    await syncJob()
  } catch (cause: any) {
    targetTab.aiChat?.messages.push({ role: 'assistant', content: t('aiAgent.error', { message: cause.message }) })
    emit('status', tabId, 'complete')
  }
}
function sqlFromResponse(text: string) { return text.match(/```sql\s*([\s\S]*?)```/i)?.[1]?.trim() }
function insert(text: string) { const sql = sqlFromResponse(text); if (sql) emit('apply', sql) }
watch(() => props.connectionId, () => { databases.value = []; tablesByDatabase.value = {}; metadataError.value = ''; databasePickerOpen.value = false; tablePickerOpen.value = false })
defineExpose({ ask: send, pasteQuery })
onMounted(syncJob)
onBeforeUnmount(() => clearTimeout(pollTimer))
</script>

<template>
  <aside class="flex min-w-80 shrink-0 flex-col border-l border-line bg-panel" :style="{ width: `${width ?? 50}%` }">
    <div class="flex justify-between border-b border-line p-3"><b class="text-sm">{{ t('aiAgent.title') }}</b><button class="text-xs text-muted" @click="clear">{{ t('aiAgent.clear') }}</button></div>
    <div class="scrollbar flex-1 space-y-3 overflow-auto p-3"><p v-if="!chat?.messages.length" class="text-sm text-muted">{{ t('aiAgent.empty') }}</p><div v-for="(message, index) in chat?.messages || []" :key="index" class="rounded p-2 text-sm" :class="message.role === 'user' ? 'ml-4 bg-accent text-white' : 'bg-canvas'"><div class="whitespace-pre-wrap">{{ message.content }}</div><button v-if="message.role === 'assistant' && sqlFromResponse(message.content)" class="mt-2 block text-xs text-accent" @click="insert(message.content)">{{ t('aiAgent.insertSql') }}</button><p v-if="message.role === 'assistant' && message.executionTimeMs !== undefined" class="mt-2 text-right text-[11px] text-muted">{{ t('aiAgent.executionTime', { time: formatExecutionTime(message.executionTimeMs) }) }}</p></div></div>
    <form class="border-t border-line p-3" @submit.prevent="send()">
      <div class="mb-3 rounded border border-line bg-canvas p-2">
        <button type="button" class="flex w-full items-center justify-between text-left text-xs font-medium text-ink" :aria-expanded="!schemaScopeCollapsed" aria-controls="schema-scope" @click="schemaScopeCollapsed = !schemaScopeCollapsed"><span>{{ t('aiAgent.schemaScope') }}</span><svg class="h-3 w-3 transition-transform" :class="schemaScopeCollapsed ? '' : 'rotate-180'" viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" /></svg></button>
        <div v-show="!schemaScopeCollapsed" id="schema-scope" class="mt-2 space-y-2">
        <div class="space-y-1.5">
          <span class="text-[11px] text-muted">{{ t('aiAgent.databases') }}</span>
          <div class="grid grid-cols-2 gap-1"><button type="button" class="scope-option" :class="chat?.databaseScope === 'all' ? 'scope-option-active' : ''" @click="setDatabaseScope('all')">{{ t('aiAgent.allDatabases') }}</button><button type="button" class="scope-option" :class="chat?.databaseScope === 'selected' ? 'scope-option-active' : ''" @click="setDatabaseScope('selected')">{{ t('aiAgent.selectDatabases') }}</button></div>
          <div v-if="chat?.databaseScope === 'selected'" class="relative"><button type="button" class="scope-picker" :aria-expanded="databasePickerOpen" @click="databasePickerOpen = !databasePickerOpen; loadDatabases()"><span class="truncate">{{ databaseScopeLabel }}</span><span>⌄</span></button><div v-if="databasePickerOpen" class="scope-menu"><input v-model="databaseSearch" type="search" class="scope-search" :placeholder="t('aiAgent.searchDatabases')" autofocus /><p v-if="metadataLoading" class="scope-empty">{{ t('aiAgent.loadingScope') }}</p><label v-for="database in filteredDatabases" :key="database.name" class="scope-check"><input :checked="hasDatabase(database.name)" type="checkbox" @change="toggleDatabase(database.name)" /><span class="truncate">{{ database.name }}</span></label><p v-if="!metadataLoading && !filteredDatabases.length" class="scope-empty">{{ t('aiAgent.noMatches') }}</p></div></div>
        </div>
        <div class="space-y-1.5">
          <span class="text-[11px] text-muted">{{ t('aiAgent.tables') }}</span>
          <div class="grid grid-cols-2 gap-1"><button type="button" class="scope-option" :class="chat?.tableScope === 'all' ? 'scope-option-active' : ''" @click="setTableScope('all')">{{ t('aiAgent.allTables') }}</button><button type="button" class="scope-option" :class="chat?.tableScope === 'selected' ? 'scope-option-active' : ''" @click="setTableScope('selected')">{{ t('aiAgent.selectTables') }}</button></div>
          <div v-if="chat?.tableScope === 'selected'" class="relative"><button type="button" class="scope-picker" :disabled="!selectedDatabases.length" :aria-expanded="tablePickerOpen" @click="tablePickerOpen = !tablePickerOpen"><span class="truncate">{{ selectedDatabases.length ? tableScopeLabel : t('aiAgent.chooseDatabaseFirst') }}</span><span>⌄</span></button><div v-if="tablePickerOpen" class="scope-menu"><input v-model="tableSearch" type="search" class="scope-search" :placeholder="t('aiAgent.searchTables')" autofocus /><p v-if="metadataLoading" class="scope-empty">{{ t('aiAgent.loadingScope') }}</p><label v-for="table in filteredTables" :key="tableKey(table)" class="scope-check"><input :checked="hasTable(table)" type="checkbox" @change="toggleTable(table)" /><span class="truncate">{{ table.database }}.{{ table.table }}</span></label><p v-if="!metadataLoading && !filteredTables.length" class="scope-empty">{{ t('aiAgent.noMatches') }}</p></div></div>
        </div>
        <p v-if="metadataError" class="text-xs text-rose-500">{{ metadataError }}</p>
        </div>
      </div>
      <textarea ref="promptInput" :value="chat?.draft || ''" class="h-20 w-full rounded border border-line bg-canvas p-2 text-sm" :placeholder="t('aiAgent.placeholder')" @input="setDraft(($event.target as HTMLTextAreaElement).value)" />
      <label v-if="hasEditorQuery" class="mt-2 flex cursor-pointer items-center gap-2 text-xs text-muted"><input v-model="includeEditorQuery" type="checkbox" class="accent-accent" />{{ t('aiAgent.includeEditorQuery') }}</label>
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
</style>
