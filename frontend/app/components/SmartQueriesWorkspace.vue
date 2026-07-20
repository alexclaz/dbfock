<script setup lang="ts">
import type { Connection, QueryResult, SmartQuery } from '~/types/database'

type SmartResultTab = { id: string; title: string; result?: QueryResult; view: 'table' | 'json' | 'csv'; copied: boolean; editing: boolean; source?: { connectionId: string; database: string; table: string } }

const props = defineProps<{ queries: SmartQuery[]; connections: Connection[]; resultTabs: SmartResultTab[]; activeResultTabId?: string; loading?: boolean; loadingMore?: boolean }>()
const emit = defineEmits<{ run: [query: SmartQuery, values: Record<string, string>, newTab: boolean]; remove: [id: string]; update: [id: string, changes: Pick<SmartQuery, 'title' | 'description' | 'sql'>]; openEditor: [query: SmartQuery]; selectResultTab: [id: string]; closeResultTab: [id: string]; setResultView: [id: string, view: SmartResultTab['view']]; copyResult: [id: string]; saveResult: [id: string, result: QueryResult]; loadMore: [] }>()
const { t } = useI18n()
const values = reactive<Record<string, Record<string, string>>>({})
const commandPressed = ref(false)
const editing = ref<SmartQuery>()
const editTitle = ref('')
const editDescription = ref('')
const editSQL = ref('')
const resultHeight = ref(46)
const resultGrid = ref<{ save: () => boolean; cancel: () => void; canSave: boolean }>()
const activeResultTab = computed(() => props.resultTabs.find((tab) => tab.id === props.activeResultTabId))
const hasResults = computed(() => props.resultTabs.some((tab) => Boolean(tab.result)))

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
        <span class="grid h-10 w-10 place-items-center rounded-xl bg-violet-500/10 text-violet-500">✦</span>
        <div><h1 class="text-xl font-semibold">{{ t('smartQueries.title') }}</h1><p class="mt-1 text-sm text-muted">{{ t('smartQueries.description') }}</p></div>
      </header>
      <div v-if="queries.length" class="grid gap-4 xl:grid-cols-2">
        <article v-for="query in queries" :key="query.id" class="rounded-xl border border-line bg-panel p-4 shadow-sm">
          <div class="flex items-start justify-between gap-3"><div class="min-w-0"><h2 class="truncate font-semibold">{{ query.title }}</h2><p class="mt-1 text-sm leading-5 text-muted">{{ query.description }}</p></div><span class="shrink-0 rounded bg-accent/10 px-2 py-0.5 text-xs text-accent">{{ connectionName(query) }}</span></div>
          <div class="mt-4 grid gap-3 sm:grid-cols-2">
            <label v-for="parameter in query.parameters" :key="parameter.key" class="block text-xs font-medium text-muted"><span class="mb-1 block">{{ parameter.key }}</span><input v-model="queryValues(query)[parameter.key]" type="text" class="h-9 w-full rounded-md border border-line bg-canvas px-2 text-sm text-ink outline-none focus:border-accent" /></label>
          </div>
          <div class="mt-4 flex items-center justify-between gap-3 border-t border-line pt-3"><button type="button" class="text-sm font-medium text-accent hover:underline" @click="openEdit(query)">{{ t('smartQueries.viewAndEdit') }}</button><div class="flex shrink-0 items-center gap-1"><button type="button" class="rounded-md p-2 text-rose-500 hover:bg-rose-500/10" :title="t('smartQueries.delete')" :aria-label="t('smartQueries.delete')" @click="emit('remove', query.id)"><svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 6h18M9 6V4h6v2m-8 0 1 15h8l1-15M10 10v7m4-7v7" /></svg></button><button type="button" class="rounded-md bg-accent px-3 py-2 text-sm font-medium text-white hover:opacity-90" @click="run(query, $event)">{{ commandPressed ? t('smartQueries.runNewTab') : t('smartQueries.run') }}</button></div></div>
        </article>
      </div>
      <div v-else class="rounded-xl border border-dashed border-line px-6 py-16 text-center"><div class="text-2xl text-muted">✦</div><h2 class="mt-3 font-medium">{{ t('smartQueries.emptyTitle') }}</h2><p class="mt-1 text-sm text-muted">{{ t('smartQueries.emptyDescription') }}</p></div>
      </div>
    </div>
    <div v-if="hasResults" class="h-1.5 shrink-0 cursor-row-resize bg-line hover:bg-accent" @pointerdown="resizeResults" />
    <section v-if="hasResults" class="flex min-h-0 shrink-0 flex-col overflow-hidden border-t border-line bg-panel" :style="{ height: `${resultHeight}%` }">
      <div v-if="resultTabs.length" class="flex h-9 items-end gap-1 overflow-x-auto border-b border-line bg-panel px-2"><button v-for="resultTab in resultTabs" :key="resultTab.id" type="button" class="group flex h-8 shrink-0 items-center gap-1 rounded-t px-2 text-xs" :class="activeResultTab?.id === resultTab.id ? 'bg-canvas font-medium text-ink' : 'text-muted hover:bg-canvas/60'" @click="emit('selectResultTab', resultTab.id)"><span>{{ resultTab.title }}</span><span class="rounded px-1 opacity-0 group-hover:opacity-100 hover:bg-line" :aria-label="t('common.close')" @click.stop="emit('closeResultTab', resultTab.id)">×</span></button></div>
      <div class="flex items-center justify-between border-b border-line px-4 py-2 text-xs text-muted"><span v-if="activeResultTab?.result">{{ t('query.rowsSummary', { rows: activeResultTab.result.rowCount, time: activeResultTab.result.executionTimeMs, affected: activeResultTab.result.affectedRows }) }}</span><span v-else>{{ t('query.queryResults') }}</span><div v-if="activeResultTab" class="flex items-center gap-2"><div class="flex rounded-md border border-line p-0.5"><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'table' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'table'" @click="emit('setResultView', activeResultTab.id, 'table')">{{ t('grid.table') }}</button><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'json' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'json'" @click="emit('setResultView', activeResultTab.id, 'json')">JSON</button><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'csv' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'csv'" @click="emit('setResultView', activeResultTab.id, 'csv')">CSV</button></div><button v-if="!activeResultTab.editing" type="button" class="rounded-md border border-line px-2.5 py-1 text-ink disabled:cursor-not-allowed disabled:opacity-50" :title="!activeResultTab.source ? t('grid.inlineEditUnsupported') : undefined" :disabled="!activeResultTab.result || activeResultTab.view === 'csv' || !activeResultTab.source" @click="activeResultTab.editing = true">{{ t('grid.edit') }}</button><button type="button" class="rounded-md border border-line px-2.5 py-1 text-ink disabled:cursor-not-allowed disabled:opacity-50" :disabled="!activeResultTab.result" @click="emit('copyResult', activeResultTab.id)">{{ activeResultTab.copied ? t('grid.copied') : t('grid.copy') }}</button><template v-if="activeResultTab.editing"><button type="button" class="rounded-md bg-accent px-2.5 py-1 font-medium text-white disabled:cursor-not-allowed disabled:opacity-50" :disabled="!resultGrid?.canSave" @click="resultGrid?.save()">{{ t('grid.save') }}</button><button type="button" class="rounded-md border border-line px-2.5 py-1 text-ink" @click="resultGrid?.cancel()">{{ t('grid.cancel') }}</button></template></div></div>
      <div class="min-h-0 flex-1"><DataGrid ref="resultGrid" :result="activeResultTab?.result" :loading="loading" :loading-more="loadingMore" :view="activeResultTab?.view" :editing="activeResultTab?.editing" :editable="Boolean(activeResultTab?.source)" @load-more="emit('loadMore')" @start-edit="activeResultTab && (activeResultTab.editing = true)" @save="activeResultTab && emit('saveResult', activeResultTab.id, $event)" @cancel="activeResultTab && (activeResultTab.editing = false)" /></div>
    </section>
  </section>
  <Teleport to="body">
    <div v-if="editing" class="fixed inset-0 z-50 grid place-items-center bg-slate-950/55 p-4 backdrop-blur-sm" @mousedown.self="editing = undefined">
      <form class="flex max-h-[90vh] w-full max-w-3xl flex-col overflow-hidden rounded-2xl border border-line bg-panel shadow-2xl" @submit.prevent="saveEdit">
        <div class="flex items-start justify-between gap-4 border-b border-line p-5"><div><h2 class="font-semibold">{{ t('smartQueries.editTitle') }}</h2><p class="mt-1 text-sm text-muted">{{ t('smartQueries.editDescription') }}</p></div><button type="button" class="rounded p-1 text-muted hover:bg-canvas" :aria-label="t('common.close')" @click="editing = undefined">×</button></div>
        <div class="scrollbar grid gap-4 overflow-auto p-5"><label class="block text-sm font-medium">{{ t('smartQueries.nameLabel') }}<input v-model="editTitle" required class="mt-1 h-10 w-full rounded-md border border-line bg-canvas px-3 text-sm outline-none focus:border-accent" /></label><label class="block text-sm font-medium">{{ t('smartQueries.descriptionLabel') }}<textarea v-model="editDescription" required rows="3" class="mt-1 w-full rounded-md border border-line bg-canvas px-3 py-2 text-sm outline-none focus:border-accent" /></label><label class="block text-sm font-medium">{{ t('smartQueries.sqlLabel') }}<textarea v-model="editSQL" required rows="12" spellcheck="false" class="mt-1 w-full rounded-md border border-line bg-canvas px-3 py-2 font-mono text-xs leading-5 outline-none focus:border-accent" /></label></div>
        <div class="flex flex-col-reverse gap-2 border-t border-line bg-canvas/40 px-5 py-4 sm:flex-row sm:items-center sm:justify-between"><button type="button" class="rounded-lg border border-line px-3.5 py-2 text-sm font-medium hover:bg-panel" @click="openInEditor">{{ t('smartQueries.openInEditor') }}</button><div class="flex gap-2"><button type="button" class="rounded-lg px-3.5 py-2 text-sm font-medium text-muted hover:bg-panel" @click="editing = undefined">{{ t('connection.cancel') }}</button><button type="submit" class="rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-white hover:bg-accent/90">{{ t('common.save') }}</button></div></div>
      </form>
    </div>
  </Teleport>
</template>
