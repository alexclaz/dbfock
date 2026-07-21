<script setup lang="ts">
import type { QueryColumn, QueryResult } from '~/types/database'
import { queryResultAsCSV, queryResultAsJSON } from '~/utils/queryResult'

const props = withDefaults(defineProps<{ result?: QueryResult; loading?: boolean; loadingMore?: boolean; view?: 'table' | 'json' | 'csv'; editing?: boolean; editable?: boolean; sortColumn?: string; sortDirection?: 'asc' | 'desc' }>(), { view: 'table', editing: false, editable: true })
const emit = defineEmits<{ loadMore: []; save: [result: QueryResult]; cancel: []; startEdit: []; sort: [column: string] }>()
const { t } = useI18n()
const columns = computed(() => props.result?.columns ?? [])
const draft = ref<QueryResult>()
const activeCell = ref<{ row: number; column: string }>()
const jsonDraft = ref('[]')
const jsonError = ref('')
const jsonEditor = ref<HTMLTextAreaElement>()
const columnWidths = reactive<Record<string, number>>({})
const inputRefs = new Map<string, HTMLInputElement>()

const displayResult = computed(() => props.editing ? draft.value ?? props.result : props.result)
const rows = computed(() => displayResult.value?.rows ?? [])
const formattedRows = computed(() => props.view === 'csv' ? queryResultAsCSV(displayResult.value) : queryResultAsJSON(displayResult.value))
const highlightedJSON = computed(() => highlightJSON(formattedRows.value))
const canSave = computed(() => !jsonError.value)

function display(value: unknown) { if (value === null) return 'NULL'; if (typeof value === 'boolean') return value ? 'true' : 'false'; return String(value) }
function cloneResult(result?: QueryResult) { return result ? { ...result, columns: result.columns.map((column) => ({ ...column })), rows: result.rows.map((row) => ({ ...row })) } : undefined }
function cellKey(row: number, column: string) { return `${row}:${column}` }
function columnWidth(column: QueryColumn) { return columnWidths[column.name] ?? 160 }
function inputValue(value: unknown) { return value === null ? '' : display(value) }

watch(() => props.editing, (editing) => {
  activeCell.value = undefined
  jsonError.value = ''
  if (!editing) { draft.value = undefined; return }
  draft.value = cloneResult(props.result)
  jsonDraft.value = queryResultAsJSON(draft.value)
}, { immediate: true })
watch(() => props.result, (result) => { if (!props.editing) draft.value = cloneResult(result) })

function highlightJSON(json: string) {
  const escaped = json.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
  return escaped.replace(/("(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*")(?=\s*:)|("(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*")|\b(true|false)\b|\bnull\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/g, (match, key, string) => {
    const color = key ? 'text-json-key' : string ? 'text-json-string' : match === 'true' || match === 'false' ? 'text-json-boolean' : match === 'null' ? 'text-json-null' : 'text-json-number'
    return `<span class="${color}">${match}</span>`
  })
}
function loadMore(event: Event) {
  if (!props.result?.hasMore || props.loading || props.loadingMore || props.editing) return
  const element = event.currentTarget as HTMLElement
  if (element.scrollHeight - element.scrollTop - element.clientHeight <= 80) emit('loadMore')
}
function parseValue(value: string, previous: unknown) {
  if (value === '' && previous === null) return null
  if (value.trim().toUpperCase() === 'NULL') return null
  if (typeof previous === 'boolean' && /^(true|false)$/i.test(value)) return value.toLowerCase() === 'true'
  if (typeof previous === 'number' && value.trim() !== '' && Number.isFinite(Number(value))) return Number(value)
  return value
}
async function editCell(row: number, column: string) {
  if (props.view !== 'table' || !props.editable) return
  if (!props.editing) emit('startEdit')
  await nextTick()
  activeCell.value = { row, column }
  await nextTick()
  inputRefs.get(cellKey(row, column))?.focus()
}
async function editJSON() {
  if (props.view !== 'json' || props.editing || !props.editable) return
  emit('startEdit')
  await nextTick()
  jsonEditor.value?.focus()
}
function updateCell(rowIndex: number, column: string, value: string) {
  const row = draft.value?.rows[rowIndex]
  if (!row) return
  row[column] = parseValue(value, row[column])
}
function finishCell() { activeCell.value = undefined }
function resetCell() { activeCell.value = undefined }
function updateJSON(value: string) {
  jsonDraft.value = value
  try {
    const parsed = JSON.parse(value)
    if (!Array.isArray(parsed)) throw new Error(t('grid.jsonArrayRequired'))
    if (draft.value) { draft.value.rows = parsed; draft.value.rowCount = parsed.length }
    jsonError.value = ''
  } catch (error: any) { jsonError.value = error.message || t('grid.invalidJson') }
}
function save() {
  if (!draft.value || !canSave.value) return false
  emit('save', cloneResult(draft.value)!)
  return true
}
function cancel() { emit('cancel') }
function resizeColumn(event: PointerEvent, column: QueryColumn) {
  event.preventDefault()
  event.stopPropagation()
  const handle = event.currentTarget as HTMLElement
  handle.setPointerCapture(event.pointerId)
  const startX = event.clientX
  const startWidth = columnWidth(column)
  document.body.classList.add('cursor-col-resize', 'select-none')
  const move = (next: PointerEvent) => { columnWidths[column.name] = Math.min(800, Math.max(36, startWidth + next.clientX - startX)) }
  const stop = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', stop)
    handle.releasePointerCapture(event.pointerId)
    document.body.classList.remove('cursor-col-resize', 'select-none')
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', stop)
}

defineExpose({ save, cancel, canSave })
</script>

<template>
  <div class="scrollbar h-full overflow-auto" @scroll="loadMore">
    <div v-if="loading" class="p-5 text-sm text-muted">{{ t('grid.loading') }}</div>
    <div v-else-if="!result" class="grid h-full place-items-center p-8 text-center text-sm text-muted">{{ t('grid.empty') }}</div>
    <table v-else-if="view === 'table'" class="table-fixed border-collapse text-left text-sm">
      <colgroup><col class="w-12"><col v-for="column in columns" :key="column.name" :style="{ width: `${columnWidth(column)}px` }"></colgroup>
      <thead class="sticky top-0 bg-panel text-xs text-muted"><tr><th class="w-12 border-b border-r border-line px-3 py-2 font-medium">#</th><th v-for="column in columns" :key="column.name" class="relative border-b border-r border-line px-3 py-2 font-medium" :style="{ width: `${columnWidth(column)}px`, maxWidth: `${columnWidth(column)}px` }"><button type="button" class="flex w-full items-center gap-1 text-left" :title="t('grid.sortColumn')" @click="$emit('sort', column.name)"><span class="truncate">{{ column.name }}</span><Icon v-if="sortColumn === column.name" :name="sortDirection === 'desc' ? 'lucide:arrow-down' : 'lucide:arrow-up'" class="h-3 w-3 shrink-0 text-accent" aria-hidden="true" /></button><small class="block truncate font-normal opacity-70">{{ column.databaseType }}</small><span class="absolute inset-y-0 -right-1 z-10 w-2 cursor-col-resize select-none hover:bg-accent/70 active:bg-accent" :title="t('grid.resizeColumn')" @pointerdown="resizeColumn($event, column)" /></th></tr></thead>
      <tbody><tr v-for="(row,index) in rows" :key="index" class="hover:bg-accent/5"><td class="border-b border-r border-line px-3 py-2 text-xs text-muted">{{ index + 1 }}</td><td v-for="column in columns" :key="column.name" class="overflow-hidden border-b border-r border-line px-3 py-2" :class="row[column.name] === null ? 'italic text-muted' : ''" :style="{ width: `${columnWidth(column)}px`, maxWidth: `${columnWidth(column)}px` }" :title="display(row[column.name])" @dblclick="editCell(index, column.name)"><input v-if="editing && activeCell?.row === index && activeCell.column === column.name" :ref="(element) => { if (element) inputRefs.set(cellKey(index, column.name), element as HTMLInputElement) }" class="-my-1 w-full rounded border border-accent bg-canvas px-1 py-1 text-sm text-ink outline-none" :value="inputValue(row[column.name])" @input="updateCell(index, column.name, ($event.target as HTMLInputElement).value)" @blur="finishCell" @keydown.enter.prevent="finishCell" @keydown.esc.prevent="resetCell"><span v-else class="block truncate">{{ display(row[column.name]) }}</span></td></tr><tr v-if="loadingMore"><td :colspan="columns.length + 1" class="p-3 text-center text-xs text-muted">{{ t('grid.loading') }}</td></tr><tr v-if="!rows.length"><td :colspan="columns.length + 1" class="p-8 text-center text-muted">{{ t('grid.noRows') }}</td></tr></tbody>
    </table>
    <div v-else-if="editing && view === 'json'" class="min-h-full bg-canvas p-4"><textarea ref="jsonEditor" class="min-h-[18rem] w-full resize-y rounded-md border border-line bg-panel p-3 font-mono text-sm leading-6 text-ink outline-none focus:border-accent" spellcheck="false" :value="jsonDraft" @input="updateJSON(($event.target as HTMLTextAreaElement).value)" /><p v-if="jsonError" class="mt-2 text-xs text-rose-500">{{ t('grid.invalidJson') }}: {{ jsonError }}</p></div>
    <div v-else-if="view === 'json'" class="min-h-full bg-canvas" @dblclick="editJSON"><pre class="min-h-full cursor-text whitespace-pre-wrap break-words p-4 font-mono text-sm text-ink" v-html="highlightedJSON" /></div>
    <pre v-else class="min-h-full bg-canvas whitespace-pre-wrap break-words p-4 font-mono text-sm text-ink">{{ formattedRows }}</pre>
  </div>
</template>
