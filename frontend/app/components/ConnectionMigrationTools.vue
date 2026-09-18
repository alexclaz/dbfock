<script setup lang="ts">
import type { Connection, DatabaseInfo, DatabaseMigrationJob, DatabaseMigrationPlan, DatabaseMigrationResult, SchemaComparison } from '~/types/database'

const props = defineProps<{ connection: Connection }>()
const api = useApi()
const store = useWorkspaceStore()
const { t } = useI18n()
const { error: notifyError, success: notifySuccess, info: notifyInfo } = useToast()
const sourceConnectionId = ref('')
const sourceDatabases = ref<DatabaseInfo[]>([])
const selectedDatabases = ref<string[]>([])
const loadingDatabases = ref(false)
const comparing = ref(false)
const comparison = ref<SchemaComparison>()
const previewing = ref(false)
const migrationPlan = ref<DatabaseMigrationPlan>()
const selectedTableKeys = ref<string[]>([])
const migrating = ref(false)
const showMigrationConfirmation = ref(false)
const maxTableSizeMB = ref(10)
const strategy = ref<'drop_recreate' | 'truncate_insert'>('drop_recreate')
const createMissingTables = ref(true)
const skipMatchingTables = ref(false)
const createSkippedTableStructures = ref(false)
const migrationResult = ref<DatabaseMigrationResult>()
const migrationJob = ref<DatabaseMigrationJob>()
let migrationPollTimer: ReturnType<typeof setTimeout> | undefined

const connectionOptions = computed(() => store.connections
  .filter(connection => connection.id !== props.connection.id)
  .map(connection => ({ value: connection.id, label: `${connection.name} · ${connection.environment === 'production' ? t('connection.production') : t('connection.development')}`, disabled: connection.status !== 'connected' })))
const systemDatabases = new Set(['information_schema', 'mysql', 'performance_schema', 'sys'])
const databaseOptions = computed(() => sourceDatabases.value.filter(database => !systemDatabases.has(database.name.toLowerCase())).map(database => ({ value: database.name, label: database.name })))
const selectedSource = computed(() => store.connections.find(connection => connection.id === sourceConnectionId.value))
const canMigrate = computed(() => sourceConnectionId.value && selectedDatabases.value.length && Number.isFinite(maxTableSizeMB.value) && maxTableSizeMB.value > 0)
const migrationPercent = computed(() => {
  const progress = migrationJob.value?.progress
  if (!progress?.totalTables) return 0
  const currentFraction = progress.currentTableEstimatedRows > 0 ? Math.min(1, progress.currentTableRows / progress.currentTableEstimatedRows) : 0
  const percent = (progress.completedTables + currentFraction) / progress.totalTables * 100
  return Math.min(migrationJob.value?.status === 'running' ? 99 : 100, Math.round(percent))
})
function tableKey(database: string, table: string) { return `${database}\u0000${table}` }
const selectedMigrationTables = computed(() => (migrationPlan.value?.tables ?? []).filter(table => selectedTableKeys.value.includes(tableKey(table.database, table.table))))
const structureOnlyMigrationTables = computed(() => {
  if (!createSkippedTableStructures.value || !migrationPlan.value) return []
  return [
    ...migrationPlan.value.tables.filter(table => !selectedTableKeys.value.includes(tableKey(table.database, table.table))),
    ...migrationPlan.value.skippedTables,
  ]
})
const selectedEstimatedRows = computed(() => selectedMigrationTables.value.reduce((total, table) => total + table.estimatedRows, 0))
const selectedSourceSize = computed(() => selectedMigrationTables.value.reduce((total, table) => total + table.sizeBytes, 0))
const allTablesSelected = computed(() => Boolean(migrationPlan.value?.tables.length) && selectedTableKeys.value.length === migrationPlan.value!.tables.length)
const migrationConfirmationDescription = computed(() => {
  const base = t('connectionTools.confirmDescription', { source: selectedSource.value?.name || '', target: props.connection.name, count: selectedDatabases.value.length, strategy: t(`connectionTools.${strategy.value === 'drop_recreate' ? 'dropRecreate' : 'truncateInsert'}`) })
  if (!structureOnlyMigrationTables.value.length) return base
  return `${base} ${t('connectionTools.confirmStructureOnlyDescription', { count: structureOnlyMigrationTables.value.length })}`
})
function toggleAllTables() {
  selectedTableKeys.value = allTablesSelected.value ? [] : (migrationPlan.value?.tables ?? []).map(table => tableKey(table.database, table.table))
}

function errorMessage(cause: unknown) { return cause instanceof Error ? cause.message : t('tree.connectionError') }
function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}
function statusLabel(status: string) { return t(`connectionTools.status.${status}`) }
function columnSide(column: NonNullable<SchemaComparison['databases'][number]['tables'][number]['columns'][number]>, side: 'source' | 'target') {
  const type = side === 'source' ? column.sourceType : column.targetType
  if (!type) return '—'
  const nullable = side === 'source' ? column.sourceNullable : column.targetNullable
  const extra = side === 'source' ? column.sourceExtra : column.targetExtra
  return `${type}${nullable ? ' NULL' : ' NOT NULL'}${extra ? ` · ${extra}` : ''}`
}
async function compareSchemas() {
  if (!sourceConnectionId.value || comparing.value) return
  comparing.value = true
  comparison.value = undefined
  try {
    comparison.value = await api<SchemaComparison>(`/connections/${props.connection.id}/schema/compare`, { method: 'POST', body: { sourceConnectionId: sourceConnectionId.value } })
  } catch (cause: unknown) { notifyError(errorMessage(cause)) }
  finally { comparing.value = false }
}
function migrationPayload(includeSelection = false) {
  return {
    sourceConnectionId: sourceConnectionId.value,
    databases: selectedDatabases.value,
    maxTableSizeBytes: Math.floor(maxTableSizeMB.value * 1024 * 1024),
    strategy: strategy.value,
    createMissingTables: createMissingTables.value,
    skipMatchingTables: skipMatchingTables.value,
    createSkippedTableStructures: createSkippedTableStructures.value,
    ...(includeSelection ? {
      selectedTables: selectedMigrationTables.value.map(table => ({ database: table.database, table: table.table })),
      structureOnlyTables: structureOnlyMigrationTables.value.map(table => ({ database: table.database, table: table.table })),
    } : {}),
  }
}
async function previewMigration() {
  if (!canMigrate.value || previewing.value) return
  previewing.value = true
  migrationPlan.value = undefined
  migrationResult.value = undefined
  try {
    migrationPlan.value = await api<DatabaseMigrationPlan>(`/connections/${props.connection.id}/migrate/preview`, { method: 'POST', body: migrationPayload() })
    selectedTableKeys.value = migrationPlan.value.tables.map(table => tableKey(table.database, table.table))
  }
  catch (cause: unknown) { notifyError(errorMessage(cause)) }
  finally { previewing.value = false }
}
async function migrate() {
  showMigrationConfirmation.value = false
  if (!canMigrate.value || !migrationPlan.value || migrating.value) return
  migrating.value = true
  migrationResult.value = undefined
  try {
    migrationJob.value = await api<DatabaseMigrationJob>(`/connections/${props.connection.id}/migrate/jobs`, { method: 'POST', body: migrationPayload(true) })
    if (import.meta.client) localStorage.setItem(`dbfock.migration.${props.connection.id}`, migrationJob.value.id)
    scheduleMigrationPoll(100)
  } catch (cause: unknown) { migrating.value = false; notifyError(errorMessage(cause)) }
}
function scheduleMigrationPoll(delay = 750) {
  if (migrationPollTimer) clearTimeout(migrationPollTimer)
  migrationPollTimer = setTimeout(() => void pollMigration(), delay)
}
async function pollMigration() {
  const jobId = migrationJob.value?.id || (import.meta.client ? localStorage.getItem(`dbfock.migration.${props.connection.id}`) : '')
  if (!jobId) return
  try {
    const job = await api<DatabaseMigrationJob>(`/connections/${props.connection.id}/migrate/jobs/${jobId}`)
    migrationJob.value = job
    migrating.value = job.status === 'running'
    if (job.status === 'running') { scheduleMigrationPoll(); return }
    if (import.meta.client) localStorage.removeItem(`dbfock.migration.${props.connection.id}`)
    if (job.status === 'complete' && job.result) {
      migrationResult.value = job.result
      if (job.result.failedTables.length) notifyError(t('connectionTools.migrationPartial', { migrated: job.result.tablesMigrated, failed: job.result.failedTables.length }))
      else notifySuccess(t('connectionTools.migrationSuccess', { tables: job.result.tablesMigrated, rows: job.result.rowsMigrated }))
    } else if (job.error) notifyError(job.error)
  } catch (cause: unknown) {
    // A 404 means the server no longer knows this job: jobs live in memory, so a
    // backend restart loses them. Retrying forever would keep the form disabled.
    if (cause instanceof ApiError && cause.status === 404) { forgetMigrationJob(); notifyInfo(t('connectionTools.migrationLost')); return }
    scheduleMigrationPoll(1500)
  }
}
function forgetMigrationJob() {
  if (migrationPollTimer) clearTimeout(migrationPollTimer)
  migrationJob.value = undefined
  migrating.value = false
  if (import.meta.client) localStorage.removeItem(`dbfock.migration.${props.connection.id}`)
}

watch(sourceConnectionId, async (connectionId) => {
  sourceDatabases.value = []
  selectedDatabases.value = []
  comparison.value = undefined
  migrationResult.value = undefined
  migrationPlan.value = undefined
  if (!connectionId) return
  loadingDatabases.value = true
  try { sourceDatabases.value = await api<DatabaseInfo[]>(`/connections/${connectionId}/databases`) }
  catch (cause: unknown) { notifyError(errorMessage(cause)) }
  finally { loadingDatabases.value = false }
})
watch([selectedDatabases, maxTableSizeMB, strategy, createMissingTables, skipMatchingTables], () => { migrationPlan.value = undefined; migrationResult.value = undefined; selectedTableKeys.value = [] }, { deep: true })
watch(() => props.connection.id, () => { sourceConnectionId.value = ''; comparison.value = undefined; migrationResult.value = undefined })
onMounted(() => { if (localStorage.getItem(`dbfock.migration.${props.connection.id}`)) { migrating.value = true; void pollMigration() } })
onBeforeUnmount(() => { if (migrationPollTimer) clearTimeout(migrationPollTimer) })
</script>

<template>
  <div class="mt-5 grid max-w-4xl gap-5">
    <section class="rounded-lg border border-line bg-panel p-4">
      <div class="flex items-start gap-3"><span class="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-accent/10 text-accent"><Icon name="lucide:scan-search" class="h-4 w-4" aria-hidden="true" /></span><div><h3 class="text-sm font-semibold">{{ t('connectionTools.assessmentTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('connectionTools.assessmentDescription', { target: connection.name }) }}</p></div></div>
      <div class="mt-4 grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto]"><label class="grid gap-1.5 text-sm font-medium">{{ t('connectionTools.sourceConnection') }}<AppSelect v-model="sourceConnectionId" :options="connectionOptions" :disabled="comparing || migrating" :placeholder="t('connectionTools.chooseConnection')" /></label><button type="button" class="self-end rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="!sourceConnectionId || comparing || migrating" @click="compareSchemas">{{ comparing ? t('connectionTools.comparing') : t('connectionTools.compare') }}</button></div>
      <div v-if="comparison" class="mt-5">
        <div class="grid gap-2 sm:grid-cols-3"><div class="rounded-md bg-canvas p-3"><span class="block text-xs text-muted">{{ t('connectionTools.tablesCompared') }}</span><strong class="mt-1 block text-lg">{{ comparison.summary.tablesCompared }}</strong></div><div class="rounded-md bg-emerald-500/10 p-3"><span class="block text-xs text-muted">{{ t('connectionTools.equalTables') }}</span><strong class="mt-1 block text-lg text-emerald-700 dark:text-emerald-300">{{ comparison.summary.equalTables }}</strong></div><div class="rounded-md bg-amber-500/10 p-3"><span class="block text-xs text-muted">{{ t('connectionTools.divergences') }}</span><strong class="mt-1 block text-lg text-amber-700 dark:text-amber-300">{{ comparison.summary.differentTables }}</strong></div></div>
        <p v-if="!comparison.databases.length" class="mt-4 rounded-md border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-sm text-emerald-700 dark:text-emerald-300">{{ t('connectionTools.schemasEqual') }}</p>
        <details v-else class="mt-4 overflow-hidden rounded-md border border-line"><summary class="cursor-pointer bg-canvas px-3 py-2 text-sm font-semibold">{{ t('connectionTools.showDivergences', { count: comparison.summary.differentTables }) }}</summary><div class="grid gap-3 p-3"><details v-for="database in comparison.databases" :key="database.name" class="overflow-hidden rounded-md border border-line"><summary class="cursor-pointer bg-canvas px-3 py-2 text-sm font-semibold">{{ database.name }} <span class="ml-2 text-xs font-normal text-muted">{{ statusLabel(database.status) }}</span></summary><div class="divide-y divide-line"><details v-for="table in database.tables" :key="table.name" class="px-3 py-2"><summary class="cursor-pointer text-sm"><Icon name="lucide:table-2" class="mr-1 inline h-3.5 w-3.5 text-muted" />{{ table.name }} <span class="ml-2 text-xs text-muted">{{ statusLabel(table.status) }}</span></summary><div v-if="table.columns.length" class="scrollbar mt-2 overflow-x-auto"><table class="min-w-full text-left text-xs"><thead class="text-muted"><tr><th class="py-1 pr-3 font-medium">{{ t('table.column') }}</th><th class="py-1 pr-3 font-medium">{{ selectedSource?.name }}</th><th class="py-1 font-medium">{{ connection.name }}</th></tr></thead><tbody><tr v-for="column in table.columns" :key="column.name" class="border-t border-line"><td class="py-1.5 pr-3 font-medium">{{ column.name }}</td><td class="py-1.5 pr-3 font-mono">{{ columnSide(column, 'source') }}</td><td class="py-1.5 font-mono">{{ columnSide(column, 'target') }}</td></tr></tbody></table></div></details></div></details></div></details>
      </div>
    </section>

    <section class="rounded-lg border border-line bg-panel p-4">
      <div class="flex items-start gap-3"><span class="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-accent/10 text-accent"><Icon name="lucide:arrow-right-left" class="h-4 w-4" aria-hidden="true" /></span><div><h3 class="text-sm font-semibold">{{ t('connectionTools.migrationTitle') }}</h3><p class="mt-1 text-sm text-muted">{{ t('connectionTools.migrationDescription', { target: connection.name }) }}</p></div></div>
      <div class="mt-4 grid gap-4 sm:grid-cols-2"><label class="grid gap-1.5 text-sm font-medium">{{ t('connectionTools.databases') }}<AppMultiSelect v-model="selectedDatabases" :options="databaseOptions" :disabled="!sourceConnectionId || loadingDatabases || migrating" :placeholder="loadingDatabases ? t('tree.loadingDatabases') : t('connectionTools.chooseDatabases')" /></label><label class="grid gap-1.5 text-sm font-medium">{{ t('connectionTools.maxTableSize') }}<div class="flex"><input v-model.number="maxTableSizeMB" class="min-w-0 flex-1 rounded-l-md border border-line bg-canvas px-3 py-2 text-sm outline-none focus:border-accent" type="number" min="0.01" step="0.01" :disabled="migrating"><span class="grid place-items-center rounded-r-md border border-l-0 border-line bg-canvas px-3 text-sm text-muted">MB</span></div></label></div>
      <fieldset class="mt-4" :disabled="migrating"><legend class="text-sm font-medium">{{ t('connectionTools.strategy') }}</legend><div class="mt-2 grid gap-2 sm:grid-cols-2"><label class="flex cursor-pointer gap-3 rounded-md border border-line p-3" :class="strategy === 'drop_recreate' ? 'border-accent bg-accent/5' : ''"><input v-model="strategy" class="mt-0.5" type="radio" value="drop_recreate"><span><span class="block text-sm font-medium">{{ t('connectionTools.dropRecreate') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('connectionTools.dropRecreateDescription') }}</span></span></label><label class="flex cursor-pointer gap-3 rounded-md border border-line p-3" :class="strategy === 'truncate_insert' ? 'border-accent bg-accent/5' : ''"><input v-model="strategy" class="mt-0.5" type="radio" value="truncate_insert"><span><span class="block text-sm font-medium">{{ t('connectionTools.truncateInsert') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('connectionTools.truncateInsertDescription') }}</span></span></label></div></fieldset>
      <label class="mt-4 flex items-start gap-3 text-sm" :class="strategy === 'drop_recreate' ? 'opacity-60' : ''"><input v-model="createMissingTables" class="mt-0.5" type="checkbox" :disabled="migrating || strategy === 'drop_recreate'"><span><span class="font-medium">{{ t('connectionTools.createMissingTables') }}</span><span class="mt-0.5 block text-xs text-muted">{{ strategy === 'drop_recreate' ? t('connectionTools.createMissingTablesDropHint') : t('connectionTools.createMissingTablesDescription') }}</span></span></label>
      <label class="mt-3 flex items-start gap-3 text-sm"><input v-model="skipMatchingTables" class="mt-0.5" type="checkbox" :disabled="migrating"><span><span class="font-medium">{{ t('connectionTools.skipMatchingTables') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('connectionTools.skipMatchingTablesDescription') }}</span></span></label>
      <label class="mt-3 flex items-start gap-3 text-sm"><input v-model="createSkippedTableStructures" class="mt-0.5" type="checkbox" :disabled="migrating"><span><span class="font-medium">{{ t('connectionTools.createSkippedTableStructures') }}</span><span class="mt-0.5 block text-xs text-muted">{{ t('connectionTools.createSkippedTableStructuresDescription') }}</span></span></label>
      <div class="mt-4 rounded-md bg-canvas px-3 py-2 text-xs text-muted"><Icon name="lucide:gauge" class="mr-1 inline h-3.5 w-3.5" />{{ t('connectionTools.efficiencyNotice') }}</div>
      <button type="button" class="mt-4 rounded-md border border-line px-3 py-2 text-sm font-medium hover:bg-canvas disabled:opacity-50" :disabled="!canMigrate || previewing || migrating" @click="previewMigration">{{ previewing ? t('connectionTools.preparingReport') : t('connectionTools.prepareReport') }}</button>
      <div v-if="migrationJob?.status === 'running'" class="mt-4 rounded-md border border-accent/30 bg-accent/5 p-3"><div class="flex items-center justify-between gap-3 text-sm"><span class="font-medium">{{ t('connectionTools.backgroundMigration') }}</span><span>{{ migrationPercent }}%</span></div><div class="mt-2 h-2 overflow-hidden rounded-full bg-line"><div class="h-full rounded-full bg-accent transition-[width] duration-300" :style="{ width: `${migrationPercent}%` }" /></div><p class="mt-2 text-xs text-muted">{{ t('connectionTools.progressSummary', { completed: migrationJob.progress.completedTables, total: migrationJob.progress.totalTables, rows: migrationJob.progress.rowsMigrated }) }}<span v-if="migrationJob.progress.currentTable"> · {{ migrationJob.progress.currentDatabase }}.{{ migrationJob.progress.currentTable }}</span></p><p class="mt-1 text-xs text-muted">{{ t('connectionTools.backgroundNotice') }}</p></div>
      <div v-if="migrationPlan" class="mt-4 rounded-md border border-line bg-canvas p-3 text-sm">
        <h4 class="font-semibold">{{ t('connectionTools.reportTitle') }}</h4>
        <p class="mt-1 text-muted">{{ t('connectionTools.reportSummary', { tables: selectedMigrationTables.length, rows: selectedEstimatedRows, size: formatBytes(selectedSourceSize) }) }}</p>
        <p v-if="structureOnlyMigrationTables.length" class="mt-1 text-muted">{{ t('connectionTools.structureOnlySummary', { count: structureOnlyMigrationTables.length }) }}</p>
        <details class="mt-3 overflow-hidden rounded border border-line"><summary class="cursor-pointer px-3 py-2 font-medium">{{ t('connectionTools.tablesToMigrate', { count: selectedMigrationTables.length }) }}</summary><div class="max-h-64 overflow-auto border-t border-line"><table class="min-w-full text-left text-xs"><thead class="sticky top-0 bg-panel text-muted"><tr><th class="w-9 px-3 py-2"><input type="checkbox" :checked="allTablesSelected" :disabled="migrating || !migrationPlan.tables.length" :aria-label="t('connectionTools.selectAllTables')" :title="t('connectionTools.selectAllTables')" @change="toggleAllTables"></th><th class="px-3 py-2 font-medium">{{ t('table.table') }}</th><th class="px-3 py-2 text-right font-medium">{{ t('connectionTools.estimatedRows') }}</th><th class="px-3 py-2 text-right font-medium">{{ t('connectionTools.sourceSize') }}</th><th class="px-3 py-2 text-right font-medium">{{ t('connectionTools.targetSize') }}</th></tr></thead><tbody><tr v-for="table in migrationPlan.tables" :key="`${table.database}.${table.table}`" class="border-t border-line" :class="selectedTableKeys.includes(tableKey(table.database, table.table)) ? '' : 'opacity-50'"><td class="px-3 py-2"><input v-model="selectedTableKeys" type="checkbox" :value="tableKey(table.database, table.table)" :disabled="migrating" :aria-label="t('connectionTools.selectNamedTable', { table: `${table.database}.${table.table}` })"></td><td class="px-3 py-2 font-mono">{{ table.database }}.{{ table.table }}</td><td class="px-3 py-2 text-right">{{ table.estimatedRows }}</td><td class="px-3 py-2 text-right">{{ formatBytes(table.sizeBytes) }}</td><td class="px-3 py-2 text-right">{{ table.targetSizeBytes == null ? '—' : formatBytes(table.targetSizeBytes) }}</td></tr></tbody></table></div></details>
        <details v-if="migrationPlan.skippedTables.length" class="mt-2"><summary class="cursor-pointer text-amber-700 dark:text-amber-300">{{ t('connectionTools.skippedTables', { count: migrationPlan.skippedTables.length }) }}</summary><ul class="mt-2 grid gap-1 text-xs text-muted"><li v-for="table in migrationPlan.skippedTables" :key="`${table.database}.${table.table}`">{{ table.database }}.{{ table.table }} · {{ formatBytes(table.sizeBytes) }}</li></ul></details>
        <button type="button" class="mt-4 rounded-md bg-accent px-3 py-2 text-sm font-medium text-white disabled:opacity-50" :disabled="(!selectedMigrationTables.length && !structureOnlyMigrationTables.length) || migrating" @click="showMigrationConfirmation = true">{{ migrating ? t('connectionTools.migrating') : t('connectionTools.migrate') }}</button>
      </div>
      <div v-if="migrationResult" class="mt-4 rounded-md border border-line bg-canvas p-3 text-sm"><p>{{ t('connectionTools.migrationSummary', { databases: migrationResult.databasesMigrated, tables: migrationResult.tablesMigrated, rows: migrationResult.rowsMigrated }) }}</p><details v-if="migrationResult.failedTables.length" open class="mt-2"><summary class="cursor-pointer font-medium text-rose-600">{{ t('connectionTools.failedTables', { count: migrationResult.failedTables.length }) }}</summary><ul class="mt-2 grid gap-2 text-xs"><li v-for="table in migrationResult.failedTables" :key="`${table.database}.${table.table}`" class="rounded border border-rose-500/20 bg-rose-500/5 p-2"><span class="font-mono font-medium">{{ table.database }}.{{ table.table }}</span><span class="mt-1 block break-words text-muted">{{ table.error }}</span></li></ul></details><details v-if="migrationResult.skippedTables.length" class="mt-2"><summary class="cursor-pointer text-amber-700 dark:text-amber-300">{{ t('connectionTools.skippedTables', { count: migrationResult.skippedTables.length }) }}</summary><ul class="mt-2 grid gap-1 text-xs text-muted"><li v-for="table in migrationResult.skippedTables" :key="`${table.database}.${table.table}`">{{ table.database }}.{{ table.table }} · {{ formatBytes(table.sizeBytes) }}</li></ul></details></div>
    </section>
    <AppConfirmDialog v-model="showMigrationConfirmation" :title="t('connectionTools.confirmTitle')" :description="migrationConfirmationDescription" :confirm-label="t('connectionTools.confirm')" :cancel-label="t('common.close')" tone="danger" @confirm="migrate" />
  </div>
</template>
