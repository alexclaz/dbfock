<script setup lang="ts">
import type { Connection, QueryResult, TableStructure, TransactionStatus } from '~/types/database'
import { queryResultAsTSV, queryResultEdits } from '~/utils/queryResult'

type TableSection = 'data' | 'structure' | 'constraints' | 'foreignKeys' | 'references' | 'triggers' | 'indexes' | 'query' | 'ddl'

const props = defineProps<{ tabId: string; connectionId: string; connection: Connection; database: string; table: string; modelValue?: string; activeSection?: TableSection; aiConfigured?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; 'update:activeSection': [value: TableSection]; status: [tabId: string, status: 'running' | 'complete'] }>()
const api = useApi()
const { t } = useI18n()
const { error: notifyError } = useToast()
const result = ref<QueryResult>()
const queryResult = ref<QueryResult>()
const structure = ref<TableStructure>()
const loading = ref(true)
const running = ref(false)
const page = ref(0)
const pageSize = ref(100)
const section = ref<TableSection>(props.activeSection || 'data')
const sql = ref(props.modelValue || `SELECT *\nFROM \`${props.database}\`.\`${props.table}\`\nLIMIT 100;`)
const error = ref('')
const sqlEditor = ref<{ insertSQL: (sql: string) => void }>()
const aiAgent = ref<{ ask: (prompt: string) => Promise<void>; pasteQuery: (sql: string) => void }>()
const aiVisible = ref(true)
const editorHeight = ref(46)
const editorWidth = ref(50)
const transactionStatus = ref<TransactionStatus>({ pending: false, pendingStatements: 0 })
const showCommit = ref(false)
const committing = ref(false)
const dataEditing = ref(false)
const dataCopied = ref(false)
const dataGrid = ref<{ save: () => boolean; cancel: () => void; canSave: boolean }>()
const showAIAgent = computed(() => props.aiConfigured && aiVisible.value)

async function loadData() {
  loading.value = true; error.value = ''
  try { result.value = await api<QueryResult>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables/${encodeURIComponent(props.table)}/data?limit=${pageSize.value}&offset=${page.value * pageSize.value}`) }
  catch (cause: any) { error.value = cause.message }
  finally { loading.value = false }
}
async function previousPage() { if (page.value === 0 || loading.value) return; page.value--; await loadData() }
async function nextPage() { if (!result.value?.hasMore || loading.value) return; page.value++; await loadData() }
async function loadStructure() {
  try { structure.value = await api<TableStructure>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables/${encodeURIComponent(props.table)}/structure`) }
  catch (cause: any) { error.value = cause.message }
}
async function execute(sqlToRun: string) {
  if (!sqlToRun.trim()) return
  running.value = true; error.value = ''
  try { queryResult.value = await api<QueryResult>(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql: sqlToRun, requestId: `table:${props.connectionId}:${props.database}:${props.table}` } }); transactionStatus.value = { pending: queryResult.value.transactionPending, pendingStatements: queryResult.value.pendingStatements } }
  catch (cause: any) { error.value = cause.message }
  finally { running.value = false }
}
function selectSection(next: TableSection) { section.value = next; emit('update:activeSection', next); if (next !== 'data' && next !== 'query' && !structure.value) loadStructure() }
const constraints = computed(() => structure.value?.constraints ?? [])
const foreignKeys = computed(() => structure.value?.foreignKeys ?? [])
const references = computed(() => structure.value?.references ?? [])
const triggers = computed(() => structure.value?.triggers ?? [])
const indexes = computed(() => structure.value?.indexes ?? [])
function explainSQL(query: string) { aiAgent.value?.ask(t('query.explainPrompt', { sql: query })) }
function improveSQL(query: string) { aiAgent.value?.ask(t('query.improvePrompt', { sql: query })) }
function resizeVertical(event: PointerEvent) { const host = (event.currentTarget as HTMLElement).parentElement; if (!host) return; const bounds = host.getBoundingClientRect(); const move = (next: PointerEvent) => editorHeight.value = Math.min(75, Math.max(25, (next.clientY - bounds.top) / bounds.height * 100)); const stop = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', stop) }; window.addEventListener('pointermove', move); window.addEventListener('pointerup', stop) }
function resizeHorizontal(event: PointerEvent) { const host = (event.currentTarget as HTMLElement).parentElement; if (!host) return; const bounds = host.getBoundingClientRect(); const move = (next: PointerEvent) => editorWidth.value = Math.min(75, Math.max(25, (next.clientX - bounds.left) / bounds.width * 100)); const stop = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', stop) }; window.addEventListener('pointermove', move); window.addEventListener('pointerup', stop) }
function hideAIAgent() { aiVisible.value = false }
function showHiddenAIAgent() { aiVisible.value = true }
async function loadTransactionStatus() { if (props.connection.environment === 'production') transactionStatus.value = await api<TransactionStatus>(`/connections/${props.connectionId}/transaction`) }
async function commitTransaction() { committing.value = true; error.value = ''; try { transactionStatus.value = await api<TransactionStatus>(`/connections/${props.connectionId}/transaction/commit`, { method: 'POST' }); showCommit.value = false } catch (cause: any) { error.value = cause.message } finally { committing.value = false } }
async function rollbackTransaction() { error.value = ''; try { transactionStatus.value = await api<TransactionStatus>(`/connections/${props.connectionId}/transaction/rollback`, { method: 'POST' }) } catch (cause: any) { error.value = cause.message } }
async function copyData() {
  if (!result.value || !navigator.clipboard) return
  try { await navigator.clipboard.writeText(queryResultAsTSV(result.value)); dataCopied.value = true; window.setTimeout(() => dataCopied.value = false, 1500) }
  catch { /* Clipboard access can be denied by the browser. */ }
}
async function saveDataEdits(edited: QueryResult) {
  if (!result.value) return
  try {
    const updates = queryResultEdits(result.value, edited)
    let transaction: QueryResult | undefined
    for (const update of updates) transaction = await api<QueryResult>(`/connections/${props.connectionId}/rows/update`, { method: 'POST', body: { database: props.database, table: props.table, ...update } })
    result.value = edited
    dataEditing.value = false
    if (transaction) transactionStatus.value = { pending: transaction.transactionPending, pendingStatements: transaction.pendingStatements }
  } catch (cause: any) { notifyError(cause.message) }
}

watch(() => [props.connectionId, props.database, props.table], loadData, { immediate: true })
watch(() => props.connectionId, loadTransactionStatus, { immediate: true })
watch(sql, (value) => emit('update:modelValue', value))
watch(error, (message) => { if (message) { notifyError(message); error.value = '' } })
onMounted(() => { const height = Number(localStorage.getItem('dbfock.table-sql-editor-height')); const width = Number(localStorage.getItem('dbfock.table-sql-editor-width')); if (height >= 25 && height <= 75) editorHeight.value = height; if (width >= 25 && width <= 75) editorWidth.value = width })
watch([editorHeight, editorWidth], ([height, width]) => { if (import.meta.client) { localStorage.setItem('dbfock.table-sql-editor-height', String(height)); localStorage.setItem('dbfock.table-sql-editor-width', String(width)) } })
</script>

<template>
  <section class="flex h-full min-h-0 flex-col">
    <div class="flex items-center gap-4 border-b border-line px-4 py-2"><div class="shrink-0"><h2 class="text-sm font-semibold"><span class="text-xs font-normal text-muted">{{ database }}</span><span class="text-muted">.</span>{{ table }}</h2></div><div class="scrollbar min-w-0 overflow-x-auto"><div class="flex w-max rounded-md border border-line p-0.5 text-xs"><button class="rounded px-2.5 py-1" :class="section === 'query' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('query')">{{ t('table.query') }}</button><button class="rounded px-2.5 py-1" :class="section === 'data' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('data')">{{ t('table.data') }}</button><button class="rounded px-2.5 py-1" :class="section === 'structure' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('structure')">{{ t('table.structure') }}</button><button class="rounded px-2.5 py-1" :class="section === 'constraints' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('constraints')">{{ t('table.constraints') }}</button><button class="rounded px-2.5 py-1" :class="section === 'foreignKeys' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('foreignKeys')">{{ t('table.foreignKeys') }}</button><button class="rounded px-2.5 py-1" :class="section === 'references' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('references')">{{ t('table.references') }}</button><button class="rounded px-2.5 py-1" :class="section === 'triggers' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('triggers')">{{ t('table.triggers') }}</button><button class="rounded px-2.5 py-1" :class="section === 'indexes' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('indexes')">{{ t('table.indexes') }}</button><button class="rounded px-2.5 py-1" :class="section === 'ddl' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('ddl')">DDL</button></div></div></div>
    <template v-if="section === 'data'"><div class="flex items-center justify-between border-b border-line px-4 py-2 text-xs text-muted"><span>{{ t('query.rows', { count: result?.rowCount || 0 }) }}{{ result?.hasMore ? '+' : '' }}</span><div class="flex items-center gap-2"><AppSelect v-model="pageSize" :disabled="loading || dataEditing" class="w-24" :options="[{ value: 50, label: '50' }, { value: 100, label: '100' }, { value: 250, label: '250' }]" @change="page = 0; loadData()" /><button :disabled="page === 0 || loading || dataEditing" @click="previousPage">{{ t('table.previous') }}</button><button :disabled="!result?.hasMore || loading || dataEditing" @click="nextPage">{{ t('table.next') }}</button><button :disabled="loading || dataEditing" @click="loadData">↻</button><button v-if="!dataEditing" type="button" class="rounded-md border border-line px-2.5 py-1 text-ink disabled:cursor-not-allowed disabled:opacity-50" :disabled="!result" @click="dataEditing = true">{{ t('grid.edit') }}</button><button type="button" class="rounded-md border border-line px-2.5 py-1 text-ink disabled:cursor-not-allowed disabled:opacity-50" :disabled="!result" @click="copyData">{{ dataCopied ? t('grid.copied') : t('grid.copy') }}</button><template v-if="dataEditing"><button type="button" class="rounded-md bg-accent px-2.5 py-1 font-medium text-white disabled:cursor-not-allowed disabled:opacity-50" :disabled="!dataGrid?.canSave" @click="dataGrid?.save()">{{ t('grid.save') }}</button><button type="button" class="rounded-md border border-line px-2.5 py-1 text-ink" @click="dataGrid?.cancel()">{{ t('grid.cancel') }}</button></template></div></div><div class="min-h-0 flex-1"><DataGrid ref="dataGrid" :result="result" :loading="loading" :editing="dataEditing" @start-edit="dataEditing = true" @save="saveDataEdits" @cancel="dataEditing = false" /></div></template>
    <div v-else-if="section === 'structure'" class="scrollbar overflow-auto p-4"><table class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.column') }}</th><th class="border-b border-line p-2">{{ t('table.type') }}</th><th class="border-b border-line p-2">{{ t('table.nullable') }}</th><th class="border-b border-line p-2">{{ t('table.key') }}</th><th class="border-b border-line p-2">{{ t('table.default') }}</th></tr></thead><tbody><tr v-for="column in structure?.columns" :key="column.name"><td class="border-b border-line p-2 font-medium">{{ column.name }}</td><td class="border-b border-line p-2 text-muted">{{ column.columnType }}</td><td class="border-b border-line p-2">{{ column.nullable ? t('table.yes') : t('table.no') }}</td><td class="border-b border-line p-2">{{ column.key || '—' }}</td><td class="border-b border-line p-2">{{ column.default || '—' }}</td></tr></tbody></table></div>
    <div v-else-if="section === 'constraints'" class="scrollbar overflow-auto p-4"><table v-if="constraints.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.type') }}</th><th class="border-b border-line p-2">{{ t('table.columns') }}</th></tr></thead><tbody><tr v-for="constraint in constraints" :key="constraint.name"><td class="border-b border-line p-2 font-medium">{{ constraint.name }}</td><td class="border-b border-line p-2">{{ constraint.type }}</td><td class="border-b border-line p-2 text-muted">{{ constraint.columns?.join(', ') || '—' }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'foreignKeys'" class="scrollbar overflow-auto p-4"><table v-if="foreignKeys.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.column') }}</th><th class="border-b border-line p-2">{{ t('table.references') }}</th></tr></thead><tbody><tr v-for="foreignKey in foreignKeys" :key="`${foreignKey.name}:${foreignKey.column}`"><td class="border-b border-line p-2 font-medium">{{ foreignKey.name }}</td><td class="border-b border-line p-2">{{ foreignKey.column }}</td><td class="border-b border-line p-2 text-muted">{{ foreignKey.referencedTable }}.{{ foreignKey.referencedColumn }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'references'" class="scrollbar overflow-auto p-4"><table v-if="references.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.table') }}</th><th class="border-b border-line p-2">{{ t('table.column') }}</th><th class="border-b border-line p-2">{{ t('table.references') }}</th></tr></thead><tbody><tr v-for="reference in references" :key="`${reference.database}:${reference.table}:${reference.name}:${reference.column}`"><td class="border-b border-line p-2 font-medium">{{ reference.name }}</td><td class="border-b border-line p-2">{{ reference.database }}.{{ reference.table }}</td><td class="border-b border-line p-2">{{ reference.column }}</td><td class="border-b border-line p-2 text-muted">{{ table }}.{{ reference.referencedColumn }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'triggers'" class="scrollbar space-y-3 overflow-auto p-4"><article v-for="trigger in triggers" :key="trigger.name" class="rounded-md border border-line"><div class="flex items-center gap-2 border-b border-line px-3 py-2 text-sm"><span class="font-medium">{{ trigger.name }}</span><span class="text-xs text-muted">{{ trigger.timing }} {{ trigger.event }}</span></div><pre class="overflow-auto whitespace-pre-wrap p-3 font-mono text-xs">{{ trigger.statement }}</pre></article><p v-if="!triggers.length" class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'indexes'" class="scrollbar overflow-auto p-4"><table v-if="indexes.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.unique') }}</th><th class="border-b border-line p-2">{{ t('table.columns') }}</th></tr></thead><tbody><tr v-for="index in indexes" :key="index.name"><td class="border-b border-line p-2 font-medium">{{ index.name }}</td><td class="border-b border-line p-2">{{ index.unique ? t('table.yes') : t('table.no') }}</td><td class="border-b border-line p-2 text-muted">{{ index.columns?.join(', ') || '—' }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <pre v-else-if="section === 'ddl'" class="scrollbar flex-1 overflow-auto whitespace-pre-wrap p-4 font-mono text-xs">{{ structure?.ddl || t('table.loadingDdl') }}</pre>
    <div v-else class="flex min-h-0 flex-1 flex-col"><div class="relative flex min-h-64 shrink-0 border-b border-line" :style="{ height: `${editorHeight}%` }"><SqlEditor ref="sqlEditor" split :width="showAIAgent ? editorWidth : 'calc(100% - 1.5rem)'" :query-actions="false" v-model="sql" :connection-id="connectionId" :connection-name="connection.name" :initial-database="database" :production="connection.environment === 'production'" :transaction-pending="transactionStatus.pending" :pending-statements="transactionStatus.pendingStatements" :committing="committing" :running="running" @execute="execute" @explain="explainSQL" @improve="improveSQL" @send-to-chat="aiAgent?.pasteQuery($event)" @commit="showCommit = true" @rollback="rollbackTransaction" /><div v-if="showAIAgent" class="w-1.5 shrink-0 cursor-col-resize bg-line hover:bg-accent" @pointerdown="resizeHorizontal" @dblclick="hideAIAgent" /><AIAgentPanel v-if="showAIAgent" ref="aiAgent" :tab-id="tabId" :width="100-editorWidth" :connection-id="connectionId" :database="database" :query="sql" @apply="sqlEditor?.insertSQL($event)" @status="(tabId, status) => $emit('status', tabId, status)" /><button v-else-if="aiConfigured" type="button" class="absolute inset-y-0 right-0 z-10 w-6 border-l border-line bg-panel text-xs font-medium text-muted hover:bg-canvas hover:text-ink" :title="t('aiAgent.title')" :aria-label="t('aiAgent.title')" style="writing-mode: vertical-rl" @click="showHiddenAIAgent">{{ t('aiAgent.title') }}</button></div><div class="h-1.5 shrink-0 cursor-row-resize bg-line hover:bg-accent" @pointerdown="resizeVertical" /><div class="flex items-center justify-between border-b border-line px-4 py-2 text-xs text-muted"><span v-if="queryResult">{{ t('query.rowsSummary', { rows: queryResult.rowCount, time: queryResult.executionTimeMs, affected: queryResult.affectedRows }) }}</span><span v-else>{{ t('query.queryResults') }}</span></div><div class="min-h-0 flex-1"><DataGrid :result="queryResult" :loading="running" /></div></div>
  </section>
  <TransactionCommitModal :model-value="showCommit" :connection-name="connection.name" :pending-statements="transactionStatus.pendingStatements" :committing="committing" @update:model-value="showCommit = $event" @confirm="commitTransaction" />
</template>
