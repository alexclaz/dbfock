<script setup lang="ts">
import type { DatabaseInfo, DatabaseMigrationJob, QueryResult, SchemaDiagram, TableInfo } from '~/types/database'

type DatabaseSection = 'tables' | 'diagram' | 'tools'
type DatabaseToolsSection = 'export' | 'import' | 'migration' | 'maintenance' | 'danger'
type NewTableColumn = { id: number; name: string; type: string; nullable: boolean; primary: boolean; autoIncrement: boolean }

const props = defineProps<{ connectionId: string; database: string; activeSection?: DatabaseSection }>()
const emit = defineEmits<{ table: [table: string]; 'update:activeSection': [value: DatabaseSection]; transactionStatus: [connectionId: string, pending: boolean, pendingStatements: number]; databaseDeleted: [connectionId: string, database: string] }>()
const api = useApi()
const { saveTextFile } = useFileSave()
const { fetchDump, dumpFileName } = useDatabaseDump()
const workspace = useWorkspaceStore()
const { t } = useI18n()
const { error: notifyError, success: notifySuccess } = useToast()
const tables = ref<TableInfo[]>()
const loading = ref(true)
const filter = ref('')
const filterInput = ref<HTMLInputElement>()
const isActive = ref(false)
const section = ref<DatabaseSection>(props.activeSection === 'diagram' || props.activeSection === 'tools' ? props.activeSection : 'tables')
const diagram = ref<SchemaDiagram>()
const diagramLoading = ref(false)
const exporting = ref(false)
const exportStructureOnly = ref(false)
const dumpInput = ref<HTMLInputElement>()
const selectedDumpFile = ref<File>()
const importingDump = ref(false)
const showImportConfirmation = ref(false)
const maintenanceRunning = ref('')
const maintenanceResult = ref<QueryResult>()
const toolsSection = ref<DatabaseToolsSection>('export')
const sourceDatabases = ref<DatabaseInfo[]>([])
const sourceLoading = ref(false)
const migrating = ref(false)
const migrationJob = ref<DatabaseMigrationJob>()
const source = reactive({ connectionId: '', database: '' })
const migrationOptions = reactive({ recreateTarget: false, ignoreDuplicates: false, structureOnly: false })
const showRecreateConfirmation = ref(false)
const showDeleteConfirmation = ref(false)
const deletingDatabase = ref(false)
const showCreateTable = ref(false)
const creatingTable = ref(false)
const newTable = reactive<{ name: string; columns: NewTableColumn[] }>({ name: '', columns: [] })
let nextNewColumnId = 1
let migrationPollTimer: ReturnType<typeof setTimeout> | undefined

function messageFor(cause: unknown) { return cause instanceof Error ? cause.message : String(cause) }
function migrationStorageKey() { return `dbfock.databaseMigration.${props.connectionId}.${props.database}` }

const filteredTables = computed(() => {
  const list = tables.value ?? []
  const query = filter.value.trim().toLowerCase()
  return query ? list.filter((table) => table.name.toLowerCase().includes(query)) : list
})
const sourceConnectionOptions = computed(() => workspace.connections.map((connection) => ({ value: connection.id, label: connection.name, disabled: connection.status !== 'connected' })))
const sourceDatabaseOptions = computed(() => sourceDatabases.value.map((item) => ({ value: item.name, label: item.name })))

async function load() {
  loading.value = true
  try { tables.value = await api<TableInfo[]>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables`) }
  catch (cause: unknown) { notifyError(messageFor(cause)) }
  finally { loading.value = false }
}
async function loadDiagram() {
  diagramLoading.value = true
  try { diagram.value = await api<SchemaDiagram>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/diagram`) }
  catch (cause: unknown) { notifyError(messageFor(cause)) }
  finally { diagramLoading.value = false }
}
function tableNameSQL(table: string) { return `\`${props.database.replaceAll('`', '``')}\`.\`${table.replaceAll('`', '``')}\`` }
function databaseNameSQL(name = props.database) { return `\`${name.replaceAll('`', '``')}\`` }
function identifierSQL(name: string) { return `\`${name.replaceAll('`', '``')}\`` }
async function exportDatabase() {
  if (exporting.value) return
  exporting.value = true
  try {
    const script = await fetchDump(props.connectionId, props.database, exportStructureOnly.value)
    if (!await saveTextFile(dumpFileName(props.database), script, 'application/sql;charset=utf-8')) return
    notifySuccess(t('database.tools.exportSuccess', { count: (script.match(/^CREATE TABLE /gm) ?? []).length }))
  } catch (cause: unknown) { notifyError(messageFor(cause)) }
  finally { exporting.value = false }
}
function requestDumpImport(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file || importingDump.value) return
  selectedDumpFile.value = file
  showImportConfirmation.value = true
  input.value = ''
}
async function importDatabaseDump() {
  const file = selectedDumpFile.value
  showImportConfirmation.value = false
  if (!file || importingDump.value) return
  importingDump.value = true
  try {
    await api(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/dump/import`, { method: 'POST', body: { sql: await file.text() } })
    notifySuccess(t('database.tools.importSuccess', { database: props.database }))
    await load()
    if (section.value === 'diagram') await loadDiagram()
  } catch (cause: unknown) { notifyError(messageFor(cause)) }
  finally { importingDump.value = false; selectedDumpFile.value = undefined }
}
async function runMaintenance(action: 'check' | 'analyze' | 'repair') {
  const list = tables.value ?? []
  if (!list.length) { notifyError(t('database.tools.noTables')); return }
  maintenanceRunning.value = action
  try {
    maintenanceResult.value = await api<QueryResult>(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql: `${action.toUpperCase()} TABLE ${list.map((table) => tableNameSQL(table.name)).join(', ')}`, historySql: `${action} all tables in ${props.database}` } })
    notifySuccess(t(`database.tools.${action}Success`))
  } catch (cause: unknown) { notifyError(messageFor(cause)) }
  finally { maintenanceRunning.value = '' }
}
async function migrateDatabase() {
  if (!source.connectionId || !source.database) { notifyError(t('database.tools.migrationRequired')); return }
  if (source.connectionId === props.connectionId && source.database === props.database) { notifyError(t('database.tools.migrationSameDatabase')); return }
  migrating.value = true
  try {
    migrationJob.value = await api<DatabaseMigrationJob>(`/connections/${props.connectionId}/migrate/jobs`, { method: 'POST', body: {
      sourceConnectionId: source.connectionId,
      databases: [source.database],
      targetDatabase: props.database,
      maxTableSizeBytes: 1_099_511_627_776,
      strategy: 'merge',
      recreateTarget: migrationOptions.recreateTarget,
      structureOnly: migrationOptions.structureOnly,
      ignoreDuplicates: migrationOptions.ignoreDuplicates,
    } })
    if (import.meta.client) localStorage.setItem(migrationStorageKey(), migrationJob.value.id)
    scheduleMigrationPoll()
  } catch (cause: unknown) { migrating.value = false; notifyError(messageFor(cause)) }
}
function scheduleMigrationPoll() {
  if (migrationPollTimer) clearTimeout(migrationPollTimer)
  migrationPollTimer = setTimeout(() => void pollMigration(), 750)
}
async function pollMigration() {
  const jobId = migrationJob.value?.id || (import.meta.client ? localStorage.getItem(migrationStorageKey()) : '')
  if (!jobId) return
  try {
    const job = await api<DatabaseMigrationJob>(`/connections/${props.connectionId}/migrate/jobs/${jobId}`)
    migrationJob.value = job
    if (job.status === 'running') { scheduleMigrationPoll(); return }
    if (import.meta.client) localStorage.removeItem(migrationStorageKey())
    migrating.value = false
    await load()
    diagram.value = undefined
    if (job.status === 'failed') { notifyError(job.error || 'Migration failed'); return }
    const result = job.result!
    if (result.failedTables.length) notifyError(t('connectionTools.migrationPartial', { migrated: result.tablesMigrated, failed: result.failedTables.length }))
    else if (migrationOptions.structureOnly) notifySuccess(t('database.tools.migrationStructureSuccess', { count: result.tablesMigrated }))
    else notifySuccess(t('database.tools.migrationSuccess', { count: result.rowsMigrated }))
  } catch { scheduleMigrationPoll() }
}
const migrationProgressPercent = computed(() => {
  const progress = migrationJob.value?.progress
  if (!progress?.totalTables) return 0
  const currentFraction = progress.currentTableEstimatedRows > 0 ? Math.min(1, progress.currentTableRows / progress.currentTableEstimatedRows) : 0
  return Math.min(migrationJob.value?.status === 'running' ? 99 : 100, Math.round((progress.completedTables + currentFraction) / progress.totalTables * 100))
})
function requestMigration() {
  if (migrationOptions.recreateTarget) { showRecreateConfirmation.value = true; return }
  void migrateDatabase()
}
function confirmRecreateMigration() { showRecreateConfirmation.value = false; void migrateDatabase() }
async function deleteDatabase() {
  showDeleteConfirmation.value = false
  deletingDatabase.value = true
  try {
    const response = await api<QueryResult>(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql: `DROP DATABASE ${databaseNameSQL()}`, historySql: `Drop database ${props.database}` } })
    if (response.transactionPending) {
      emit('transactionStatus', props.connectionId, response.transactionPending, response.pendingStatements)
      notifySuccess(t('transaction.queued'))
      return
    }
    notifySuccess(t('database.tools.deleteSuccess', { database: props.database }))
    emit('databaseDeleted', props.connectionId, props.database)
  } catch (cause: unknown) { notifyError(messageFor(cause)) }
  finally { deletingDatabase.value = false }
}
function addNewTableColumn(values: Partial<Omit<NewTableColumn, 'id'>> = {}) {
  newTable.columns.push({ id: nextNewColumnId++, name: '', type: 'varchar(255)', nullable: true, primary: false, autoIncrement: false, ...values })
}
function openCreateTable() {
  if (creatingTable.value) return
  newTable.name = ''
  newTable.columns = []
  addNewTableColumn({ name: 'id', type: 'bigint unsigned', nullable: false, primary: true, autoIncrement: true })
  showCreateTable.value = true
}
function closeCreateTable() { if (!creatingTable.value) showCreateTable.value = false }
function removeNewTableColumn(id: number) { if (newTable.columns.length > 1) newTable.columns = newTable.columns.filter((column) => column.id !== id) }
function newTableSQL() {
  const name = newTable.name.trim()
  if (!/^[A-Za-z_][A-Za-z0-9_$]*$/.test(name)) throw new Error(t('database.createTableInvalidName'))
  if (!newTable.columns.length) throw new Error(t('database.createTableColumnsRequired'))
  const names = new Set<string>()
  const definitions = newTable.columns.map((column) => {
    const columnName = column.name.trim()
    const type = column.type.trim()
    if (!/^[A-Za-z_][A-Za-z0-9_$]*$/.test(columnName) || names.has(columnName.toLowerCase())) throw new Error(t('database.createTableInvalidColumn'))
    if (!type || /;|--|#|\/\*|\*\//.test(type)) throw new Error(t('database.createTableInvalidDefinition'))
    names.add(columnName.toLowerCase())
    return `${identifierSQL(columnName)} ${type} ${column.nullable ? 'NULL' : 'NOT NULL'}${column.autoIncrement ? ' AUTO_INCREMENT' : ''}`
  })
  const primary = newTable.columns.filter((column) => column.primary).map((column) => identifierSQL(column.name.trim()))
  if (primary.length) definitions.push(`PRIMARY KEY (${primary.join(', ')})`)
  return `CREATE TABLE ${tableNameSQL(name)} (\n  ${definitions.join(',\n  ')}\n) ENGINE=InnoDB`
}
async function createTable() {
  if (creatingTable.value) return
  creatingTable.value = true
  try {
    const name = newTable.name.trim()
    const response = await api<QueryResult>(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql: newTableSQL(), historySql: `Create table ${props.database}.${name}` } })
    if (response.transactionPending) emit('transactionStatus', props.connectionId, response.transactionPending, response.pendingStatements)
    showCreateTable.value = false
    if (response.transactionPending) {
      notifySuccess(t('transaction.queued'))
      return
    }
    diagram.value = undefined
    await load()
    notifySuccess(t('database.createTableSuccess', { table: name }))
    emit('table', name)
  } catch (cause: unknown) { notifyError(messageFor(cause)) }
  finally { creatingTable.value = false }
}
function selectSection(next: DatabaseSection) {
  section.value = next
  emit('update:activeSection', next)
  if (next === 'tables') focusTableFilterInput()
  if (next === 'diagram' && !diagram.value) void loadDiagram()
}

function focusTableFilterInput() {
  if (section.value !== 'tables') return
  nextTick(() => filterInput.value?.focus())
}
function focusTableFilter(event: KeyboardEvent) {
  if (!(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== 'f' || section.value !== 'tables' || !filterInput.value) return
  event.preventDefault()
  focusTableFilterInput()
  filterInput.value.select()
}

onActivated(() => { isActive.value = true; window.addEventListener('keydown', focusTableFilter); focusTableFilterInput() })
onMounted(() => { if (localStorage.getItem(migrationStorageKey())) { migrating.value = true; void pollMigration() } })
onDeactivated(() => { isActive.value = false; window.removeEventListener('keydown', focusTableFilter) })
onBeforeUnmount(() => {
  window.removeEventListener('keydown', focusTableFilter)
  if (migrationPollTimer) clearTimeout(migrationPollTimer)
})
watch(() => [props.connectionId, props.database], () => { load(); diagram.value = undefined; if (section.value === 'diagram') void loadDiagram() }, { immediate: true })
watch(() => props.activeSection, (next) => {
  if (!next || next === section.value) return
  section.value = next
  if (next === 'tables') focusTableFilterInput()
  if (next === 'diagram' && !diagram.value) void loadDiagram()
})
watch(() => source.connectionId, async (connectionId) => {
  source.database = ''
  sourceDatabases.value = []
  if (!connectionId) return
  sourceLoading.value = true
  try { sourceDatabases.value = await api<DatabaseInfo[]>(`/connections/${connectionId}/databases`) }
  catch (cause: unknown) { notifyError(messageFor(cause)) }
  finally { sourceLoading.value = false }
})
</script>

<template>
  <section class="flex h-full min-h-0 flex-col">
    <header class="space-y-3 border-b border-line px-5 py-4 lg:px-7">
      <div class="flex items-center gap-2"><Icon name="lucide:database" class="h-5 w-5 text-muted" aria-hidden="true" /><h1 class="text-xl font-semibold">{{ database }}</h1></div>
      <div class="inline-flex rounded-md border border-line p-0.5 text-xs">
          <button type="button" class="rounded px-2.5 py-1" :class="section === 'tables' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('tables')">{{ t('database.viewTables') }}</button>
          <button type="button" class="rounded px-2.5 py-1" :class="section === 'diagram' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('diagram')">{{ t('database.viewDiagram') }}</button>
          <button type="button" class="rounded px-2.5 py-1" :class="section === 'tools' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('tools')">{{ t('database.viewTools') }}</button>
      </div>
    </header>
    <div v-if="section === 'tables'" class="flex flex-wrap items-center justify-end gap-3 border-b border-line bg-canvas/40 px-5 py-3 lg:px-7">
      <label class="flex min-w-0 w-full items-center gap-2 rounded-lg border border-line bg-panel px-3 py-2 text-muted shadow-sm sm:w-80">
        <Icon name="lucide:search" class="h-4 w-4 shrink-0" aria-hidden="true" />
        <span class="sr-only">{{ t('stats.filter') }}</span>
        <input ref="filterInput" v-model="filter" class="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted" :placeholder="t('database.filterPlaceholder')" >
      </label>
      <button type="button" class="inline-flex shrink-0 items-center gap-2 rounded-md bg-accent px-3 py-2 text-sm font-medium text-white disabled:opacity-50" :disabled="creatingTable" @click="openCreateTable"><Icon name="lucide:plus" class="h-4 w-4" aria-hidden="true" />{{ t('database.createTable') }}</button>
    </div>

    <div v-if="section === 'tables'" class="scrollbar min-h-0 flex-1 overflow-auto px-5 py-6 lg:px-7">
      <div v-if="loading" class="grid h-64 place-items-center text-sm text-muted">{{ t('tree.loadingTables') }}</div>
      <div v-else class="overflow-hidden rounded-lg border border-line bg-panel">
        <div class="scrollbar overflow-auto">
          <table class="min-w-full text-left text-sm">
            <thead class="sticky top-0 bg-panel text-xs uppercase tracking-wide text-muted"><tr><th class="border-b border-line px-4 py-3 font-medium">{{ t('table.table') }}</th><th class="border-b border-line px-4 py-3 font-medium">{{ t('table.columns') }}</th><th class="w-10 border-b border-line px-4 py-3" /></tr></thead>
            <tbody>
              <tr v-for="table in filteredTables" :key="table.name" class="cursor-pointer border-b border-line last:border-b-0 hover:bg-canvas" @click="emit('table', table.name)">
                <td class="px-4 py-2.5 font-medium"><span class="flex items-center gap-2"><Icon name="lucide:table-2" class="h-3.5 w-3.5 shrink-0 text-muted" aria-hidden="true" />{{ table.name }}</span></td>
                <td class="px-4 py-2.5 text-muted">{{ table.columnCount }}</td>
                <td class="px-4 py-2.5 text-right"><button type="button" class="grid h-6 w-6 place-items-center rounded text-muted hover:bg-line hover:text-ink" :title="t('tree.viewTable')" :aria-label="t('tree.viewTable')" @click.stop="emit('table', table.name)"><Icon name="lucide:eye" class="h-3.5 w-3.5" aria-hidden="true" /></button></td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!filteredTables.length" class="px-4 py-8 text-center text-sm text-muted">{{ filter ? t('stats.noMatches') : t('database.noTables') }}</p>
      </div>
    </div>
    <div v-else-if="section === 'diagram'" class="min-h-0 flex-1"><ErDiagram :tables="diagram?.tables ?? []" :loading="diagramLoading" @open-table="emit('table', $event)" /></div>
    <div v-else class="scrollbar min-h-0 flex-1 overflow-auto px-5 py-6 lg:px-7">
      <header class="border-b border-line pb-5"><h2 class="text-xl font-semibold">{{ t('database.tools.title') }}</h2><p class="mt-1 text-sm text-muted">{{ t('database.tools.description') }}</p></header>
      <div class="mt-6 flex min-h-0 flex-1 flex-col gap-6 md:flex-row">
        <nav class="flex shrink-0 gap-1 border-b border-line pb-4 md:w-48 md:flex-col md:border-b-0 md:border-r md:pb-0 md:pr-5">
          <button type="button" class="tools-nav flex items-center gap-2" :class="toolsSection === 'export' ? 'tools-nav-active' : ''" @click="toolsSection = 'export'"><Icon name="lucide:download" class="h-4 w-4" aria-hidden="true" />{{ t('database.tools.exportTitle') }}</button>
          <button type="button" class="tools-nav flex items-center gap-2" :class="toolsSection === 'import' ? 'tools-nav-active' : ''" @click="toolsSection = 'import'"><Icon name="lucide:upload" class="h-4 w-4" aria-hidden="true" />{{ t('database.tools.importTitle') }}</button>
          <button type="button" class="tools-nav flex items-center gap-2" :class="toolsSection === 'migration' ? 'tools-nav-active' : ''" @click="toolsSection = 'migration'"><Icon name="lucide:arrow-right-left" class="h-4 w-4" aria-hidden="true" />{{ t('database.tools.migrationTitle') }}</button>
          <button type="button" class="tools-nav flex items-center gap-2" :class="toolsSection === 'maintenance' ? 'tools-nav-active' : ''" @click="toolsSection = 'maintenance'"><Icon name="lucide:wrench" class="h-4 w-4" aria-hidden="true" />{{ t('database.tools.maintenanceTitle') }}</button>
          <button type="button" class="tools-nav tools-nav-danger flex items-center gap-2" :class="toolsSection === 'danger' ? 'tools-nav-danger-active' : ''" @click="toolsSection = 'danger'"><Icon name="lucide:trash-2" class="h-4 w-4" aria-hidden="true" />{{ t('database.tools.deleteTitle') }}</button>
        </nav>
        <div class="min-w-0 flex-1 pb-8">
          <section v-if="toolsSection === 'export'" class="max-w-xl"><h3 class="text-base font-semibold">{{ t('database.tools.exportTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('database.tools.exportDescription') }}</p><label class="mt-5 flex items-start gap-3 text-sm"><input v-model="exportStructureOnly" class="mt-0.5" type="checkbox" :disabled="exporting"><span><span class="font-medium">{{ t('database.tools.exportStructureOnly') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('database.tools.exportStructureOnlyDescription') }}</span></span></label><button type="button" class="mt-5 rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="exporting" @click="exportDatabase">{{ exporting ? t('database.tools.exporting') : t('database.tools.export') }}</button></section>
          <section v-else-if="toolsSection === 'import'" class="max-w-xl"><h3 class="text-base font-semibold">{{ t('database.tools.importTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('database.tools.importDescription', { database }) }}</p><input ref="dumpInput" class="sr-only" type="file" accept=".sql,application/sql,text/plain" @change="requestDumpImport"><button type="button" class="mt-5 flex items-center gap-1.5 rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="importingDump" @click="dumpInput?.click()"><Icon name="lucide:upload" class="h-4 w-4" aria-hidden="true" />{{ importingDump ? t('database.tools.importing') : t('database.tools.import') }}</button></section>
          <section v-else-if="toolsSection === 'migration'" class="max-w-3xl">
            <h3 class="text-base font-semibold">{{ t('database.tools.migrationTitle') }}</h3>
            <p class="mt-1 text-sm text-muted">{{ t('database.tools.migrationDescription', { database }) }}</p>
            <div class="mt-6 grid gap-3 md:grid-cols-2"><label class="grid gap-1.5 text-sm font-medium">{{ t('database.tools.sourceConnection') }}<AppSelect v-model="source.connectionId" :options="sourceConnectionOptions" :disabled="migrating" :placeholder="t('database.tools.chooseConnection')" /></label><label class="grid gap-1.5 text-sm font-medium">{{ t('database.tools.sourceDatabase') }}<AppSelect v-model="source.database" :options="sourceDatabaseOptions" :disabled="migrating || sourceLoading || !source.connectionId" :placeholder="t('database.tools.chooseDatabase')" /></label></div>
            <div class="mt-5 grid gap-3"><label class="flex items-start gap-3 text-sm"><input v-model="migrationOptions.recreateTarget" class="mt-0.5" type="checkbox" :disabled="migrating" ><span><span class="font-medium">{{ t('database.tools.recreateTarget') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('database.tools.recreateTargetDescription') }}</span></span></label><label class="flex items-start gap-3 text-sm"><input v-model="migrationOptions.structureOnly" class="mt-0.5" type="checkbox" :disabled="migrating" ><span><span class="font-medium">{{ t('database.tools.migrationStructureOnly') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('database.tools.migrationStructureOnlyDescription') }}</span></span></label><label class="flex items-start gap-3 text-sm" :class="migrationOptions.structureOnly ? 'opacity-50' : ''"><input v-model="migrationOptions.ignoreDuplicates" class="mt-0.5" type="checkbox" :disabled="migrating || migrationOptions.structureOnly" ><span><span class="font-medium">{{ t('database.tools.ignoreDuplicates') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('database.tools.ignoreDuplicatesDescription') }}</span></span></label></div>
            <div v-if="migrationJob" class="mt-5 rounded-md border border-line bg-canvas p-3 text-sm">
              <div class="flex items-center justify-between gap-3"><span>{{ t('connectionTools.progressSummary', { completed: migrationJob.progress.completedTables, total: migrationJob.progress.totalTables, rows: migrationJob.progress.rowsMigrated }) }}</span><span class="font-medium">{{ migrationProgressPercent }}%</span></div>
              <div class="mt-2 h-2 overflow-hidden rounded-full bg-line"><div class="h-full rounded-full bg-accent transition-all" :style="{ width: `${migrationProgressPercent}%` }" /></div>
              <p v-if="migrationJob.status === 'running' && migrationJob.progress.currentTable" class="mt-2 font-mono text-xs text-muted">{{ migrationJob.progress.currentDatabase }}.{{ migrationJob.progress.currentTable }}</p>
              <details v-if="migrationJob.result?.failedTables.length" class="mt-3"><summary class="cursor-pointer text-rose-600">{{ t('connectionTools.failedTables', { count: migrationJob.result.failedTables.length }) }}</summary><ul class="mt-2 grid gap-2 text-xs"><li v-for="failure in migrationJob.result.failedTables" :key="`${failure.database}.${failure.table}`"><span class="font-mono">{{ failure.database }}.{{ failure.table }}</span><span class="block text-muted">{{ failure.error }}</span></li></ul></details>
            </div>
            <button type="button" class="mt-6 rounded-md bg-accent px-3 py-2 text-sm text-white disabled:opacity-50" :disabled="migrating || sourceLoading || !source.database" @click="requestMigration">{{ migrating ? t('database.tools.migrating') : t('database.tools.migrate') }}</button>
          </section>
          <section v-else-if="toolsSection === 'maintenance'" class="max-w-3xl"><h3 class="text-base font-semibold">{{ t('database.tools.maintenanceTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('database.tools.maintenanceDescription') }}</p><div class="mt-5 flex flex-wrap gap-2"><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="Boolean(maintenanceRunning)" @click="runMaintenance('check')">{{ maintenanceRunning === 'check' ? t('database.tools.running') : t('database.tools.check') }}</button><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="Boolean(maintenanceRunning)" @click="runMaintenance('analyze')">{{ maintenanceRunning === 'analyze' ? t('database.tools.running') : t('database.tools.analyze') }}</button><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="Boolean(maintenanceRunning)" @click="runMaintenance('repair')">{{ maintenanceRunning === 'repair' ? t('database.tools.running') : t('database.tools.repair') }}</button></div><div v-if="maintenanceResult?.columns.length" class="scrollbar mt-5 overflow-auto rounded-md border border-line"><table class="min-w-full text-left text-xs"><thead class="bg-canvas text-muted"><tr><th v-for="column in maintenanceResult.columns" :key="column.name" class="px-3 py-2 font-medium">{{ column.name }}</th></tr></thead><tbody><tr v-for="(row, index) in maintenanceResult.rows" :key="index" class="border-t border-line"><td v-for="column in maintenanceResult.columns" :key="column.name" class="px-3 py-2">{{ row[column.name] }}</td></tr></tbody></table></div></section>
          <section v-else class="max-w-xl"><h3 class="text-base font-semibold text-rose-600">{{ t('database.tools.deleteTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('database.tools.deleteDescription', { database }) }}</p><button type="button" class="mt-5 rounded-md border border-rose-500/40 px-3 py-2 text-sm text-rose-600 hover:bg-rose-500/10 disabled:opacity-50" :disabled="deletingDatabase" @click="showDeleteConfirmation = true">{{ deletingDatabase ? t('database.tools.deleting') : t('database.tools.delete') }}</button></section>
        </div>
      </div>
    </div>
    <AppConfirmDialog v-model="showRecreateConfirmation" :title="t('database.tools.recreateTargetTitle')" :description="t('database.tools.recreateTargetConfirm', { database })" :confirm-label="t('database.tools.recreateAndMigrate')" :cancel-label="t('common.close')" tone="danger" @confirm="confirmRecreateMigration" />
    <AppConfirmDialog v-model="showImportConfirmation" :title="t('database.tools.importConfirmTitle')" :description="t('database.tools.importConfirm', { database })" :confirm-label="t('database.tools.recreateAndImport')" :cancel-label="t('common.close')" tone="danger" @confirm="importDatabaseDump" />
    <AppConfirmDialog v-model="showDeleteConfirmation" :title="t('database.tools.deleteConfirmTitle')" :description="t('database.tools.deleteConfirm', { database })" :confirm-label="t('database.tools.delete')" :cancel-label="t('common.close')" tone="danger" @confirm="deleteDatabase" />
    <Teleport to="body">
      <div v-if="showCreateTable" class="fixed inset-0 z-[60] grid place-items-center bg-slate-950/55 p-4 backdrop-blur-sm" @mousedown.self="closeCreateTable">
        <form class="flex max-h-[calc(100vh-2rem)] w-full max-w-3xl flex-col overflow-hidden rounded-2xl border border-line bg-panel shadow-2xl" @submit.prevent="createTable">
          <div class="scrollbar overflow-auto p-6"><h2 class="text-base font-semibold tracking-tight">{{ t('database.createTableTitle') }}</h2><p class="mt-2 text-sm leading-6 text-muted">{{ t('database.createTableDescription') }}</p><label class="mt-5 grid max-w-sm gap-1.5 text-sm font-medium">{{ t('table.name') }}<input v-model.trim="newTable.name" class="rounded-md border border-line bg-canvas px-3 py-2 text-ink outline-none focus:border-accent" required autofocus :disabled="creatingTable" :placeholder="t('database.createTableNamePlaceholder')"></label><div class="mt-6 flex items-center justify-between gap-3"><h3 class="text-sm font-semibold">{{ t('database.createTableColumns') }}</h3><button type="button" class="inline-flex items-center gap-1.5 rounded-md border border-line px-2.5 py-1.5 text-sm hover:bg-canvas disabled:opacity-50" :disabled="creatingTable" @click="addNewTableColumn()"><Icon name="lucide:plus" class="h-4 w-4" aria-hidden="true" />{{ t('database.createTableAddColumn') }}</button></div><div class="mt-3 overflow-hidden rounded-lg border border-line"><div class="scrollbar overflow-x-auto"><table class="min-w-[620px] w-full text-left text-sm"><thead class="bg-canvas text-xs text-muted"><tr><th class="px-3 py-2 font-medium">{{ t('table.column') }}</th><th class="px-3 py-2 font-medium">{{ t('table.type') }}</th><th class="px-3 py-2 text-center font-medium">{{ t('table.nullable') }}</th><th class="px-3 py-2 text-center font-medium">{{ t('database.createTablePrimary') }}</th><th class="px-3 py-2 text-center font-medium">{{ t('database.createTableAutoIncrement') }}</th><th class="w-10 px-2 py-2"><span class="sr-only">{{ t('grid.actions') }}</span></th></tr></thead><tbody><tr v-for="column in newTable.columns" :key="column.id" class="border-t border-line"><td class="p-2"><input v-model.trim="column.name" class="w-full rounded border border-line bg-canvas px-2 py-1.5 text-ink outline-none focus:border-accent" required :disabled="creatingTable" :placeholder="t('table.structureNamePlaceholder')"></td><td class="p-2"><input v-model.trim="column.type" class="w-full rounded border border-line bg-canvas px-2 py-1.5 font-mono text-ink outline-none focus:border-accent" required :disabled="creatingTable" placeholder="varchar(255)"></td><td class="p-2 text-center"><input v-model="column.nullable" type="checkbox" :disabled="creatingTable"></td><td class="p-2 text-center"><input v-model="column.primary" type="checkbox" :disabled="creatingTable"></td><td class="p-2 text-center"><input v-model="column.autoIncrement" type="checkbox" :disabled="creatingTable"></td><td class="p-2 text-center"><button type="button" class="grid rounded p-1 text-rose-600 hover:bg-rose-500/10 disabled:opacity-50" :disabled="creatingTable || newTable.columns.length === 1" :title="t('database.createTableRemoveColumn')" :aria-label="t('database.createTableRemoveColumn')" @click="removeNewTableColumn(column.id)"><Icon name="lucide:trash-2" class="h-4 w-4" aria-hidden="true" /></button></td></tr></tbody></table></div></div></div>
          <div class="flex flex-col-reverse gap-2 border-t border-line bg-canvas/40 px-5 py-4 sm:flex-row sm:justify-end"><button type="button" class="rounded-lg px-3.5 py-2 text-sm font-medium text-muted hover:bg-panel hover:text-ink" :disabled="creatingTable" @click="closeCreateTable">{{ t('common.close') }}</button><button type="submit" class="rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-white shadow-sm disabled:opacity-50" :disabled="creatingTable">{{ creatingTable ? t('database.createTableCreating') : t('database.createTable') }}</button></div>
        </form>
      </div>
    </Teleport>
  </section>
</template>

<style scoped>
.tools-nav { @apply rounded-md px-3 py-2 text-left text-sm text-muted hover:bg-canvas hover:text-ink; }
.tools-nav-active { @apply bg-accent/10 font-medium text-accent hover:bg-accent/10 hover:text-accent; }
.tools-nav-danger { @apply text-rose-600 hover:bg-rose-500/10 hover:text-rose-600; }
.tools-nav-danger-active { @apply bg-rose-500/10 font-medium text-rose-600 hover:bg-rose-500/10 hover:text-rose-600; }
</style>
