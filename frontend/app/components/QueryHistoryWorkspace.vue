<script setup lang="ts">
import type { Connection, QueryHistory, QueryTabHistory } from '~/types/database'

const props = defineProps<{ tabs: QueryTabHistory[]; queries: QueryHistory[]; connections: Connection[] }>()
const emit = defineEmits<{ openTab: [tab: QueryTabHistory]; removeTab: [tab: QueryTabHistory]; open: [query: QueryHistory]; save: [query: QueryHistory]; remove: [query: QueryHistory] }>()
const { t } = useI18n()
const expandedQueryIds = ref(new Set<string>())

function connectionName(query: { connectionId: string }) { return props.connections.find((connection) => connection.id === query.connectionId)?.name || t('savedQueries.connectionUnavailable') }
function isExpanded(id: string) { return expandedQueryIds.value.has(id) }
function toggleExpanded(id: string) {
  const next = new Set(expandedQueryIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedQueryIds.value = next
}
function preview(sql: string) { return sql.trim().replace(/\s+/g, ' ') }
function date(value: string) { return value.replace('T', ' ').replace(/\.\d+Z$/, '').slice(0, 16) }
</script>

<template>
  <section class="scrollbar h-full overflow-auto p-5 lg:p-6">
    <div class="w-full">
      <header class="mb-6"><div class="flex items-center gap-3"><span class="grid h-10 w-10 place-items-center rounded-xl bg-accent/10 text-accent"><Icon name="lucide:history" class="h-5 w-5" aria-hidden="true" /></span><div><h1 class="text-xl font-semibold">{{ t('query.historyTitle') }}</h1><p class="mt-1 text-sm text-muted">{{ t('query.historyDescription') }}</p></div></div></header>
      <section class="mb-8">
        <div class="mb-3 flex items-center justify-between gap-3"><h2 class="text-sm font-semibold">{{ t('query.tabHistoryTitle') }}</h2><span class="rounded bg-canvas px-2 py-0.5 text-xs text-muted">{{ tabs.length }}</span></div>
        <div v-if="tabs.length" class="grid gap-3">
          <article v-for="tab in tabs" :key="tab.id" class="rounded-xl border border-line bg-panel p-4">
            <div class="flex items-start gap-3"><Icon name="lucide:file-code-2" class="mt-0.5 h-4 w-4 shrink-0 text-accent" aria-hidden="true" /><div class="min-w-0 flex-1"><div class="flex flex-wrap items-center gap-2 text-xs"><strong class="text-sm text-ink">{{ tab.title }}</strong><span class="text-muted">{{ connectionName(tab) }}</span><time class="text-muted">{{ date(tab.closedAt) }}</time></div><code class="mt-2 block max-h-10 overflow-hidden font-mono text-[11px] leading-5 text-muted whitespace-pre-wrap break-words">{{ preview(tab.sql) }}</code><div class="mt-3 flex flex-wrap items-center gap-3"><button type="button" class="text-xs font-medium text-accent hover:underline" @click="emit('openTab', tab)">{{ t('query.reopenTabHistory') }}</button><button type="button" class="text-xs font-medium text-rose-500 hover:underline" @click="emit('removeTab', tab)">{{ t('query.removeTabHistory') }}</button></div></div></div>
          </article>
        </div>
        <div v-else class="rounded-xl border border-dashed border-line px-6 py-10 text-center"><Icon name="lucide:file-clock" class="mx-auto h-6 w-6 text-muted" aria-hidden="true" /><p class="mt-3 text-sm text-muted">{{ t('query.emptyTabHistory') }}</p></div>
      </section>
      <section>
      <div class="mb-3 flex items-center justify-between gap-3"><h2 class="text-sm font-semibold">{{ t('query.executionHistoryTitle') }}</h2><span class="rounded bg-canvas px-2 py-0.5 text-xs text-muted">{{ queries.length }}</span></div>
      <div v-if="queries.length" class="grid gap-3">
        <article v-for="query in queries" :key="query.id" class="rounded-xl border border-line bg-panel p-4">
          <div class="flex items-start gap-3"><Icon :name="query.status === 'error' ? 'lucide:circle-x' : 'lucide:circle-check'" class="mt-0.5 h-4 w-4 shrink-0" :class="query.status === 'error' ? 'text-rose-500' : 'text-emerald-500'" aria-hidden="true" /><div class="min-w-0 flex-1"><div class="flex flex-wrap items-center gap-2 text-xs"><span class="rounded bg-canvas px-2 py-0.5 font-medium text-ink">{{ query.type }}</span><span class="text-muted">{{ connectionName(query) }}</span><time class="text-muted">{{ date(query.createdAt) }}</time><span class="text-muted">{{ query.executionTimeMs }} ms</span></div><code class="mt-2 block font-mono text-[11px] leading-5 text-muted whitespace-pre-wrap break-words" :class="isExpanded(query.id) ? 'max-h-80 overflow-auto pr-2' : 'max-h-10 overflow-hidden'">{{ isExpanded(query.id) ? query.sql : preview(query.sql) }}</code><p v-if="query.errorMessage" class="mt-2 text-xs text-rose-500">{{ query.errorMessage }}</p><div class="mt-3 flex flex-wrap items-center gap-3"><button type="button" class="text-xs font-medium text-accent hover:underline" :aria-expanded="isExpanded(query.id)" @click="toggleExpanded(query.id)">{{ isExpanded(query.id) ? t('savedQueries.collapseQuery') : t('savedQueries.expandQuery') }}</button><button type="button" class="text-xs font-medium text-accent hover:underline" @click="emit('open', query)">{{ t('query.openFromHistory') }}</button><button type="button" class="text-xs font-medium text-accent hover:underline" @click="emit('save', query)">{{ t('query.saveFromHistory') }}</button><button type="button" class="text-xs font-medium text-rose-500 hover:underline" @click="emit('remove', query)">{{ t('query.deleteHistory') }}</button></div></div></div>
        </article>
      </div>
      <div v-else class="rounded-xl border border-dashed border-line px-6 py-10 text-center"><Icon name="lucide:history" class="mx-auto h-6 w-6 text-muted" aria-hidden="true" /><p class="mt-3 text-sm text-muted">{{ t('query.emptyHistoryDescription') }}</p></div>
      </section>
    </div>
  </section>
</template>
