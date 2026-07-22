<script setup lang="ts">
import type { QueryResult, SchemaDiagram, TableStructure } from '~/types/database'
import { queryResultAsCSV, queryResultAsJSON, queryResultAsTSV, queryResultEdits } from '~/utils/queryResult'

type TableSection = 'data' | 'structure' | 'constraints' | 'foreignKeys' | 'references' | 'triggers' | 'indexes' | 'ddl' | 'diagram'

const props = defineProps<{ connectionId: string; database: string; table: string; activeSection?: TableSection }>()
const emit = defineEmits<{ 'update:activeSection': [value: TableSection]; transactionStatus: [connectionId: string, pending: boolean, pendingStatements: number]; 'open-table': [table: string] }>()
const api = useApi()
const { t } = useI18n()
const { error: notifyError } = useToast()
const result = ref<QueryResult>()
const structure = ref<TableStructure>()
const loading = ref(true)
const page = ref(0)
const pageSize = ref(100)
const sortColumn = ref<string>()
const sortDirection = ref<'asc' | 'desc'>()
const tableSections: TableSection[] = ['data', 'structure', 'constraints', 'foreignKeys', 'references', 'triggers', 'indexes', 'diagram', 'ddl']
const section = ref<TableSection>(tableSections.includes(props.activeSection as TableSection) ? props.activeSection! : 'data')
const error = ref('')
const dataEditing = ref(false)
const dataView = ref<'table' | 'json' | 'csv'>('table')
const dataCopied = ref(false)
const dataGrid = ref<{ save: () => boolean; cancel: () => void; canSave: boolean }>()
const ddlCopied = ref(false)
const ddlEl = ref<HTMLElement>()

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
  else { sortColumn.value = undefined; sortDirection.value = undefined }
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
  else if (next !== 'data' && next !== 'diagram' && !structure.value) void loadStructure()
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

watch(() => [props.connectionId, props.database, props.table], loadData, { immediate: true })
watch(error, (message) => { if (message) { notifyError(message); error.value = '' } })
</script>

<template>
  <section class="flex h-full min-h-0 flex-col">
    <div class="flex items-center gap-4 border-b border-line px-4 py-2">
      <div class="shrink-0"><h2 class="text-sm font-semibold"><span class="text-xs font-normal text-muted">{{ database }}</span><span class="text-muted">.</span>{{ table }}</h2></div>
      <div class="scrollbar min-w-0 overflow-x-auto"><div class="flex w-max rounded-md border border-line p-0.5 text-xs">
        <button class="rounded px-2.5 py-1" :class="section === 'data' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('data')">{{ t('table.data') }}</button><button class="rounded px-2.5 py-1" :class="section === 'structure' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('structure')">{{ t('table.structure') }}</button><button class="rounded px-2.5 py-1" :class="section === 'constraints' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('constraints')">{{ t('table.constraints') }}</button><button class="rounded px-2.5 py-1" :class="section === 'foreignKeys' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('foreignKeys')">{{ t('table.foreignKeys') }}</button><button class="rounded px-2.5 py-1" :class="section === 'references' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('references')">{{ t('table.references') }}</button><button class="rounded px-2.5 py-1" :class="section === 'triggers' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('triggers')">{{ t('table.triggers') }}</button><button class="rounded px-2.5 py-1" :class="section === 'indexes' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('indexes')">{{ t('table.indexes') }}</button><button class="rounded px-2.5 py-1" :class="section === 'diagram' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('diagram')">{{ t('table.diagram') }}</button><button class="rounded px-2.5 py-1" :class="section === 'ddl' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('ddl')">DDL</button>
      </div></div>
    </div>
    <template v-if="section === 'data'"><div class="flex items-center justify-between border-b border-line px-4 py-2 text-xs text-muted"><span>{{ t('query.rows', { count: result?.rowCount || 0 }) }}{{ result?.hasMore ? '+' : '' }}</span><div class="flex items-center gap-2"><AppSelect v-model="pageSize" :disabled="loading || dataEditing" class="w-24" :options="[{ value: 50, label: '50' }, { value: 100, label: '100' }, { value: 250, label: '250' }]" @change="page = 0; loadData()" /><button :disabled="page === 0 || loading || dataEditing" @click="previousPage">{{ t('table.previous') }}</button><button :disabled="!result?.hasMore || loading || dataEditing" @click="nextPage">{{ t('table.next') }}</button><button class="grid rounded p-1 hover:bg-canvas disabled:opacity-60" :aria-label="t('stats.refresh')" :disabled="loading || dataEditing" @click="loadData"><Icon name="lucide:refresh-cw" class="h-4 w-4" aria-hidden="true" /></button><button type="button" class="grid rounded p-1 hover:bg-canvas disabled:opacity-60" :title="dataCopied ? t('grid.copied') : t('grid.copy')" :aria-label="dataCopied ? t('grid.copied') : t('grid.copy')" :disabled="!result" @click="copyData"><Icon :name="dataCopied ? 'lucide:check' : 'lucide:copy'" class="h-4 w-4" aria-hidden="true" /></button><div class="flex rounded-md border border-line p-0.5"><button type="button" class="rounded px-2.5 py-1" :class="dataView === 'table' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="dataView === 'table'" @click="dataView = 'table'">{{ t('grid.table') }}</button><button type="button" class="rounded px-2.5 py-1" :class="dataView === 'json' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="dataView === 'json'" @click="dataView = 'json'">JSON</button><button type="button" class="rounded px-2.5 py-1" :class="dataView === 'csv' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="dataView === 'csv'" @click="dataView = 'csv'">CSV</button></div><template v-if="dataEditing"><button type="button" class="rounded-md bg-accent px-2.5 py-1 font-medium text-white disabled:cursor-not-allowed disabled:opacity-50" :disabled="!dataGrid?.canSave" @click="dataGrid?.save()">{{ t('grid.save') }}</button><button type="button" class="rounded-md border border-line px-2.5 py-1 text-ink" @click="dataGrid?.cancel()">{{ t('grid.cancel') }}</button></template></div></div><div class="min-h-0 flex-1"><DataGrid ref="dataGrid" :result="result" :loading="loading" :view="dataView" :editing="dataEditing" :sort-column="sortColumn" :sort-direction="sortDirection" @start-edit="dataEditing = true" @save="saveDataEdits" @cancel="dataEditing = false" @sort="toggleSort" /></div></template>
    <div v-else-if="section === 'structure'" class="scrollbar overflow-auto p-4"><table class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.column') }}</th><th class="border-b border-line p-2">{{ t('table.type') }}</th><th class="border-b border-line p-2">{{ t('table.nullable') }}</th><th class="border-b border-line p-2">{{ t('table.key') }}</th><th class="border-b border-line p-2">{{ t('table.default') }}</th></tr></thead><tbody><tr v-for="column in structure?.columns" :key="column.name"><td class="border-b border-line p-2 font-medium">{{ column.name }}</td><td class="border-b border-line p-2 text-muted">{{ column.columnType }}</td><td class="border-b border-line p-2">{{ column.nullable ? t('table.yes') : t('table.no') }}</td><td class="border-b border-line p-2">{{ column.key || '—' }}</td><td class="border-b border-line p-2">{{ column.default || '—' }}</td></tr></tbody></table></div>
    <div v-else-if="section === 'constraints'" class="scrollbar overflow-auto p-4"><table v-if="constraints.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.type') }}</th><th class="border-b border-line p-2">{{ t('table.columns') }}</th></tr></thead><tbody><tr v-for="constraint in constraints" :key="constraint.name"><td class="border-b border-line p-2 font-medium">{{ constraint.name }}</td><td class="border-b border-line p-2">{{ constraint.type }}</td><td class="border-b border-line p-2 text-muted">{{ constraint.columns?.join(', ') || '—' }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'foreignKeys'" class="scrollbar overflow-auto p-4"><table v-if="foreignKeys.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.column') }}</th><th class="border-b border-line p-2">{{ t('table.references') }}</th></tr></thead><tbody><tr v-for="foreignKey in foreignKeys" :key="`${foreignKey.name}:${foreignKey.column}`"><td class="border-b border-line p-2 font-medium">{{ foreignKey.name }}</td><td class="border-b border-line p-2">{{ foreignKey.column }}</td><td class="border-b border-line p-2 text-muted">{{ foreignKey.referencedTable }}.{{ foreignKey.referencedColumn }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'references'" class="scrollbar overflow-auto p-4"><table v-if="references.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.table') }}</th><th class="border-b border-line p-2">{{ t('table.column') }}</th><th class="border-b border-line p-2">{{ t('table.references') }}</th></tr></thead><tbody><tr v-for="reference in references" :key="`${reference.database}:${reference.table}:${reference.name}:${reference.column}`"><td class="border-b border-line p-2 font-medium">{{ reference.name }}</td><td class="border-b border-line p-2">{{ reference.database }}.{{ reference.table }}</td><td class="border-b border-line p-2">{{ reference.column }}</td><td class="border-b border-line p-2 text-muted">{{ table }}.{{ reference.referencedColumn }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'triggers'" class="scrollbar space-y-3 overflow-auto p-4"><article v-for="trigger in triggers" :key="trigger.name" class="rounded-md border border-line"><div class="flex items-center gap-2 border-b border-line px-3 py-2 text-sm"><span class="font-medium">{{ trigger.name }}</span><span class="text-xs text-muted">{{ trigger.timing }} {{ trigger.event }}</span></div><pre class="overflow-auto whitespace-pre-wrap p-3 font-mono text-xs">{{ trigger.statement }}</pre></article><p v-if="!triggers.length" class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'indexes'" class="scrollbar overflow-auto p-4"><table v-if="indexes.length" class="min-w-full text-left text-sm"><thead class="text-xs text-muted"><tr><th class="border-b border-line p-2">{{ t('table.name') }}</th><th class="border-b border-line p-2">{{ t('table.unique') }}</th><th class="border-b border-line p-2">{{ t('table.columns') }}</th></tr></thead><tbody><tr v-for="index in indexes" :key="index.name"><td class="border-b border-line p-2 font-medium">{{ index.name }}</td><td class="border-b border-line p-2">{{ index.unique ? t('table.yes') : t('table.no') }}</td><td class="border-b border-line p-2 text-muted">{{ index.columns?.join(', ') || '—' }}</td></tr></tbody></table><p v-else class="text-sm text-muted">{{ t('table.empty') }}</p></div>
    <div v-else-if="section === 'diagram'" class="min-h-0 flex-1"><ErDiagram :tables="diagramTables" :focus-table="table" :loading="diagramLoading" @open-table="emit('open-table', $event)" /></div>
    <div v-else class="flex min-h-0 flex-1 flex-col"><div class="flex items-center justify-end border-b border-line px-4 py-2"><button type="button" class="grid rounded p-1 hover:bg-canvas disabled:opacity-60" :title="ddlCopied ? t('grid.copied') : t('grid.copy')" :aria-label="ddlCopied ? t('grid.copied') : t('grid.copy')" :disabled="!structure?.ddl" @click="copyDDL"><Icon :name="ddlCopied ? 'lucide:check' : 'lucide:copy'" class="h-4 w-4" aria-hidden="true" /></button></div><pre ref="ddlEl" tabindex="0" class="scrollbar flex-1 overflow-auto whitespace-pre-wrap p-4 font-mono text-xs outline-none" @keydown="selectAllDdl">{{ structure?.ddl || t('table.loadingDdl') }}</pre></div>
  </section>
</template>
