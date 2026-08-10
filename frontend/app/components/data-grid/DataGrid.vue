<script setup lang="ts">
import type { QueryColumn, QueryResult } from '~/types/database'
import { queryResultAsCSV, queryResultAsJSON } from '~/utils/queryResult'

const props = withDefaults(defineProps<{ result?: QueryResult; loading?: boolean; loadingMore?: boolean; view?: 'table' | 'json' | 'csv'; editing?: boolean; editable?: boolean; editableColumns?: string[]; jsonEditable?: boolean; sortable?: boolean; sortColumn?: string; sortDirection?: 'asc' | 'desc' }>(), { view: 'table', editing: false, editable: true, jsonEditable: true, sortable: false })
const emit = defineEmits<{ loadMore: []; save: [result: QueryResult]; cancel: []; startEdit: []; sort: [column: string, direction: 'asc' | 'desc'] }>()
const { t } = useI18n()
const columns = computed(() => props.result?.columns ?? [])
const draft = ref<QueryResult>()
const activeCell = ref<{ row: number; column: string }>()
const jsonDraft = ref('[]')
const jsonError = ref('')
const jsonEditor = ref<HTMLTextAreaElement>()
const columnWidths = reactive<Record<string, number>>({})
const inputRefs = new Map<string, HTMLInputElement>()
const copiedColumn = ref<string>()
const sortMenuColumn = ref<string>()

const displayResult = computed(() => props.editing ? draft.value ?? props.result : props.result)
const rows = computed(() => displayResult.value?.rows ?? [])
const formattedRows = computed(() => props.view === 'csv' ? queryResultAsCSV(displayResult.value) : queryResultAsJSON(displayResult.value))
const highlightedJSON = computed(() => highlightJSON(formattedRows.value))
const highlightedCSV = computed(() => highlightCSV(formattedRows.value))
const isDirty = computed(() => Boolean(draft.value) && JSON.stringify(draft.value!.rows) !== JSON.stringify(props.result?.rows))
const canSave = computed(() => !jsonError.value && isDirty.value)
const canSort = computed(() => props.sortable)

function display(value: unknown) { if (value === null) return 'NULL'; if (typeof value === 'boolean') return value ? 'true' : 'false'; return String(value) }
function cloneResult(result?: QueryResult) { return result ? { ...result, columns: result.columns.map((column) => ({ ...column })), rows: result.rows.map((row) => ({ ...row })) } : undefined }
function cellKey(row: number, column: string) { return `${row}:${column}` }
function columnWidth(column: QueryColumn) { return columnWidths[column.name] ?? 160 }
function inputValue(value: unknown) { return value === null ? '' : display(value) }
async function copyColumnName(name: string) {
  if (!navigator.clipboard) return
  try {
    await navigator.clipboard.writeText(name)
    copiedColumn.value = name
    window.setTimeout(() => { if (copiedColumn.value === name) copiedColumn.value = undefined }, 1500)
  } catch { /* Clipboard access can be denied by the browser. */ }
}
function toggleSortMenu(column: string) { sortMenuColumn.value = sortMenuColumn.value === column ? undefined : column }
function selectSort(column: string, direction: 'asc' | 'desc') { emit('sort', column, direction); sortMenuColumn.value = undefined }

watch(() => props.editing, (editing) => {
  activeCell.value = undefined
  jsonError.value = ''
  if (!editing) { draft.value = undefined; return }
  draft.value = cloneResult(props.result)
  jsonDraft.value = queryResultAsJSON(draft.value)
}, { immediate: true })
watch(() => props.result, (result) => { if (!props.editing) draft.value = cloneResult(result) })

function highlightJSON(json: string) {
  const escaped = escapeHTML(json)
  return escaped.replace(/("(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*")(?=\s*:)|("(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*")|\b(true|false)\b|\bnull\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/g, (match, key, string) => {
    const color = key ? 'text-json-key' : string ? 'text-json-string' : match === 'true' || match === 'false' ? 'text-json-boolean' : match === 'null' ? 'text-json-null' : 'text-json-number'
    return `<span class="${color}">${match}</span>`
  })
}
function escapeHTML(value: string) { return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;') }
function csvCellColor(value: string, isHeader: boolean) {
  if (isHeader) return 'font-semibold text-accent'
  if (!value) return 'text-muted'
  if (/^"(?:[^"\r\n]|"")*"$/.test(value)) return 'text-json-string'
  if (/^(true|false)$/i.test(value)) return 'text-json-boolean'
  if (/^-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?$/.test(value)) return 'text-json-number'
  return 'text-ink'
}
function highlightCSV(csv: string) {
  let html = ''
  let cellStart = 0
  let row = 0
  let inQuotes = false
  const addCell = (end: number) => { const cell = csv.slice(cellStart, end); html += `<span class="${csvCellColor(cell, row === 0)}">${escapeHTML(cell)}</span>` }
  for (let index = 0; index < csv.length; index += 1) {
    const character = csv[index]
    if (character === '"') {
      if (inQuotes && csv[index + 1] === '"') { index += 1; continue }
      inQuotes = !inQuotes
    }
    if (inQuotes) continue
    if (character === ',') { addCell(index); html += '<span class="text-muted/70">,</span>'; cellStart = index + 1; continue }
    if (character === '\r' || character === '\n') {
      addCell(index)
      const newlineLength = character === '\r' && csv[index + 1] === '\n' ? 2 : 1
      html += csv.slice(index, index + newlineLength)
      cellStart = index + newlineLength
      row += 1
      if (newlineLength === 2) index += 1
    }
  }
  addCell(csv.length)
  return html
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
  if (props.view !== 'table' || !props.editable || (props.editableColumns && !props.editableColumns.includes(column))) return
  if (!props.editing) emit('startEdit')
  await nextTick()
  activeCell.value = { row, column }
  await nextTick()
  inputRefs.get(cellKey(row, column))?.focus()
}
async function editJSON() {
  if (props.view !== 'json' || props.editing || !props.editable || !props.jsonEditable) return
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
  <div class="scrollbar h-full overflow-auto" @scroll="loadMore" @click="sortMenuColumn = undefined">
    <div v-if="loading" class="p-5 text-sm text-muted">{{ t('grid.loading') }}</div>
    <div v-else-if="!result" class="grid h-full place-items-center p-8 text-center text-sm text-muted">{{ t('grid.empty') }}</div>
    <table v-else-if="view === 'table'" class="table-fixed border-collapse text-left text-xs">
      <colgroup><col class="w-10"><col v-for="column in columns" :key="column.name" :style="{ width: `${columnWidth(column)}px` }"></colgroup>
      <thead class="sticky top-0 bg-panel text-[11px] text-muted"><tr><th class="w-10 border-b border-r border-line px-2 py-1 font-medium">#</th><th v-for="column in columns" :key="column.name" class="relative border-b border-r border-line px-2 py-1 font-medium" :style="{ width: `${columnWidth(column)}px`, maxWidth: `${columnWidth(column)}px` }"><div class="flex items-center gap-0.5"><span class="min-w-0 flex-1 truncate">{{ column.name }}</span><button type="button" class="grid h-4 w-4 shrink-0 place-items-center rounded hover:bg-canvas hover:text-ink" :title="copiedColumn === column.name ? t('grid.copied') : t('grid.copyColumn')" :aria-label="copiedColumn === column.name ? t('grid.copied') : t('grid.copyColumn')" @click="copyColumnName(column.name)"><Icon :name="copiedColumn === column.name ? 'lucide:check' : 'lucide:copy'" class="h-3 w-3" aria-hidden="true" /></button><button v-if="canSort" type="button" class="grid h-4 w-4 shrink-0 place-items-center rounded hover:bg-canvas hover:text-ink" :class="sortColumn === column.name ? 'text-accent' : ''" :title="t('grid.sortColumn')" :aria-label="t('grid.sortColumn')" :aria-expanded="sortMenuColumn === column.name" @click.stop="toggleSortMenu(column.name)"><Icon :name="sortColumn === column.name ? (sortDirection === 'desc' ? 'lucide:arrow-down' : 'lucide:arrow-up') : 'lucide:arrow-up-down'" class="h-3 w-3" aria-hidden="true" /></button></div><small class="block truncate text-[10px] leading-3 font-normal opacity-70">{{ column.databaseType }}</small><div v-if="sortMenuColumn === column.name" class="absolute right-1 top-full z-20 mt-1 w-28 rounded-md border border-line bg-panel p-0.5 shadow-lg" @click.stop><button type="button" class="flex w-full items-center gap-1.5 rounded px-2 py-1 text-left hover:bg-canvas hover:text-ink" :class="sortColumn === column.name && sortDirection === 'asc' ? 'bg-canvas text-accent' : ''" @click="selectSort(column.name, 'asc')"><Icon name="lucide:arrow-up" class="h-3 w-3" aria-hidden="true" />{{ t('grid.sortAscending') }}</button><button type="button" class="flex w-full items-center gap-1.5 rounded px-2 py-1 text-left hover:bg-canvas hover:text-ink" :class="sortColumn === column.name && sortDirection === 'desc' ? 'bg-canvas text-accent' : ''" @click="selectSort(column.name, 'desc')"><Icon name="lucide:arrow-down" class="h-3 w-3" aria-hidden="true" />{{ t('grid.sortDescending') }}</button></div><span class="absolute inset-y-0 -right-1 z-10 w-2 cursor-col-resize select-none hover:bg-accent/70 active:bg-accent" :title="t('grid.resizeColumn')" @pointerdown="resizeColumn($event, column)" /></th></tr></thead>
      <tbody class="select-text"><tr v-for="(row,index) in rows" :key="index" class="odd:bg-panel/60 hover:bg-accent/10"><td class="border-b border-r border-line px-2 py-1 text-[11px] text-muted">{{ index + 1 }}</td><td v-for="column in columns" :key="column.name" class="overflow-hidden border-b border-r border-line px-2 py-1 leading-4" :class="row[column.name] === null ? 'italic text-muted' : ''" :style="{ width: `${columnWidth(column)}px`, maxWidth: `${columnWidth(column)}px` }" :title="display(row[column.name])" @dblclick="editCell(index, column.name)"><input v-if="editing && activeCell?.row === index && activeCell.column === column.name" :ref="(element) => { if (element) inputRefs.set(cellKey(index, column.name), element as HTMLInputElement) }" class="-my-0.5 w-full rounded border border-accent bg-canvas px-1 py-0.5 text-xs text-ink outline-none" :value="inputValue(row[column.name])" @input="updateCell(index, column.name, ($event.target as HTMLInputElement).value)" @blur="finishCell" @keydown.enter.prevent="finishCell" @keydown.esc.prevent="resetCell"><span v-else class="block truncate">{{ display(row[column.name]) }}</span></td></tr><tr v-if="loadingMore"><td :colspan="columns.length + 1" class="p-3 text-center text-xs text-muted">{{ t('grid.loading') }}</td></tr><tr v-if="!rows.length"><td :colspan="columns.length + 1" class="p-8 text-center text-muted">{{ t('grid.noRows') }}</td></tr></tbody>
    </table>
    <div v-else-if="editing && view === 'json'" class="min-h-full bg-canvas p-2"><textarea ref="jsonEditor" class="min-h-[18rem] w-full resize-y rounded-md border border-line bg-panel p-2 font-mono text-xs leading-4 text-ink outline-none focus:border-accent" spellcheck="false" :value="jsonDraft" @input="updateJSON(($event.target as HTMLTextAreaElement).value)" /><p v-if="jsonError" class="mt-2 text-xs text-rose-500">{{ t('grid.invalidJson') }}: {{ jsonError }}</p></div>
    <div v-else-if="view === 'json'" class="min-h-full bg-canvas" @dblclick="editJSON"><pre class="min-h-full cursor-text whitespace-pre-wrap break-words p-2 font-mono text-xs leading-4 text-ink" v-html="highlightedJSON" /></div>
    <pre v-else class="min-h-full whitespace-pre-wrap break-words bg-canvas p-2 font-mono text-xs leading-4 text-ink" v-html="highlightedCSV" />
  </div>
</template>
