<script setup lang="ts">
import type { DatabaseInfo, QueryResult, SchemaDiagram, TableInfo, TableStructure } from '~/types/database'
import { queryResultAsCSV, queryResultAsJSON, queryResultAsTSV, queryResultEdits } from '~/utils/queryResult'
import { parseTableImport, tableInsertStatements } from '~/utils/tableTransfer'

type TableSection = 'data' | 'structure' | 'constraints' | 'foreignKeys' | 'references' | 'triggers' | 'indexes' | 'ddl' | 'diagram' | 'tools'
type TableToolsSection = 'import' | 'export' | 'migration' | 'maintenance'

const props = defineProps<{ connectionId: string; database: string; table: string; activeSection?: TableSection }>()
const emit = defineEmits<{ 'update:activeSection': [value: TableSection]; transactionStatus: [connectionId: string, pending: boolean, pendingStatements: number]; 'open-database': [database: string]; 'open-table': [table: string] }>()
const api = useApi()
const workspace = useWorkspaceStore()
const { t } = useI18n()
const { error: notifyError, success: notifySuccess } = useToast()
const result = ref<QueryResult>()
const structure = ref<TableStructure>()
const loading = ref(true)
const page = ref(0)
const pageSize = ref(100)
const sortColumn = ref<string>()
const sortDirection = ref<'asc' | 'desc'>('asc')
const tableSections: TableSection[] = ['data', 'structure', 'constraints', 'foreignKeys', 'references', 'triggers', 'indexes', 'ddl', 'tools', 'diagram']
const section = ref<TableSection>(tableSections.includes(props.activeSection as TableSection) ? props.activeSection! : 'data')
const error = ref('')
const dataEditing = ref(false)
const dataView = ref<'table' | 'json' | 'csv'>('table')
const dataCopied = ref(false)
const dataGrid = ref<{ save: () => boolean; cancel: () => void; canSave: boolean }>()
const ddlCopied = ref(false)
const ddlEl = ref<HTMLElement>()
const importInput = ref<HTMLInputElement>()
const transferring = ref(false)
const sourceDatabases = ref<DatabaseInfo[]>([])
const sourceTables = ref<TableInfo[]>([])
const sourceLoading = ref(false)
const migrating = ref(false)
const source = reactive({ connectionId: '', database: '', table: '' })
const migrationOptions = reactive({ truncateBefore: false, ignoreDuplicates: false })
const maintenanceRunning = ref('')
const maintenanceResult = ref<QueryResult>()
const showTruncateConfirmation = ref(false)
const showMigrationTruncateConfirmation = ref(false)
const toolsSection = ref<TableToolsSection>('import')

async function loadData() {
  loading.value = true
  error.value = ''
  try {
    const params = new URLSearchParams({ limit: String(pageSize.value), offset: String(page.value * pageSize.value) })
    if (sortColumn.value) { params.set('sort', sortColumn.value); params.set('direction', sortDirection.value ?? 'asc') }
    result.value = await api<QueryResult>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables/${encodeURIComponent(props.table)}/data?${params.toString()}`)
  }
  catch (cause: any) { error.value = cause.message }
  finally { loading.value = false }
}
async function previousPage() { if (page.value === 0 || loading.value) return; page.value--; await loadData() }
async function nextPage() { if (!result.value?.hasMore || loading.value) return; page.value++; await loadData() }
async function toggleSort(column: string) {
  if (loading.value || dataEditing.value) return
  if (sortColumn.value !== column) { sortColumn.value = column; sortDirection.value = 'asc' }
  else if (sortDirection.value === 'asc') { sortDirection.value = 'desc' }
  else { sortColumn.value = undefined; sortDirection.value = 'asc' }
  page.value = 0
  await loadData()
}
async function loadStructure() {
  try { structure.value = await api<TableStructure>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables/${encodeURIComponent(props.table)}/structure`) }
  catch (cause: any) { error.value = cause.message }
}
const schemaDiagram = ref<SchemaDiagram>()
const diagramLoading = ref(false)
async function loadSchemaDiagram() {
  diagramLoading.value = true
  try { schemaDiagram.value = await api<SchemaDiagram>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/diagram`) }
  catch (cause: any) { notifyError(cause.message) }
  finally { diagramLoading.value = false }
}
const diagramTables = computed(() => {
  const tables = schemaDiagram.value?.tables ?? []
  const current = tables.find((item) => item.name === props.table)
  if (!current) return tables.filter((item) => item.name === props.table)
  const related = new Set([props.table])
  for (const fk of current.foreignKeys) related.add(fk.referencedTable)
  for (const item of tables) if (item.foreignKeys.some((fk) => fk.referencedTable === props.table)) related.add(item.name)
  return tables.filter((item) => related.has(item.name))
})
function selectSection(next: TableSection) {
  section.value = next
  emit('update:activeSection', next)
  if (next === 'diagram' && !schemaDiagram.value) void loadSchemaDiagram()
  else if (next !== 'data' && next !== 'diagram' && next !== 'tools' && !structure.value) void loadStructure()
}
const constraints = computed(() => structure.value?.constraints ?? [])
const foreignKeys = computed(() => structure.value?.foreignKeys ?? [])
const references = computed(() => structure.value?.references ?? [])
const triggers = computed(() => structure.value?.triggers ?? [])
const indexes = computed(() => structure.value?.indexes ?? [])
async function copyData() {
  if (!result.value || !navigator.clipboard) return
  const contents = dataView.value === 'json' ? queryResultAsJSON(result.value) : dataView.value === 'csv' ? queryResultAsCSV(result.value) : queryResultAsTSV(result.value)
  try { await navigator.clipboard.writeText(contents); dataCopied.value = true; window.setTimeout(() => dataCopied.value = false, 1500) }
  catch { /* Clipboard access can be denied by the browser. */ }
}
async function copyDDL() {
  if (!structure.value?.ddl || !navigator.clipboard) return
  try { await navigator.clipboard.writeText(structure.value.ddl); ddlCopied.value = true; window.setTimeout(() => ddlCopied.value = false, 1500) }
  catch { /* Clipboard access can be denied by the browser. */ }
}
function selectAllDdl(event: KeyboardEvent) {
  if (!(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== 'a') return
  event.preventDefault()
  const element = ddlEl.value
  if (!element) return
  const selection = window.getSelection()
  const range = document.createRange()
  range.selectNodeContents(element)
  selection?.removeAllRanges()
  selection?.addRange(range)
}
async function saveDataEdits(edited: QueryResult) {
  if (!result.value) return
  try {
    const updates = queryResultEdits(result.value, edited)
    let transaction: QueryResult | undefined
    for (const update of updates) transaction = await api<QueryResult>(`/connections/${props.connectionId}/rows/update`, { method: 'POST', body: { database: props.database, table: props.table, ...update } })
    result.value = edited
    dataEditing.value = false
    if (transaction) emit('transactionStatus', props.connectionId, transaction.transactionPending, transaction.pendingStatements)
  } catch (cause: any) { notifyError(cause.message) }
}
const sourceConnectionOptions = computed(() => workspace.connections.map((connection) => ({ value: connection.id, label: connection.name, disabled: connection.status !== 'connected' })))
const sourceDatabaseOptions = computed(() => sourceDatabases.value.map((database) => ({ value: database.name, label: database.name })))
const sourceTableOptions = computed(() => sourceTables.value.map((item) => ({ value: item.name, label: item.name })))
function tableNameSQL() { return `\`${props.database.replaceAll('`', '``')}\`.\`${props.table.replaceAll('`', '``')}\`` }
async function runMaintenance(action: 'check' | 'analyze' | 'repair' | 'truncate') {
  const statements = { check: `CHECK TABLE ${tableNameSQL()}`, analyze: `ANALYZE TABLE ${tableNameSQL()}`, repair: `REPAIR TABLE ${tableNameSQL()}`, truncate: `TRUNCATE TABLE ${tableNameSQL()}` }
  maintenanceRunning.value = action
  try {
    const response = await api<QueryResult>(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql: statements[action], historySql: `${action} ${props.database}.${props.table}` } })
    maintenanceResult.value = response
    if (response.transactionPending) emit('transactionStatus', props.connectionId, response.transactionPending, response.pendingStatements)
    if (action === 'truncate') await loadData()
    notifySuccess(t(`table.tools.${action}Success`))
  } catch (cause: any) { notifyError(cause.message) }
  finally { maintenanceRunning.value = '' }
}
async function confirmTruncate() { showTruncateConfirmation.value = false; await runMaintenance('truncate') }
async function migrateTable(confirmedTruncate = false) {
  if (!source.connectionId || !source.database || !source.table) { notifyError(t('table.tools.migrationRequired')); return }
  if (source.connectionId === props.connectionId && source.database === props.database && source.table === props.table) { notifyError(t('table.tools.migrationSameTable')); return }
  if (migrationOptions.truncateBefore && !confirmedTruncate) { showMigrationTruncateConfirmation.value = true; return }
  migrating.value = true
  try {
    const targetStructure = await api<TableStructure>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables/${encodeURIComponent(props.table)}/structure`)
    const allowedColumns = new Set(targetStructure.columns.map((column) => column.name))
    const rows: Record<string, unknown>[] = []
    let columns: string[] = []
    let offset = 0
    let hasMore = true
    while (hasMore) {
      const sourceResult = await api<QueryResult>(`/connections/${source.connectionId}/databases/${encodeURIComponent(source.database)}/tables/${encodeURIComponent(source.table)}/data?limit=100&offset=${offset}`)
      if (!columns.length) columns = sourceResult.columns.map((column) => column.name).filter((column) => allowedColumns.has(column))
      rows.push(...sourceResult.rows)
      offset += sourceResult.rows.length
      hasMore = sourceResult.hasMore
      if (rows.length >= 10_000 && hasMore) throw new Error(t('table.importRowLimit'))
    }
    if (!rows.length) throw new Error(t('table.importEmpty'))
    if (!columns.length) throw new Error(t('table.tools.migrationColumns'))
    let importedRows = 0
    let transaction: QueryResult | undefined
    const columnTypes = Object.fromEntries(targetStructure.columns.map((column) => [column.name, column.databaseType]))
    if (migrationOptions.truncateBefore) await api<QueryResult>(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql: `TRUNCATE TABLE ${tableNameSQL()}`, historySql: `truncate ${props.database}.${props.table} before migration` } })
    for (const sql of tableInsertStatements(props.database, props.table, columns, rows.map((row) => columns.map((column) => row[column])), 80_000, columnTypes, migrationOptions.ignoreDuplicates)) {
      transaction = await api<QueryResult>(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql, historySql: `Migrate ${source.database}.${source.table} into ${props.database}.${props.table}` } })
      importedRows += transaction.affectedRows
    }
    if (transaction) emit('transactionStatus', props.connectionId, transaction.transactionPending, transaction.pendingStatements)
    await loadData()
    notifySuccess(t('table.tools.migrationSuccess', { count: importedRows }))
  } catch (cause: any) { notifyError(cause.message) }
  finally { migrating.value = false }
}
async function confirmMigrationTruncate() { showMigrationTruncateConfirmation.value = false; await migrateTable(true) }
function transferFileName(extension: 'csv' | 'json') {
  return `dbfock-${props.database}-${props.table}-${new Date().toISOString().slice(0, 10)}.${extension}`.replaceAll(/[^a-zA-Z0-9._-]/g, '-')
}
function downloadTable(contents: string, extension: 'csv' | 'json') {
  const file = new Blob([contents], { type: extension === 'csv' ? 'text/csv;charset=utf-8' : 'application/json;charset=utf-8' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(file)
  link.download = transferFileName(extension)
  link.click()
  URL.revokeObjectURL(link.href)
}
async function exportTable(format: 'csv' | 'json') {
  if (transferring.value) return
  transferring.value = true
  try {
    const rows: Record<string, unknown>[] = []
    let columns: QueryResult['columns'] = []
    let offset = 0
    let hasMore = true
    while (hasMore) {
      const params = new URLSearchParams({ limit: '100', offset: String(offset) })
      if (sortColumn.value) { params.set('sort', sortColumn.value); params.set('direction', sortDirection.value) }
      const pageResult = await api<QueryResult>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables/${encodeURIComponent(props.table)}/data?${params}`)
      columns = pageResult.columns
      rows.push(...pageResult.rows)
      offset += pageResult.rows.length
      hasMore = pageResult.hasMore
      if (rows.length >= 50_000 && hasMore) throw new Error(t('table.exportLimit'))
    }
    const exported: QueryResult = { columns, rows, rowCount: rows.length, executionTimeMs: 0, affectedRows: 0, hasMore: false, transactionPending: false, pendingStatements: 0 }
    downloadTable(format === 'csv' ? queryResultAsCSV(exported) : queryResultAsJSON(exported), format)
    notifySuccess(t('table.exported', { count: rows.length }))
  } catch (cause: any) { notifyError(cause.message) }
  finally { transferring.value = false }
}
function chooseImport() { if (!transferring.value) importInput.value?.click() }
async function importTable(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || transferring.value) return
  if (file.size > 10 * 1024 * 1024) { notifyError(t('table.importFileTooLarge')); return }
  transferring.value = true
  try {
    const imported = parseTableImport(await file.text(), file.name)
    if (!imported.rows.length) throw new Error(t('table.importEmpty'))
    if (imported.rows.length > 10_000) throw new Error(t('table.importRowLimit'))
    const tableStructure = await api<TableStructure>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables/${encodeURIComponent(props.table)}/structure`)
    structure.value = tableStructure
    const allowedColumns = new Set(tableStructure.columns.map((column) => column.name))
    if (!imported.columns.length || imported.columns.some((column) => !allowedColumns.has(column))) throw new Error(t('table.importColumns'))
    let importedRows = 0
    let transaction: QueryResult | undefined
    const columnTypes = Object.fromEntries(tableStructure.columns.map((column) => [column.name, column.databaseType]))
    const statements = tableInsertStatements(props.database, props.table, imported.columns, imported.rows, 80_000, columnTypes)
    for (const sql of statements) {
      transaction = await api<QueryResult>(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql, historySql: `Import ${imported.rows.length} row(s) into ${props.database}.${props.table}` } })
      importedRows += transaction.affectedRows
    }
    if (transaction) emit('transactionStatus', props.connectionId, transaction.transactionPending, transaction.pendingStatements)
    await loadData()
    notifySuccess(t('table.imported', { count: importedRows }))
  } catch (cause: any) { notifyError(cause.message) }
  finally { transferring.value = false }
}

watch(() => source.connectionId, async (connectionId) => {
  source.database = ''
  source.table = ''
  sourceDatabases.value = []
  sourceTables.value = []
  if (!connectionId) return
  sourceLoading.value = true
  try { sourceDatabases.value = await api<DatabaseInfo[]>(`/connections/${connectionId}/databases`) }
  catch (cause: any) { notifyError(cause.message) }
  finally { sourceLoading.value = false }
})
watch(() => source.database, async (database) => {
  source.table = ''
  sourceTables.value = []
  if (!source.connectionId || !database) return
  sourceLoading.value = true
  try { sourceTables.value = await api<TableInfo[]>(`/connections/${source.connectionId}/databases/${encodeURIComponent(database)}/tables`) }
  catch (cause: any) { notifyError(cause.message) }
  finally { sourceLoading.value = false }
})
watch(() => [props.connectionId, props.database, props.table], loadData, { immediate: true })
watch(error, (message) => { if (message) { notifyError(message); error.value = '' } })
</script>

<template>
  <section class="flex h-full min-h-0 flex-col">
    <header class="space-y-3 border-b border-line px-5 py-4 lg:px-7">
      <div class="flex items-center gap-2"><Icon name="lucide:table-2" class="h-5 w-5 text-muted" aria-hidden="true" /><h1 class="text-xl font-semibold"><button type="button" class="text-sm font-normal text-muted hover:text-accent hover:underline focus-ring" :aria-label="database" @click="emit('open-database', database)">{{ database }}.</button>{{ table }}</h1></div>
      <div class="scrollbar max-w-full overflow-x-auto"><div class="flex w-max rounded-md border border-line p-0.5 text-xs">
        <button class="rounded px-2.5 py-1" :class="section === 'data' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('data')">{{ t('table.data') }}</button><button class="rounded px-2.5 py-1" :class="section === 'structure' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('structure')">{{ t('table.structure') }}</button><button class="rounded px-2.5 py-1" :class="section === 'constraints' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('constraints')">{{ t('table.constraints') }}</button><button class="rounded px-2.5 py-1" :class="section === 'foreignKeys' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('foreignKeys')">{{ t('table.foreignKeys') }}</button><button class="rounded px-2.5 py-1" :class="section === 'references' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('references')">{{ t('table.references') }}</button><button class="rounded px-2.5 py-1" :class="section === 'triggers' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('triggers')">{{ t('table.triggers') }}</button><button class="rounded px-2.5 py-1" :class="section === 'indexes' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('indexes')">{{ t('table.indexes') }}</button><button class="rounded px-2.5 py-1" :class="section === 'ddl' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('ddl')">DDL</button><button class="rounded px-2.5 py-1" :class="section === 'tools' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('tools')">{{ t('table.tools.title') }}</button><button class="rounded px-2.5 py-1" :class="section === 'diagram' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('diagram')">{{ t('table.diagram') }}</button>
      </div>
      </div>
    </header>
    <template v-if="section === 'data'">
      <div class="flex items-center justify-between gap-3 border-b border-line px-4 py-2 text-xs text-muted">
        <span>{{ t('query.rows', { count: result?.rowCount || 0 }) }}{{ result?.hasMore ? '+' : '' }}</span>
        <div class="flex flex-wrap items-center justify-end gap-2">
          <AppSelect v-model="pageSize" :disabled="loading || dataEditing" class="w-24" :options="[{ value: 50, label: '50' }, { value: 100, label: '100' }, { value: 250, label: '250' }]" @change="page = 0; loadData()" />
          <button :disabled="page === 0 || loading || dataEditing" @click="previousPage">{{ t('table.previous') }}</button>
          <button :disabled="!result?.hasMore || loading || dataEditing" @click="nextPage">{{ t('table.next') }}</button>
          <button class="grid rounded p-1 hover:bg-canvas disabled:opacity-60" :aria-label="t('stats.refresh')" :disabled="loading || dataEditing" @click="loadData"><Icon name="lucide:refresh-cw" class="h-4 w-4" aria-hidden="true" /></button>
          <button type="button" class="grid rounded p-1 hover:bg-canvas disabled:opacity-60" :title="dataCopied ? t('grid.copied') : t('grid.copy')" :aria-label="dataCopied ? t('grid.copied') : t('grid.copy')" :disabled="!result" @click="copyData"><Icon :name="dataCopied ? 'lucide:check' : 'lucide:copy'" class="h-4 w-4" aria-hidden="true" /></button>
          <div class="flex rounded-md border border-line p-0.5">
            <button type="button" class="rounded px-2.5 py-1" :class="dataView === 'table' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="dataView === 'table'" @click="dataView = 'table'">{{ t('grid.table') }}</button>
            <button type="button" class="rounded px-2.5 py-1" :class="dataView === 'json' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="dataView === 'json'" @click="dataView = 'json'">JSON</button>
            <button type="button" class="rounded px-2.5 py-1" :class="dataView === 'csv' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="dataView === 'csv'" @click="dataView = 'csv'">CSV</button>
          </div>
          <template v-if="dataEditing"><button type="button" class="rounded-md bg-accent px-2.5 py-1 font-medium text-white disabled:cursor-not-allowed disabled:opacity-50" :disabled="!dataGrid?.canSave" @click="dataGrid?.save()">{{ t('grid.save') }}</button><button type="button" class="rounded-md border border-line px-2.5 py-1 text-ink" @click="dataGrid?.cancel()">{{ t('grid.cancel') }}</button></template>
        </div>
      </div>
      <div class="min-h-0 flex-1"><DataGrid ref="dataGrid" :result="result" :loading="loading" :view="dataView" :editing="dataEditing" :sort-column="sortColumn" :sort-direction="sortDirection" @start-edit="dataEditing = true" @save="saveDataEdits" @cancel="dataEditing = false" @sort="toggleSort" /></div>
    </template>
    <div v-else-if="section === 'structure'" class="scrollbar overflow-auto p-4"><table class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.column') }}</th><th class="border-b border-line p-2">{{ t('table.type') }}</th><th class="border-b border-line p-2">{{ t('table.nullable') }}</th><th class="border-b border-line p-2">{{ t('table.key') }}</th><th class="border-b border-line p-2">{{ t('table.default') }}</th></tr></thead><tbody><tr v-for="column in structure?.columns" :key="column.name"><td class="border-b border-line p-2 font-medium">{{ column.name }}</td><td class="border-b border-line p-2 text-muted">{{ column.columnType }}</td><td class="border-b border-line p-2">{{ column.nullable ? t('table.yes') : t('table.no') }}</td><td class="border-b border-line p-2">{{ column.key || '—' }}</td><td class="border-b border-line p-2">{{ column.default || '—' }}</td></tr></tbody></table></div>
    <div v-else-if="section === 'constraints'" class="scrollbar overflow-auto p-4"><table v-if="constraints.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.type') }}</th><th class="border-b border-line p-2">{{ t('table.columns') }}</th></tr></thead><tbody><tr v-for="constraint in constraints" :key="constraint.name"><td class="border-b border-line p-2 font-medium">{{ constraint.name }}</td><td class="border-b border-line p-2">{{ constraint.type }}</td><td class="border-b border-line p-2 text-muted">{{ constraint.columns?.join(', ') || '—' }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'foreignKeys'" class="scrollbar overflow-auto p-4"><table v-if="foreignKeys.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.column') }}</th><th class="border-b border-line p-2">{{ t('table.references') }}</th></tr></thead><tbody><tr v-for="foreignKey in foreignKeys" :key="`${foreignKey.name}:${foreignKey.column}`"><td class="border-b border-line p-2 font-medium">{{ foreignKey.name }}</td><td class="border-b border-line p-2">{{ foreignKey.column }}</td><td class="border-b border-line p-2 text-muted">{{ foreignKey.referencedTable }}.{{ foreignKey.referencedColumn }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'references'" class="scrollbar overflow-auto p-4"><table v-if="references.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.table') }}</th><th class="border-b border-line p-2">{{ t('table.column') }}</th><th class="border-b border-line p-2">{{ t('table.references') }}</th></tr></thead><tbody><tr v-for="reference in references" :key="`${reference.database}:${reference.table}:${reference.name}:${reference.column}`"><td class="border-b border-line p-2 font-medium">{{ reference.name }}</td><td class="border-b border-line p-2">{{ reference.database }}.{{ reference.table }}</td><td class="border-b border-line p-2">{{ reference.column }}</td><td class="border-b border-line p-2 text-muted">{{ table }}.{{ reference.referencedColumn }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'triggers'" class="scrollbar space-y-3 overflow-auto p-4"><article v-for="trigger in triggers" :key="trigger.name" class="rounded-md border border-line"><div class="flex items-center gap-2 border-b border-line px-3 py-2 text-sm"><span class="font-medium">{{ trigger.name }}</span><span class="text-xs text-muted">{{ trigger.timing }} {{ trigger.event }}</span></div><pre class="overflow-auto whitespace-pre-wrap p-3 font-mono text-xs">{{ trigger.statement }}</pre></article><p v-if="!triggers.length" class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'indexes'" class="scrollbar overflow-auto p-4"><table v-if="indexes.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.unique') }}</th><th class="border-b border-line p-2">{{ t('table.columns') }}</th></tr></thead><tbody><tr v-for="index in indexes" :key="index.name"><td class="border-b border-line p-2 font-medium">{{ index.name }}</td><td class="border-b border-line p-2">{{ index.unique ? t('table.yes') : t('table.no') }}</td><td class="border-b border-line p-2 text-muted">{{ index.columns?.join(', ') || '—' }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'tools'" class="scrollbar min-h-0 flex-1 overflow-auto px-5 py-6 lg:px-7">
      <header class="border-b border-line pb-5"><h2 class="text-xl font-semibold">{{ t('table.tools.title') }}</h2><p class="mt-1 text-sm text-muted">{{ t('table.tools.description') }}</p></header>
      <div class="mt-6 flex min-h-0 flex-1 flex-col gap-6 md:flex-row">
        <nav class="flex shrink-0 gap-1 border-b border-line pb-4 md:w-48 md:flex-col md:border-b-0 md:border-r md:pb-0 md:pr-5">
          <button type="button" class="tools-nav flex items-center gap-2" :class="toolsSection === 'import' ? 'tools-nav-active' : ''" @click="toolsSection = 'import'"><Icon name="lucide:upload" class="h-4 w-4" aria-hidden="true" />{{ t('table.tools.importTitle') }}</button>
          <button type="button" class="tools-nav flex items-center gap-2" :class="toolsSection === 'export' ? 'tools-nav-active' : ''" @click="toolsSection = 'export'"><Icon name="lucide:download" class="h-4 w-4" aria-hidden="true" />{{ t('table.tools.exportTitle') }}</button>
          <button type="button" class="tools-nav flex items-center gap-2" :class="toolsSection === 'migration' ? 'tools-nav-active' : ''" @click="toolsSection = 'migration'"><Icon name="lucide:arrow-right-left" class="h-4 w-4" aria-hidden="true" />{{ t('table.tools.migrationTitle') }}</button>
          <button type="button" class="tools-nav flex items-center gap-2" :class="toolsSection === 'maintenance' ? 'tools-nav-active' : ''" @click="toolsSection = 'maintenance'"><Icon name="lucide:wrench" class="h-4 w-4" aria-hidden="true" />{{ t('table.tools.maintenanceTitle') }}</button>
        </nav>
        <div class="min-w-0 flex-1 pb-8">
          <section v-if="toolsSection === 'import'" class="max-w-xl"><h3 class="text-base font-semibold">{{ t('table.tools.importTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('table.tools.importDescription') }}</p><input ref="importInput" class="sr-only" type="file" accept=".csv,text/csv,.json,application/json" @change="importTable" /><button type="button" class="mt-6 rounded-md bg-accent px-3 py-2 text-sm text-white disabled:opacity-50" :disabled="transferring" @click="chooseImport">{{ transferring ? t('table.tools.importing') : t('table.import') }}</button></section>
          <section v-else-if="toolsSection === 'export'" class="max-w-xl"><h3 class="text-base font-semibold">{{ t('table.tools.exportTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('table.tools.exportDescription') }}</p><div class="mt-6 flex flex-wrap gap-2"><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="transferring" @click="exportTable('csv')">{{ t('table.exportCsv') }}</button><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="transferring" @click="exportTable('json')">{{ t('table.exportJson') }}</button></div></section>
          <section v-else-if="toolsSection === 'migration'" class="max-w-3xl"><h3 class="text-base font-semibold">{{ t('table.tools.migrationTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('table.tools.migrationDescription', { table: `${database}.${table}` }) }}</p><div class="mt-6 grid gap-3 md:grid-cols-3"><label class="grid gap-1.5 text-sm font-medium">{{ t('table.tools.sourceConnection') }}<AppSelect v-model="source.connectionId" :options="sourceConnectionOptions" :disabled="migrating" :placeholder="t('table.tools.chooseConnection')" /></label><label class="grid gap-1.5 text-sm font-medium">{{ t('table.tools.sourceDatabase') }}<AppSelect v-model="source.database" :options="sourceDatabaseOptions" :disabled="migrating || sourceLoading || !source.connectionId" :placeholder="t('table.tools.chooseDatabase')" /></label><label class="grid gap-1.5 text-sm font-medium">{{ t('table.tools.sourceTable') }}<AppSelect v-model="source.table" :options="sourceTableOptions" :disabled="migrating || sourceLoading || !source.database" :placeholder="t('table.tools.chooseTable')" /></label></div><div class="mt-5 grid gap-3"><label class="flex items-start gap-3 text-sm"><input v-model="migrationOptions.truncateBefore" class="mt-0.5" type="checkbox" :disabled="migrating" /><span><span class="font-medium">{{ t('table.tools.truncateBefore') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('table.tools.truncateBeforeDescription') }}</span></span></label><label class="flex items-start gap-3 text-sm"><input v-model="migrationOptions.ignoreDuplicates" class="mt-0.5" type="checkbox" :disabled="migrating" /><span><span class="font-medium">{{ t('table.tools.ignoreDuplicates') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('table.tools.ignoreDuplicatesDescription') }}</span></span></label></div><button type="button" class="mt-6 rounded-md bg-accent px-3 py-2 text-sm text-white disabled:opacity-50" :disabled="migrating || sourceLoading || !source.table" @click="() => migrateTable()">{{ migrating ? t('table.tools.migrating') : t('table.tools.migrate') }}</button></section>
          <section v-else class="max-w-3xl"><h3 class="text-base font-semibold">{{ t('table.tools.maintenanceTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('table.tools.maintenanceDescription') }}</p><div class="mt-6 flex flex-wrap gap-2"><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="Boolean(maintenanceRunning)" @click="runMaintenance('check')">{{ maintenanceRunning === 'check' ? t('table.tools.running') : t('table.tools.check') }}</button><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="Boolean(maintenanceRunning)" @click="runMaintenance('analyze')">{{ maintenanceRunning === 'analyze' ? t('table.tools.running') : t('table.tools.analyze') }}</button><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="Boolean(maintenanceRunning)" @click="runMaintenance('repair')">{{ maintenanceRunning === 'repair' ? t('table.tools.running') : t('table.tools.repair') }}</button><button type="button" class="rounded-md border border-rose-500/40 px-3 py-2 text-sm text-rose-600 hover:bg-rose-500/10 disabled:opacity-50" :disabled="Boolean(maintenanceRunning)" @click="showTruncateConfirmation = true">{{ t('table.tools.truncate') }}</button></div><div v-if="maintenanceResult?.columns.length" class="scrollbar mt-5 overflow-auto rounded-md border border-line"><table class="min-w-full text-left text-xs"><thead class="bg-canvas text-muted"><tr><th v-for="column in maintenanceResult.columns" :key="column.name" class="px-3 py-2 font-medium">{{ column.name }}</th></tr></thead><tbody><tr v-for="(row, index) in maintenanceResult.rows" :key="index" class="border-t border-line"><td v-for="column in maintenanceResult.columns" :key="column.name" class="px-3 py-2">{{ row[column.name] }}</td></tr></tbody></table></div></section>
        </div>
      </div>
    </div>
    <div v-else-if="section === 'diagram'" class="min-h-0 flex-1"><ErDiagram :tables="diagramTables" :focus-table="table" :loading="diagramLoading" @open-table="emit('open-table', $event)" /></div>
    <div v-else class="flex min-h-0 flex-1 flex-col"><div class="flex items-center justify-end border-b border-line px-4 py-2"><button type="button" class="grid rounded p-1 hover:bg-canvas disabled:opacity-60" :title="ddlCopied ? t('grid.copied') : t('grid.copy')" :aria-label="ddlCopied ? t('grid.copied') : t('grid.copy')" :disabled="!structure?.ddl" @click="copyDDL"><Icon :name="ddlCopied ? 'lucide:check' : 'lucide:copy'" class="h-4 w-4" aria-hidden="true" /></button></div><pre ref="ddlEl" tabindex="0" class="scrollbar flex-1 overflow-auto whitespace-pre-wrap p-4 font-mono text-xs outline-none" @keydown="selectAllDdl">{{ structure?.ddl || t('table.loadingDdl') }}</pre></div>
    <AppConfirmDialog v-model="showTruncateConfirmation" :title="t('table.tools.truncateTitle')" :description="t('table.tools.truncateDescription', { table: `${database}.${table}` })" :confirm-label="t('table.tools.truncate')" :cancel-label="t('common.close')" tone="danger" @confirm="confirmTruncate" />
    <AppConfirmDialog v-model="showMigrationTruncateConfirmation" :title="t('table.tools.truncateBeforeTitle')" :description="t('table.tools.truncateBeforeConfirm', { table: `${database}.${table}` })" :confirm-label="t('table.tools.truncateAndMigrate')" :cancel-label="t('common.close')" tone="danger" @confirm="confirmMigrationTruncate" />
  </section>
</template>

<style scoped>
.tools-nav { @apply rounded-md px-3 py-2 text-left text-sm text-muted hover:bg-canvas hover:text-ink; }
.tools-nav-active { @apply bg-accent/10 font-medium text-accent hover:bg-accent/10 hover:text-accent; }
</style>
