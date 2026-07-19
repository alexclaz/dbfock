<script setup lang="ts">
import type { QueryResult } from '~/types/database'
import { queryResultAsCSV, queryResultAsJSON } from '~/utils/queryResult'
const props = withDefaults(defineProps<{ result?: QueryResult; loading?: boolean; loadingMore?: boolean; view?: 'table' | 'json' | 'csv' }>(), { view: 'table' })
const emit = defineEmits<{ loadMore: [] }>()
const { t } = useI18n()
function display(value: unknown) { if (value === null) return 'NULL'; if (typeof value === 'boolean') return value ? 'true' : 'false'; return String(value) }
const columns = computed(() => props.result?.columns ?? [])
const rows = computed(() => props.result?.rows ?? [])
const formattedRows = computed(() => props.view === 'csv' ? queryResultAsCSV(props.result) : queryResultAsJSON(props.result))
const highlightedJSON = computed(() => highlightJSON(formattedRows.value))

function highlightJSON(json: string) {
  const escaped = json.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
  return escaped.replace(/("(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*")(?=\s*:)|("(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*")|\b(true|false)\b|\bnull\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/g, (match, key, string) => {
    const color = key ? 'text-json-key' : string ? 'text-json-string' : match === 'true' || match === 'false' ? 'text-json-boolean' : match === 'null' ? 'text-json-null' : 'text-json-number'
    return `<span class="${color}">${match}</span>`
  })
}
function loadMore(event: Event) {
  if (!props.result?.hasMore || props.loading || props.loadingMore) return
  const element = event.currentTarget as HTMLElement
  if (element.scrollHeight - element.scrollTop - element.clientHeight <= 80) emit('loadMore')
}
</script>
<template><div class="scrollbar h-full overflow-auto" @scroll="loadMore"><div v-if="loading" class="p-5 text-sm text-muted">{{ t('grid.loading') }}</div><div v-else-if="!result" class="grid h-full place-items-center p-8 text-center text-sm text-muted">{{ t('grid.empty') }}</div><table v-else-if="view === 'table'" class="min-w-full border-collapse text-left text-sm"><thead class="sticky top-0 bg-panel text-xs text-muted"><tr><th class="w-12 border-b border-r border-line px-3 py-2 font-medium">#</th><th v-for="column in columns" :key="column.name" class="border-b border-r border-line px-3 py-2 font-medium"><div>{{ column.name }}</div><small class="font-normal opacity-70">{{ column.databaseType }}</small></th></tr></thead><tbody><tr v-for="(row,index) in rows" :key="index" class="hover:bg-accent/5"><td class="border-b border-r border-line px-3 py-2 text-xs text-muted">{{ index + 1 }}</td><td v-for="column in columns" :key="column.name" class="max-w-xs truncate border-b border-r border-line px-3 py-2" :class="row[column.name] === null ? 'italic text-muted' : ''" :title="display(row[column.name])">{{ display(row[column.name]) }}</td></tr><tr v-if="loadingMore"><td :colspan="columns.length + 1" class="p-3 text-center text-xs text-muted">{{ t('grid.loading') }}</td></tr><tr v-if="!rows.length"><td :colspan="columns.length + 1" class="p-8 text-center text-muted">{{ t('grid.noRows') }}</td></tr></tbody></table><pre v-else class="min-h-full bg-canvas whitespace-pre-wrap break-words p-4 font-mono text-sm text-ink" v-html="view === 'json' ? highlightedJSON : formattedRows" /></div></template>
