<script setup lang="ts">
import type { Connection, QueryResult, SmartQuery } from '~/types/database'

type SmartResultTab = { id: string; title: string; result?: QueryResult; view: 'table' | 'json' | 'csv'; copied: boolean; editing: boolean; sources?: { connectionId: string; database: string; table: string; columns: string[]; primaryKey: string[] }[] }

const props = defineProps<{ queries: SmartQuery[]; connections: Connection[]; resultTabs: SmartResultTab[]; activeResultTabId?: string; loading?: boolean; loadingMore?: boolean }>()
const emit = defineEmits<{ run: [query: SmartQuery, values: Record<string, string>, newTab: boolean]; remove: [id: string]; update: [id: string, changes: Pick<SmartQuery, 'title' | 'description' | 'sql'>]; openEditor: [query: SmartQuery]; selectResultTab: [id: string]; closeResultTab: [id: string]; copyResult: [id: string]; saveResult: [id: string, result: QueryResult]; loadMore: [] }>()
const { t } = useI18n()
const values = reactive<Record<string, Record<string, string>>>({})
const commandPressed = ref(false)
const editing = ref<SmartQuery>()
const editTitle = ref('')
const editDescription = ref('')
const editSQL = ref('')
const resultHeight = ref(46)
const activeResultTab = computed(() => props.resultTabs.find((tab) => tab.id === props.activeResultTabId))
const hasResults = computed(() => props.resultTabs.some((tab) => Boolean(tab.result)))
const resultSummary = computed(() => activeResultTab.value?.result ? t('query.rowsSummary', { rows: activeResultTab.value.result.rowCount, time: activeResultTab.value.result.executionTimeMs, affected: activeResultTab.value.result.affectedRows }) : t('query.queryResults'))

function queryValues(query: SmartQuery) {
  return values[query.id] ??= Object.fromEntries(query.parameters.map((parameter) => [parameter.key, parameter.defaultValue]))
}
function connectionName(query: SmartQuery) { return props.connections.find((connection) => connection.id === query.connectionId)?.name || t('savedQueries.connectionUnavailable') }
function openEdit(query: SmartQuery) {
  editing.value = query
  editTitle.value = query.title
  editDescription.value = query.description
  editSQL.value = query.sql
}
function editChanges() { return { title: editTitle.value.trim(), description: editDescription.value.trim(), sql: editSQL.value.trim() } }
function saveEdit() {
  if (!editing.value || !editTitle.value.trim() || !editDescription.value.trim() || !editSQL.value.trim()) return
  emit('update', editing.value.id, editChanges())
  editing.value = undefined
}
function removeEditing() {
  if (!editing.value) return
  emit('remove', editing.value.id)
  editing.value = undefined
}
function openInEditor() {
  if (!editing.value || !editTitle.value.trim() || !editDescription.value.trim() || !editSQL.value.trim()) return
  const query = { ...editing.value, ...editChanges() }
  emit('update', query.id, editChanges())
  emit('openEditor', query)
  editing.value = undefined
}
function run(query: SmartQuery, event: MouseEvent) { emit('run', query, queryValues(query), event.metaKey) }
function setCommandState(event: KeyboardEvent) { commandPressed.value = event.metaKey }
function resizeResults(event: PointerEvent) {
  const container = (event.currentTarget as HTMLElement).parentElement
  if (!container) return
  const startY = event.clientY
  const startHeight = resultHeight.value
  const containerHeight = container.getBoundingClientRect().height
  const resize = (moveEvent: PointerEvent) => { resultHeight.value = Math.min(75, Math.max(24, startHeight + ((startY - moveEvent.clientY) / containerHeight) * 100)) }
  const stop = () => {
    window.removeEventListener('pointermove', resize)
    window.removeEventListener('pointerup', stop)
  }
  window.addEventListener('pointermove', resize)
  window.addEventListener('pointerup', stop)
}
onMounted(() => {
  window.addEventListener('keydown', setCommandState)
  window.addEventListener('keyup', setCommandState)
  window.addEventListener('blur', () => { commandPressed.value = false })
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', setCommandState)
  window.removeEventListener('keyup', setCommandState)
})
</script>

<template>
  <section class="flex h-full min-h-0 flex-col">
    <div class="scrollbar min-h-0 flex-1 overflow-auto p-5 lg:p-6">
      <div class="w-full">
      <header class="mb-6 flex items-start gap-3">
        <span class="grid h-10 w-10 place-items-center rounded-xl bg-violet-500/10 text-violet-500"><Icon name="lucide:sparkles" class="h-5 w-5" aria-hidden="true" /></span>
        <div><h1 class="text-xl font-semibold">{{ t('smartQueries.title') }}</h1><p class="mt-1 text-sm text-muted">{{ t('smartQueries.description') }}</p></div>
      </header>
      <div v-if="queries.length" class="grid gap-4 xl:grid-cols-2">
        <article v-for="query in queries" :key="query.id" class="rounded-xl border border-line bg-panel p-4 shadow-sm">
          <div class="flex items-start justify-between gap-3"><div class="min-w-0"><h2 class="truncate font-semibold">{{ query.title }}</h2><p class="mt-1 text-sm leading-5 text-muted">{{ query.description }}</p></div><span class="shrink-0 rounded bg-accent/10 px-2 py-0.5 text-xs text-accent">{{ connectionName(query) }}</span></div>
          <div class="mt-4 grid gap-3 sm:grid-cols-2">
            <label v-for="parameter in query.parameters" :key="parameter.key" class="block text-xs font-medium text-muted"><span class="mb-1 block">{{ parameter.key }}</span><input v-model="queryValues(query)[parameter.key]" type="text" class="h-9 w-full rounded-md border border-line bg-canvas px-2 text-sm text-ink outline-none focus:border-accent" ></label>
          </div>
          <div class="mt-4 flex items-center justify-between gap-3 border-t border-line pt-3"><button type="button" class="text-sm font-medium text-accent hover:underline" @click="openEdit(query)">{{ t('smartQueries.viewAndEdit') }}</button><div class="flex shrink-0 items-center gap-1"><button type="button" class="rounded-md bg-accent px-3 py-2 text-sm font-medium text-white hover:opacity-90" @click="run(query, $event)">{{ commandPressed ? t('smartQueries.runNewTab') : t('smartQueries.run') }}</button></div></div>
        </article>
      </div>
      <div v-else class="rounded-xl border border-dashed border-line px-6 py-16 text-center"><Icon name="lucide:sparkles" class="mx-auto h-7 w-7 text-muted" aria-hidden="true" /><h2 class="mt-3 font-medium">{{ t('smartQueries.emptyTitle') }}</h2><p class="mt-1 text-sm text-muted">{{ t('smartQueries.emptyDescription') }}</p></div>
      </div>
    </div>
    <div v-if="hasResults" class="h-1.5 shrink-0 cursor-row-resize bg-line hover:bg-accent" @pointerdown="resizeResults" />
    <section v-if="hasResults" class="flex min-h-0 shrink-0 flex-col overflow-hidden border-t border-line bg-panel" :style="{ height: `${resultHeight}%` }">
      <QueryResults :result-tabs="resultTabs" :active-result-tab-id="activeResultTabId" :loading="loading" :loading-more="loadingMore" :summary="resultSummary" @select-tab="emit('selectResultTab', $event)" @close-tab="emit('closeResultTab', $event)" @copy="emit('copyResult', $event)" @save="(id, result) => emit('saveResult', id, result)" @load-more="emit('loadMore')" />
    </section>
  </section>
  <Teleport to="body">
    <div v-if="editing" class="fixed inset-0 z-50 grid place-items-center bg-slate-950/55 p-4 backdrop-blur-sm" @mousedown.self="editing = undefined">
      <form class="flex max-h-[90vh] w-full max-w-3xl flex-col overflow-hidden rounded-2xl border border-line bg-panel shadow-2xl" @submit.prevent="saveEdit">
        <div class="flex items-start justify-between gap-4 border-b border-line p-5"><div><h2 class="font-semibold">{{ t('smartQueries.editTitle') }}</h2><p class="mt-1 text-sm text-muted">{{ t('smartQueries.editDescription') }}</p></div><button type="button" class="rounded p-1 text-muted hover:bg-canvas" :aria-label="t('common.close')" @click="editing = undefined"><Icon name="lucide:x" class="h-4 w-4" aria-hidden="true" /></button></div>
        <div class="scrollbar grid gap-4 overflow-auto p-5"><label class="block text-sm font-medium">{{ t('smartQueries.nameLabel') }}<input v-model="editTitle" required class="mt-1 h-10 w-full rounded-md border border-line bg-canvas px-3 text-sm outline-none focus:border-accent" ></label><label class="block text-sm font-medium">{{ t('smartQueries.descriptionLabel') }}<textarea v-model="editDescription" required rows="3" class="mt-1 w-full rounded-md border border-line bg-canvas px-3 py-2 text-sm outline-none focus:border-accent" /></label><label class="block text-sm font-medium">{{ t('smartQueries.sqlLabel') }}<textarea v-model="editSQL" required rows="12" spellcheck="false" class="mt-1 w-full rounded-md border border-line bg-canvas px-3 py-2 font-mono text-xs leading-5 outline-none focus:border-accent" /></label></div>
        <div class="flex flex-col-reverse gap-2 border-t border-line bg-canvas/40 px-5 py-4 sm:flex-row sm:items-center sm:justify-between"><button type="button" class="rounded-lg border border-line px-3.5 py-2 text-sm font-medium hover:bg-panel" @click="openInEditor">{{ t('smartQueries.openInEditor') }}</button><div class="flex gap-2"><button type="button" class="rounded-lg px-3.5 py-2 text-sm font-medium text-rose-500 hover:bg-rose-500/10" @click="removeEditing">{{ t('smartQueries.delete') }}</button><button type="button" class="rounded-lg px-3.5 py-2 text-sm font-medium text-muted hover:bg-panel" @click="editing = undefined">{{ t('connection.cancel') }}</button><button type="submit" class="rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-white hover:bg-accent/90">{{ t('common.save') }}</button></div></div>
      </form>
    </div>
  </Teleport>
</template>
