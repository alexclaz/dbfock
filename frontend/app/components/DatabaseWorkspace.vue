<script setup lang="ts">
import type { DatabaseInfo, QueryResult, SchemaDiagram, TableInfo, TableStructure } from '~/types/database'
import { tableInsertStatements } from '~/utils/tableTransfer'

type DatabaseSection = 'tables' | 'diagram' | 'tools'
type DatabaseToolsSection = 'export' | 'import' | 'migration' | 'maintenance' | 'danger'

const props = defineProps<{ connectionId: string; database: string; activeSection?: DatabaseSection }>()
const emit = defineEmits<{ table: [table: string]; 'update:activeSection': [value: DatabaseSection]; transactionStatus: [connectionId: string, pending: boolean, pendingStatements: number]; databaseDeleted: [connectionId: string, database: string] }>()
const api = useApi()
const workspace = useWorkspaceStore()
const { t } = useI18n()
const { error: notifyError, success: notifySuccess } = useToast()
const tables = ref<TableInfo[]>()
const loading = ref(true)
const filter = ref('')
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
const source = reactive({ connectionId: '', database: '' })
const migrationOptions = reactive({ recreateTarget: false, ignoreDuplicates: false })
const showRecreateConfirmation = ref(false)
const showDeleteConfirmation = ref(false)
const deletingDatabase = ref(false)

function messageFor(cause: unknown) { return cause instanceof Error ? cause.message : String(cause) }

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
function sourceTablePath(table: string, suffix: 'structure' | 'data') { return `/connections/${source.connectionId}/databases/${encodeURIComponent(source.database)}/tables/${encodeURIComponent(table)}/${suffix}` }
function createTableSQL(ddl: string, table: string) {
  const name = tableNameSQL(table)
  const created = ddl.replace(/^CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:`(?:``|[^`])+`\.)?`(?:``|[^`])+`/i, `CREATE TABLE IF NOT EXISTS ${name}`)
  if (created === ddl) throw new Error(t('database.tools.migrationCreateError', { table }))
  return created
}
function downloadDump(contents: string) {
  const link = document.createElement('a')
  link.href = URL.createObjectURL(new Blob([contents], { type: 'application/sql;charset=utf-8' }))
  link.download = `dbfock-dump-${props.database}-${new Date().toISOString().slice(0, 10)}.sql`.replaceAll(/[^a-zA-Z0-9._-]/g, '-')
  link.click()
  URL.revokeObjectURL(link.href)
}
async function exportDatabase() {
  if (exporting.value) return
  exporting.value = true
  try {
    const list = tables.value ?? await api<TableInfo[]>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables`)
    const structures = await Promise.all(list.map((table) => api<TableStructure>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables/${encodeURIComponent(table.name)}/structure`)))
    const databaseName = `\`${props.database.replaceAll('`', '``')}\``
    const script = ['SET FOREIGN_KEY_CHECKS=0', `CREATE DATABASE IF NOT EXISTS ${databaseName}`, `USE ${databaseName}`, ...structures.map((structure) => structure.ddl.trim().replace(/;?$/, ''))]
    if (!exportStructureOnly.value) {
      for (let index = 0; index < list.length; index++) {
        const table = list[index]!
        const structure = structures[index]!
        const columnTypes = Object.fromEntries(structure.columns.map((column) => [column.name, column.databaseType]))
        let offset = 0
        let hasMore = true
        while (hasMore) {
          const page = await api<QueryResult>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables/${encodeURIComponent(table.name)}/data?limit=500&offset=${offset}`)
          script.push(...tableInsertStatements(props.database, table.name, page.columns.map((column) => column.name), page.rows.map((row) => page.columns.map((column) => row[column.name])), 80_000, columnTypes))
          offset += page.rows.length
          hasMore = page.hasMore
        }
      }
    }
    script.push('SET FOREIGN_KEY_CHECKS=1')
    downloadDump(`${script.join(';\n')};\n`)
    notifySuccess(t('database.tools.exportSuccess', { count: structures.length }))
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
    const sourceTables = await api<TableInfo[]>(`/connections/${source.connectionId}/databases/${encodeURIComponent(source.database)}/tables`)
    if (!sourceTables.length) throw new Error(t('database.tools.noSourceTables'))
    if (migrationOptions.recreateTarget) {
      const targetTables = tables.value ?? await api<TableInfo[]>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables`)
      if (targetTables.length) await api(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql: `DROP TABLE IF EXISTS ${targetTables.map((table) => tableNameSQL(table.name)).join(', ')}`, historySql: `Drop all tables in ${props.database} before database migration` } })
      tables.value = []
    }
    const structures = new Map<string, TableStructure>()
    for (const table of sourceTables) structures.set(table.name, await api<TableStructure>(sourceTablePath(table.name, 'structure')))

    const pending = [...sourceTables]
    let lastError: unknown
    while (pending.length) {
      let created = 0
      for (let index = pending.length - 1; index >= 0; index--) {
        const table = pending[index]!
        try {
          await api(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql: createTableSQL(structures.get(table.name)!.ddl, table.name), historySql: `Create ${props.database}.${table.name} during database migration` } })
          pending.splice(index, 1)
          created++
        } catch (cause: unknown) { lastError = cause }
      }
      if (!created) throw lastError instanceof Error ? lastError : new Error(t('database.tools.migrationCreateError', { table: pending[0]?.name ?? '' }))
    }

    let migratedRows = 0
    for (const table of sourceTables) {
      const targetStructure = await api<TableStructure>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables/${encodeURIComponent(table.name)}/structure`)
      const allowedColumns = new Set(targetStructure.columns.map((column) => column.name))
      const columnTypes = Object.fromEntries(targetStructure.columns.map((column) => [column.name, column.databaseType]))
      let columns: string[] = []
      let offset = 0
      let hasMore = true
      while (hasMore) {
        const page = await api<QueryResult>(`${sourceTablePath(table.name, 'data')}?limit=100&offset=${offset}`)
        if (!columns.length) columns = page.columns.map((column) => column.name).filter((column) => allowedColumns.has(column))
        if (columns.length) for (const sql of tableInsertStatements(props.database, table.name, columns, page.rows.map((row) => columns.map((column) => row[column])), 80_000, columnTypes, migrationOptions.ignoreDuplicates)) {
          const result = await api<QueryResult>(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql, historySql: `Migrate ${source.database}.${table.name} into ${props.database}.${table.name}` } })
          migratedRows += result.affectedRows
          if (result.transactionPending) emit('transactionStatus', props.connectionId, result.transactionPending, result.pendingStatements)
        }
        offset += page.rows.length
        hasMore = page.hasMore
      }
    }
    await load()
    notifySuccess(t('database.tools.migrationSuccess', { count: migratedRows }))
  } catch (cause: unknown) { notifyError(messageFor(cause)) }
  finally { migrating.value = false }
}
function requestMigration() {
  if (migrationOptions.recreateTarget) { showRecreateConfirmation.value = true; return }
  void migrateDatabase()
}
function confirmRecreateMigration() { showRecreateConfirmation.value = false; void migrateDatabase() }
async function deleteDatabase() {
  showDeleteConfirmation.value = false
  deletingDatabase.value = true
  try {
    await api(`/connections/${props.connectionId}/query`, { method: 'POST', body: { sql: `DROP DATABASE ${databaseNameSQL()}`, historySql: `Drop database ${props.database}` } })
    notifySuccess(t('database.tools.deleteSuccess', { database: props.database }))
    emit('databaseDeleted', props.connectionId, props.database)
  } catch (cause: unknown) { notifyError(messageFor(cause)) }
  finally { deletingDatabase.value = false }
}
function selectSection(next: DatabaseSection) {
  section.value = next
  emit('update:activeSection', next)
  if (next === 'diagram' && !diagram.value) void loadDiagram()
}

watch(() => [props.connectionId, props.database], () => { load(); diagram.value = undefined; if (section.value === 'diagram') void loadDiagram() }, { immediate: true })
watch(() => props.activeSection, (next) => {
  if (!next || next === section.value) return
  section.value = next
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
    <div v-if="section === 'tables'" class="border-b border-line bg-canvas/40 px-5 py-3 lg:px-7">
      <label class="flex w-full max-w-md items-center gap-2 rounded-lg border border-line bg-panel px-3 py-2 text-muted shadow-sm">
        <Icon name="lucide:search" class="h-4 w-4 shrink-0" aria-hidden="true" />
        <span class="sr-only">{{ t('stats.filter') }}</span>
        <input v-model="filter" class="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted" :placeholder="t('database.filterPlaceholder')" >
      </label>
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
          <section v-else-if="toolsSection === 'migration'" class="max-w-3xl"><h3 class="text-base font-semibold">{{ t('database.tools.migrationTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('database.tools.migrationDescription', { database }) }}</p><div class="mt-6 grid gap-3 md:grid-cols-2"><label class="grid gap-1.5 text-sm font-medium">{{ t('database.tools.sourceConnection') }}<AppSelect v-model="source.connectionId" :options="sourceConnectionOptions" :disabled="migrating" :placeholder="t('database.tools.chooseConnection')" /></label><label class="grid gap-1.5 text-sm font-medium">{{ t('database.tools.sourceDatabase') }}<AppSelect v-model="source.database" :options="sourceDatabaseOptions" :disabled="migrating || sourceLoading || !source.connectionId" :placeholder="t('database.tools.chooseDatabase')" /></label></div><div class="mt-5 grid gap-3"><label class="flex items-start gap-3 text-sm"><input v-model="migrationOptions.recreateTarget" class="mt-0.5" type="checkbox" :disabled="migrating" ><span><span class="font-medium">{{ t('database.tools.recreateTarget') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('database.tools.recreateTargetDescription') }}</span></span></label><label class="flex items-start gap-3 text-sm"><input v-model="migrationOptions.ignoreDuplicates" class="mt-0.5" type="checkbox" :disabled="migrating" ><span><span class="font-medium">{{ t('database.tools.ignoreDuplicates') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('database.tools.ignoreDuplicatesDescription') }}</span></span></label></div><button type="button" class="mt-6 rounded-md bg-accent px-3 py-2 text-sm text-white disabled:opacity-50" :disabled="migrating || sourceLoading || !source.database" @click="requestMigration">{{ migrating ? t('database.tools.migrating') : t('database.tools.migrate') }}</button></section>
          <section v-else-if="toolsSection === 'maintenance'" class="max-w-3xl"><h3 class="text-base font-semibold">{{ t('database.tools.maintenanceTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('database.tools.maintenanceDescription') }}</p><div class="mt-5 flex flex-wrap gap-2"><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="Boolean(maintenanceRunning)" @click="runMaintenance('check')">{{ maintenanceRunning === 'check' ? t('database.tools.running') : t('database.tools.check') }}</button><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="Boolean(maintenanceRunning)" @click="runMaintenance('analyze')">{{ maintenanceRunning === 'analyze' ? t('database.tools.running') : t('database.tools.analyze') }}</button><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="Boolean(maintenanceRunning)" @click="runMaintenance('repair')">{{ maintenanceRunning === 'repair' ? t('database.tools.running') : t('database.tools.repair') }}</button></div><div v-if="maintenanceResult?.columns.length" class="scrollbar mt-5 overflow-auto rounded-md border border-line"><table class="min-w-full text-left text-xs"><thead class="bg-canvas text-muted"><tr><th v-for="column in maintenanceResult.columns" :key="column.name" class="px-3 py-2 font-medium">{{ column.name }}</th></tr></thead><tbody><tr v-for="(row, index) in maintenanceResult.rows" :key="index" class="border-t border-line"><td v-for="column in maintenanceResult.columns" :key="column.name" class="px-3 py-2">{{ row[column.name] }}</td></tr></tbody></table></div></section>
          <section v-else class="max-w-xl"><h3 class="text-base font-semibold text-rose-600">{{ t('database.tools.deleteTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('database.tools.deleteDescription', { database }) }}</p><button type="button" class="mt-5 rounded-md border border-rose-500/40 px-3 py-2 text-sm text-rose-600 hover:bg-rose-500/10 disabled:opacity-50" :disabled="deletingDatabase" @click="showDeleteConfirmation = true">{{ deletingDatabase ? t('database.tools.deleting') : t('database.tools.delete') }}</button></section>
        </div>
      </div>
    </div>
    <AppConfirmDialog v-model="showRecreateConfirmation" :title="t('database.tools.recreateTargetTitle')" :description="t('database.tools.recreateTargetConfirm', { database })" :confirm-label="t('database.tools.recreateAndMigrate')" :cancel-label="t('common.close')" tone="danger" @confirm="confirmRecreateMigration" />
    <AppConfirmDialog v-model="showImportConfirmation" :title="t('database.tools.importConfirmTitle')" :description="t('database.tools.importConfirm', { database })" :confirm-label="t('database.tools.recreateAndImport')" :cancel-label="t('common.close')" tone="danger" @confirm="importDatabaseDump" />
    <AppConfirmDialog v-model="showDeleteConfirmation" :title="t('database.tools.deleteConfirmTitle')" :description="t('database.tools.deleteConfirm', { database })" :confirm-label="t('database.tools.delete')" :cancel-label="t('common.close')" tone="danger" @confirm="deleteDatabase" />
  </section>
</template>

<style scoped>
.tools-nav { @apply rounded-md px-3 py-2 text-left text-sm text-muted hover:bg-canvas hover:text-ink; }
.tools-nav-active { @apply bg-accent/10 font-medium text-accent hover:bg-accent/10 hover:text-accent; }
.tools-nav-danger { @apply text-rose-600 hover:bg-rose-500/10 hover:text-rose-600; }
.tools-nav-danger-active { @apply bg-rose-500/10 font-medium text-rose-600 hover:bg-rose-500/10 hover:text-rose-600; }
</style>
