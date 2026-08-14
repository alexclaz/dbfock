<script setup lang="ts">
import type { QueryResult } from '~/types/database'

type ResultTab = {
  id: string
  title: string
  result?: QueryResult
  view: 'table' | 'json' | 'csv'
  copied: boolean
  editing: boolean
  dirty?: boolean
  sources?: { columns: string[]; primaryKey?: string[] }[]
  insertTarget?: { database: string; table: string; columns: string[] }
  sortColumn?: string
  sortDirection?: 'asc' | 'desc'
  refreshable?: boolean
}
type GridMutations = { insertedRows: { index: number; row: Record<string, unknown>; useDefaults: boolean }[]; deletedRows: { index: number; row: Record<string, unknown> }[] }

const props = withDefaults(defineProps<{ resultTabs: ResultTab[]; activeResultTabId?: string; loading?: boolean; loadingMore?: boolean; canCreateTab?: boolean; summary?: string }>(), { canCreateTab: false, summary: '' })
const emit = defineEmits<{ selectTab: [id: string]; closeTab: [id: string]; createTab: []; copy: [id: string]; save: [id: string, result: QueryResult, mutations: GridMutations]; loadMore: []; sort: [id: string, column: string, direction: 'asc' | 'desc']; refresh: [id: string] }>()
const { t } = useI18n()
type ResultGrid = { save: () => boolean; cancel: () => void; addRow: () => Promise<void>; canSave: boolean }
const resultGrids = ref<Record<string, ResultGrid | undefined>>({})
const activeResultTab = computed(() => props.resultTabs.find((tab) => tab.id === props.activeResultTabId))
const resultGrid = computed(() => activeResultTab.value ? resultGrids.value[activeResultTab.value.id] : undefined)
const searches = reactive<Record<string, { open: boolean; value: string }>>({})
const searchInput = ref<HTMLInputElement>()
const pendingClose = ref<ResultTab>()
const activeSearch = computed(() => activeResultTab.value ? searches[activeResultTab.value.id] : undefined)
const searchActive = computed(() => Boolean(activeSearch.value?.open && activeSearch.value.value.trim()))
const canModifyRows = computed(() => activeResultTab.value ? canModifyRowsFor(activeResultTab.value) : false)
async function openSearch() {
  const resultTab = activeResultTab.value
  if (!resultTab) return
  const search = searches[resultTab.id] ?? (searches[resultTab.id] = { open: false, value: '' })
  search.open = true
  await nextTick()
  searchInput.value?.focus()
  searchInput.value?.select()
}
function closeSearch() { if (!activeSearch.value) return; activeSearch.value.open = false; activeSearch.value.value = '' }
function updateSearch(value: string) { if (activeSearch.value) activeSearch.value.value = value }
function searchActiveFor(resultTab: ResultTab) { const search = searches[resultTab.id]; return Boolean(search?.open && search.value.trim()) }
function canModifyRowsFor(resultTab: ResultTab) { return Boolean(resultTab.sources?.length === 1 && resultTab.sources[0]?.primaryKey?.length && resultTab.view === 'table' && !searchActiveFor(resultTab)) }
function displayedResultFor(resultTab: ResultTab) {
  const current = resultTab.result
  if (!current || !searchActiveFor(resultTab)) return current
  const needle = searches[resultTab.id]!.value.trim().toLowerCase()
  const rows = current.rows.filter((row) => current.columns.some((column) => (row[column.name] === null ? 'null' : String(row[column.name])).toLowerCase().includes(needle)))
  return { ...current, rows, rowCount: rows.length, hasMore: false }
}
function setResultGrid(id: string, grid: ResultGrid | null) { resultGrids.value[id] = grid ?? undefined }
function sortResult(resultTab: ResultTab, column: string, direction: 'asc' | 'desc') {
  if (!resultTab?.result || resultTab.editing) return
  emit('sort', resultTab.id, column, direction)
}
function requestClose(resultTab: ResultTab) { if (resultTab.dirty) pendingClose.value = resultTab; else emit('closeTab', resultTab.id) }
function discardAndClose() { const resultTab = pendingClose.value; pendingClose.value = undefined; if (resultTab) emit('closeTab', resultTab.id) }
function saveAndClose() { const resultTab = pendingClose.value; if (!resultTab || !resultGrids.value[resultTab.id]?.save()) return; pendingClose.value = undefined; emit('closeTab', resultTab.id) }
watch(searchActive, (active) => { if (active && activeResultTab.value?.editing) resultGrid.value?.cancel() })
</script>

<template>
  <div v-if="resultTabs.length" class="flex h-9 items-end gap-1 overflow-x-auto border-b border-line bg-panel px-2"><button v-for="resultTab in resultTabs" :key="resultTab.id" type="button" class="group flex h-8 shrink-0 items-center gap-1 rounded-t px-2 text-xs" :class="activeResultTab?.id === resultTab.id ? 'bg-canvas font-medium text-ink' : 'text-muted hover:bg-canvas/60'" @click="emit('selectTab', resultTab.id)"><span>{{ resultTab.title }}</span><span v-if="resultTab.dirty" class="h-1.5 w-1.5 rounded-full bg-amber-500" :title="t('grid.save')" /><span class="rounded p-1 opacity-0 group-hover:opacity-100 hover:bg-line" :aria-label="t('common.close')" @click.stop="requestClose(resultTab)"><Icon name="lucide:x" class="h-3.5 w-3.5" aria-hidden="true" /></span></button><button v-if="canCreateTab" type="button" class="mb-1 grid h-6 w-6 shrink-0 place-items-center rounded text-muted hover:bg-canvas hover:text-ink" :title="t('query.newResultTab')" :aria-label="t('query.newResultTab')" @click="emit('createTab')"><Icon name="lucide:plus" class="h-4 w-4" aria-hidden="true" /></button></div>
  <div class="flex items-center justify-between border-b border-line px-4 py-2 text-xs text-muted"><span>{{ summary || t('query.results') }}</span><div v-if="activeResultTab" class="flex items-center gap-2"><button v-if="canModifyRows" type="button" class="grid rounded p-1 hover:bg-canvas" :title="t('grid.addRow')" :aria-label="t('grid.addRow')" @click="resultGrid?.addRow()"><Icon name="lucide:plus" class="h-4 w-4" aria-hidden="true" /></button><button type="button" class="grid rounded p-1 hover:bg-canvas disabled:opacity-60" :title="activeResultTab.copied ? t('grid.copied') : t('grid.copy')" :aria-label="activeResultTab.copied ? t('grid.copied') : t('grid.copy')" :disabled="!activeResultTab.result" @click="emit('copy', activeResultTab.id)"><Icon :name="activeResultTab.copied ? 'lucide:check' : 'lucide:copy'" class="h-4 w-4" aria-hidden="true" /></button><button type="button" class="grid rounded p-1 hover:bg-canvas disabled:opacity-60" :title="t('grid.refresh')" :aria-label="t('grid.refresh')" :disabled="!activeResultTab.refreshable || activeResultTab.editing || loading || loadingMore" @click="emit('refresh', activeResultTab.id)"><Icon name="lucide:refresh-cw" class="h-4 w-4" aria-hidden="true" /></button><button type="button" class="grid rounded p-1 hover:bg-canvas" :class="activeSearch?.open ? 'bg-canvas text-ink' : ''" :title="t('grid.search')" :aria-label="t('grid.search')" :aria-pressed="activeSearch?.open" @click="activeSearch?.open ? closeSearch() : openSearch()"><Icon name="lucide:search" class="h-4 w-4" aria-hidden="true" /></button><div class="flex rounded-md border border-line p-0.5"><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'table' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'table'" @click="activeResultTab.view = 'table'">{{ t('grid.table') }}</button><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'json' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'json'" @click="activeResultTab.view = 'json'">JSON</button><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'csv' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'csv'" @click="activeResultTab.view = 'csv'">CSV</button></div><template v-if="activeResultTab.editing && resultGrid?.canSave"><button type="button" class="rounded-md bg-accent px-2.5 py-1 font-medium text-white" @click="resultGrid?.save()">{{ t('grid.save') }}</button><button type="button" class="rounded-md border border-line px-2.5 py-1 text-ink" @click="activeResultTab.editing = false">{{ t('grid.cancel') }}</button></template></div></div>
  <div v-if="activeSearch?.open" class="flex items-center gap-2 border-b border-line px-4 py-2 text-xs"><Icon name="lucide:search" class="h-4 w-4 shrink-0 text-muted" aria-hidden="true" /><input ref="searchInput" :value="activeSearch.value" type="search" class="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted" :placeholder="t('grid.searchPlaceholder')" :aria-label="t('grid.search')" @input="updateSearch(($event.target as HTMLInputElement).value)" @keydown.esc.prevent="closeSearch"><span v-if="searchActive" class="shrink-0 text-muted">{{ (displayedResultFor(activeResultTab!)?.rows.length ?? 0) ? t('grid.searchMatches', { count: displayedResultFor(activeResultTab!)?.rows.length ?? 0, total: activeResultTab?.result?.rows.length || 0 }) : t('grid.searchNoMatches') }}</span><button type="button" class="grid shrink-0 rounded p-1 text-muted hover:bg-canvas hover:text-ink" :title="t('grid.searchClose')" :aria-label="t('grid.searchClose')" @click="closeSearch"><Icon name="lucide:x" class="h-4 w-4" aria-hidden="true" /></button></div>
  <div class="min-h-0 flex-1"><DataGrid v-for="resultTab in resultTabs" v-show="resultTab.id === activeResultTab?.id" :key="resultTab.id" :ref="(grid) => setResultGrid(resultTab.id, grid as unknown as ResultGrid | null)" :result="displayedResultFor(resultTab)" :loading="loading" :loading-more="loadingMore" :view="resultTab.view" :editing="resultTab.editing" :editable="canModifyRowsFor(resultTab)" :editable-columns="resultTab.sources?.flatMap((source) => source.columns)" :primary-key="resultTab.sources?.[0]?.primaryKey" :insert-target="resultTab.insertTarget" :json-editable="!resultTab.sources?.length" :sortable="!resultTab.editing" :sort-column="resultTab.sortColumn" :sort-direction="resultTab.sortDirection" :row-actions="canModifyRowsFor(resultTab)" @load-more="emit('loadMore')" @start-edit="resultTab.editing = true" @dirty="resultTab.dirty = $event" @save="(result, mutations) => emit('save', resultTab.id, result, mutations)" @cancel="resultTab.editing = false" @sort="(column, direction) => sortResult(resultTab, column, direction)" /></div>
  <Teleport to="body"><div v-if="pendingClose" class="fixed inset-0 z-[60] grid place-items-center bg-slate-950/55 p-4 backdrop-blur-sm" @mousedown.self="pendingClose = undefined"><section class="w-full max-w-md overflow-hidden rounded-2xl border border-line bg-panel shadow-2xl" role="dialog" aria-modal="true" :aria-label="t('tabs.saveChangesTitle')"><div class="p-6"><h2 class="text-base font-semibold tracking-tight">{{ t('tabs.saveChangesTitle') }}</h2><p class="mt-2 text-sm leading-6 text-muted">{{ t('tabs.saveBeforeClose', { name: pendingClose.title }) }}</p></div><div class="flex flex-col-reverse gap-2 border-t border-line bg-canvas/40 px-5 py-4 sm:flex-row sm:justify-end"><button type="button" class="rounded-lg px-3.5 py-2 text-sm font-medium text-muted hover:bg-panel hover:text-ink" @click="discardAndClose">{{ t('tabs.closeOnly') }}</button><button type="button" class="rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-white shadow-sm hover:bg-accent/90" @click="saveAndClose">{{ t('tabs.saveAndClose') }}</button></div></section></div></Teleport>
</template>
