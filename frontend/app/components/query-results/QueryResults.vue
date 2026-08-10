<script setup lang="ts">
import type { QueryResult } from '~/types/database'

type ResultTab = {
  id: string
  title: string
  result?: QueryResult
  view: 'table' | 'json' | 'csv'
  copied: boolean
  editing: boolean
  sources?: { columns: string[] }[]
  sortColumn?: string
  sortDirection?: 'asc' | 'desc'
  refreshable?: boolean
}
type GridMutations = { insertedRows: { row: Record<string, unknown>; useDefaults: boolean }[]; deletedRows: Record<string, unknown>[] }

const props = withDefaults(defineProps<{ resultTabs: ResultTab[]; activeResultTabId?: string; loading?: boolean; loadingMore?: boolean; canCreateTab?: boolean; summary?: string }>(), { canCreateTab: false, summary: '' })
const emit = defineEmits<{ selectTab: [id: string]; closeTab: [id: string]; createTab: []; copy: [id: string]; save: [id: string, result: QueryResult, mutations: GridMutations]; loadMore: []; sort: [id: string, column: string, direction: 'asc' | 'desc']; refresh: [id: string] }>()
const { t } = useI18n()
const resultGrid = ref<{ save: () => boolean; cancel: () => void; addRow: () => Promise<void>; canSave: boolean }>()
const activeResultTab = computed(() => props.resultTabs.find((tab) => tab.id === props.activeResultTabId))
const showSearch = ref(false)
const search = ref('')
const searchInput = ref<HTMLInputElement>()
const searchActive = computed(() => showSearch.value && search.value.trim().length > 0)
const canModifyRows = computed(() => Boolean(activeResultTab.value?.sources?.length === 1 && activeResultTab.value.view === 'table' && !searchActive.value))
const displayedResult = computed(() => {
  const current = activeResultTab.value?.result
  if (!current || !searchActive.value) return current
  const needle = search.value.trim().toLowerCase()
  const rows = current.rows.filter((row) => current.columns.some((column) => (row[column.name] === null ? 'null' : String(row[column.name])).toLowerCase().includes(needle)))
  return { ...current, rows, rowCount: rows.length, hasMore: false }
})
async function openSearch() { showSearch.value = true; await nextTick(); searchInput.value?.focus(); searchInput.value?.select() }
function closeSearch() { showSearch.value = false; search.value = '' }
function sortResult(column: string, direction: 'asc' | 'desc') {
  const resultTab = activeResultTab.value
  if (!resultTab?.result || resultTab.editing) return
  emit('sort', resultTab.id, column, direction)
}
watch(searchActive, (active) => { if (active && activeResultTab.value?.editing) resultGrid.value?.cancel() })
</script>

<template>
  <div v-if="resultTabs.length" class="flex h-9 items-end gap-1 overflow-x-auto border-b border-line bg-panel px-2"><button v-for="resultTab in resultTabs" :key="resultTab.id" type="button" class="group flex h-8 shrink-0 items-center gap-1 rounded-t px-2 text-xs" :class="activeResultTab?.id === resultTab.id ? 'bg-canvas font-medium text-ink' : 'text-muted hover:bg-canvas/60'" @click="emit('selectTab', resultTab.id)"><span>{{ resultTab.title }}</span><span class="rounded p-1 opacity-0 group-hover:opacity-100 hover:bg-line" :aria-label="t('common.close')" @click.stop="emit('closeTab', resultTab.id)"><Icon name="lucide:x" class="h-3.5 w-3.5" aria-hidden="true" /></span></button><button v-if="canCreateTab" type="button" class="mb-1 grid h-6 w-6 shrink-0 place-items-center rounded text-muted hover:bg-canvas hover:text-ink" :title="t('query.newResultTab')" :aria-label="t('query.newResultTab')" @click="emit('createTab')"><Icon name="lucide:plus" class="h-4 w-4" aria-hidden="true" /></button></div>
  <div class="flex items-center justify-between border-b border-line px-4 py-2 text-xs text-muted"><span>{{ summary || t('query.results') }}</span><div v-if="activeResultTab" class="flex items-center gap-2"><button v-if="canModifyRows" type="button" class="grid rounded p-1 hover:bg-canvas" :title="t('grid.addRow')" :aria-label="t('grid.addRow')" @click="resultGrid?.addRow()"><Icon name="lucide:plus" class="h-4 w-4" aria-hidden="true" /></button><button type="button" class="grid rounded p-1 hover:bg-canvas disabled:opacity-60" :title="activeResultTab.copied ? t('grid.copied') : t('grid.copy')" :aria-label="activeResultTab.copied ? t('grid.copied') : t('grid.copy')" :disabled="!activeResultTab.result" @click="emit('copy', activeResultTab.id)"><Icon :name="activeResultTab.copied ? 'lucide:check' : 'lucide:copy'" class="h-4 w-4" aria-hidden="true" /></button><button type="button" class="grid rounded p-1 hover:bg-canvas disabled:opacity-60" :title="t('grid.refresh')" :aria-label="t('grid.refresh')" :disabled="!activeResultTab.refreshable || activeResultTab.editing || loading || loadingMore" @click="emit('refresh', activeResultTab.id)"><Icon name="lucide:refresh-cw" class="h-4 w-4" aria-hidden="true" /></button><button type="button" class="grid rounded p-1 hover:bg-canvas" :class="showSearch ? 'bg-canvas text-ink' : ''" :title="t('grid.search')" :aria-label="t('grid.search')" :aria-pressed="showSearch" @click="showSearch ? closeSearch() : openSearch()"><Icon name="lucide:search" class="h-4 w-4" aria-hidden="true" /></button><div class="flex rounded-md border border-line p-0.5"><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'table' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'table'" @click="activeResultTab.view = 'table'">{{ t('grid.table') }}</button><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'json' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'json'" @click="activeResultTab.view = 'json'">JSON</button><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'csv' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'csv'" @click="activeResultTab.view = 'csv'">CSV</button></div><template v-if="activeResultTab.editing && resultGrid?.canSave"><button type="button" class="rounded-md bg-accent px-2.5 py-1 font-medium text-white" @click="resultGrid?.save()">{{ t('grid.save') }}</button><button type="button" class="rounded-md border border-line px-2.5 py-1 text-ink" @click="activeResultTab.editing = false">{{ t('grid.cancel') }}</button></template></div></div>
  <div v-if="showSearch" class="flex items-center gap-2 border-b border-line px-4 py-2 text-xs"><Icon name="lucide:search" class="h-4 w-4 shrink-0 text-muted" aria-hidden="true" /><input ref="searchInput" v-model="search" type="search" class="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted" :placeholder="t('grid.searchPlaceholder')" :aria-label="t('grid.search')" @keydown.esc.prevent="closeSearch"><span v-if="searchActive" class="shrink-0 text-muted">{{ displayedResult?.rows.length ? t('grid.searchMatches', { count: displayedResult.rows.length, total: activeResultTab?.result?.rows.length || 0 }) : t('grid.searchNoMatches') }}</span><button type="button" class="grid shrink-0 rounded p-1 text-muted hover:bg-canvas hover:text-ink" :title="t('grid.searchClose')" :aria-label="t('grid.searchClose')" @click="closeSearch"><Icon name="lucide:x" class="h-4 w-4" aria-hidden="true" /></button></div>
  <div class="min-h-0 flex-1"><DataGrid ref="resultGrid" :result="displayedResult" :loading="loading" :loading-more="loadingMore" :view="activeResultTab?.view" :editing="activeResultTab?.editing" :editable="Boolean(activeResultTab?.sources?.length) && !searchActive" :editable-columns="activeResultTab?.sources?.flatMap((source) => source.columns)" :json-editable="!activeResultTab?.sources?.length" :sortable="!activeResultTab?.editing" :sort-column="activeResultTab?.sortColumn" :sort-direction="activeResultTab?.sortDirection" :row-actions="canModifyRows" @load-more="emit('loadMore')" @start-edit="activeResultTab && (activeResultTab.editing = true)" @save="(result, mutations) => activeResultTab && emit('save', activeResultTab.id, result, mutations)" @cancel="activeResultTab && (activeResultTab.editing = false)" @sort="sortResult" /></div>
</template>
