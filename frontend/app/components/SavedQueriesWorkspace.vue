<script setup lang="ts">
import type { Connection, SavedQuery } from '~/types/database'

const props = defineProps<{ queries: SavedQuery[]; connections: Connection[] }>()
const emit = defineEmits<{ open: [id: string]; remove: [id: string] }>()
const { t } = useI18n()
const expandedQueryIds = ref(new Set<string>())

function connectionFor(query: SavedQuery) { return props.connections.find((connection) => connection.id === query.connectionId) }
function locationFor(query: SavedQuery) {
  const connection = connectionFor(query)
  if (!connection) return t('savedQueries.connectionUnavailable')
  return connection.initialDatabase ? `${connection.name} · ${connection.initialDatabase}` : connection.name
}
function sqlPreview(sql: string) {
  const trimmed = sql.trim()
  if (trimmed.includes('\n')) return trimmed
  return trimmed.replace(/\s+\b(LEFT\s+(?:OUTER\s+)?JOIN|RIGHT\s+(?:OUTER\s+)?JOIN|INNER\s+JOIN|CROSS\s+JOIN|GROUP\s+BY|ORDER\s+BY|INSERT\s+INTO|DELETE\s+FROM|UNION\s+ALL|SELECT|FROM|WHERE|HAVING|LIMIT|OFFSET|JOIN|UNION|UPDATE|SET|VALUES)\b/gi, '\n$1')
}
function isExpanded(queryId: string) { return expandedQueryIds.value.has(queryId) }
function toggleExpanded(queryId: string) {
  const next = new Set(expandedQueryIds.value)
  if (next.has(queryId)) next.delete(queryId)
  else next.add(queryId)
  expandedQueryIds.value = next
}
</script>

<template>
  <section class="scrollbar h-full overflow-auto p-5 lg:p-6">
    <div class="w-full">
      <header class="mb-6"><div class="flex items-center gap-3"><span class="grid h-10 w-10 place-items-center rounded-xl bg-accent/10 text-accent"><Icon name="lucide:bookmark" class="h-5 w-5" aria-hidden="true" /></span><div><h1 class="text-xl font-semibold">{{ t('savedQueries.title') }}</h1><p class="mt-1 text-sm text-muted">{{ t('savedQueries.description') }}</p></div></div></header>
      <div v-if="queries.length" class="grid gap-3">
        <article v-for="query in queries" :key="query.id" tabindex="0" class="group cursor-pointer rounded-xl border border-line bg-panel p-4 text-left transition-colors hover:border-accent/40 hover:bg-canvas focus:outline-none focus:ring-2 focus:ring-accent/50" @click="emit('open', query.id)" @keydown.enter="emit('open', query.id)" @keydown.space.prevent="emit('open', query.id)">
          <div class="flex items-start gap-3"><Icon name="lucide:bookmark" class="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" /><span class="min-w-0 flex-1"><span class="flex items-center justify-between gap-3"><strong class="truncate text-sm">{{ query.name }}</strong><span class="shrink-0 rounded bg-accent/10 px-2 py-0.5 text-xs text-accent">{{ t('savedQueries.connection', { name: locationFor(query) }) }}</span></span><code class="mt-1.5 block font-mono text-[11px] leading-5 text-muted whitespace-pre-wrap break-words" :class="isExpanded(query.id) ? 'max-h-80 overflow-auto pr-2' : 'max-h-10 overflow-hidden'">{{ sqlPreview(query.sql) }}</code><button type="button" class="mt-2 text-xs font-medium text-accent hover:underline" :aria-expanded="isExpanded(query.id)" @click.stop="toggleExpanded(query.id)">{{ isExpanded(query.id) ? t('savedQueries.collapseQuery') : t('savedQueries.expandQuery') }}</button></span><button type="button" class="rounded p-1.5 text-rose-500 opacity-0 transition-opacity hover:bg-rose-500/10 group-hover:opacity-100 focus:opacity-100" :title="t('savedQueries.delete')" :aria-label="t('savedQueries.delete')" @click.stop="emit('remove', query.id)"><Icon name="lucide:trash-2" class="h-4 w-4" aria-hidden="true" /></button><Icon name="lucide:arrow-right" class="h-4 w-4 text-muted opacity-0 transition-opacity group-hover:opacity-100" aria-hidden="true" /></div>
        </article>
      </div>
      <div v-else class="rounded-xl border border-dashed border-line px-6 py-16 text-center"><Icon name="lucide:bookmark" class="mx-auto h-7 w-7 text-muted" aria-hidden="true" /><h2 class="mt-3 font-medium">{{ t('savedQueries.emptyTitle') }}</h2><p class="mt-1 text-sm text-muted">{{ t('savedQueries.emptyDescription') }}</p></div>
    </div>
  </section>
</template>
