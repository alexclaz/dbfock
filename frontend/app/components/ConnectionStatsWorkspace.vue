<script setup lang="ts">
import type { Connection, ConnectionMetadata, ConnectionStats } from '~/types/database'

type StatsSection = 'stats' | 'session-status' | 'global-status' | 'session-variables' | 'global-variables' | 'engines' | 'user-privileges' | 'plugins'

const props = defineProps<{ connection: Connection }>()
const api = useApi()
const { locale, t } = useI18n()
const activeSection = ref<StatsSection>('stats')
const stats = ref<ConnectionStats>()
const metadata = reactive<Partial<Record<Exclude<StatsSection, 'stats'>, ConnectionMetadata>>>({})
const statsError = ref('')
const metadataError = ref('')
const loadingStats = ref(false)
const loadingMetadata = ref(false)
const filter = ref('')

const sections: { id: StatsSection; label: string; icon: string; description: string }[] = [
  { id: 'stats', label: 'stats.section.stats', icon: 'lucide:chart-no-axes-combined', description: 'stats.description' },
  { id: 'session-status', label: 'stats.section.sessionStatus', icon: 'lucide:activity', description: 'stats.sessionStatusDescription' },
  { id: 'global-status', label: 'stats.section.globalStatus', icon: 'lucide:globe-2', description: 'stats.globalStatusDescription' },
  { id: 'session-variables', label: 'stats.section.sessionVariables', icon: 'lucide:list', description: 'stats.sessionVariablesDescription' },
  { id: 'global-variables', label: 'stats.section.globalVariables', icon: 'lucide:list-tree', description: 'stats.globalVariablesDescription' },
  { id: 'engines', label: 'stats.section.engines', icon: 'lucide:database', description: 'stats.enginesDescription' },
  { id: 'user-privileges', label: 'stats.section.userPrivileges', icon: 'lucide:shield-check', description: 'stats.userPrivilegesDescription' },
  { id: 'plugins', label: 'stats.section.plugins', icon: 'lucide:blocks', description: 'stats.pluginsDescription' },
]

const maxDayCount = computed(() => Math.max(1, ...(stats.value?.queriesByDay.map((day) => day.success + day.failed) ?? [0])))
const maxOperationCount = computed(() => Math.max(1, ...(stats.value?.queriesByOperation.map((operation) => operation.count) ?? [0])))
const successRate = computed(() => stats.value?.totalQueries ? Math.round(stats.value.successfulQueries / stats.value.totalQueries * 100) : 0)
const activeMetadata = computed(() => activeSection.value === 'stats' ? undefined : metadata[activeSection.value])
const activeSectionDetails = computed(() => sections.find((section) => section.id === activeSection.value)!)
const filteredRows = computed(() => {
  const rows = activeMetadata.value?.rows ?? []
  const term = filter.value.trim().toLocaleLowerCase(locale.value)
  return term ? rows.filter((row) => row.some((value) => value.toLocaleLowerCase(locale.value).includes(term))) : rows
})

function dayLabel(date: string) { return new Intl.DateTimeFormat(locale.value, { weekday: 'short' }).format(new Date(`${date}T12:00:00`)).replace('.', '') }
function number(value: number) { return new Intl.NumberFormat(locale.value).format(value) }
function lastQuery(value?: string) { return value ? new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : t('stats.noneYet') }
function barHeight(value: number) { return `${Math.max(value ? 8 : 2, value / maxDayCount.value * 100)}%` }
function operationWidth(value: number) { return `${value / maxOperationCount.value * 100}%` }

async function loadStats() {
  loadingStats.value = true
  statsError.value = ''
  try { stats.value = await api<ConnectionStats>(`/connections/${props.connection.id}/stats`) }
  catch (cause: any) { statsError.value = cause.message || t('stats.loadError') }
  finally { loadingStats.value = false }
}
async function loadMetadata(section = activeSection.value) {
  if (section === 'stats') return loadStats()
  loadingMetadata.value = true
  metadataError.value = ''
  try { metadata[section] = await api<ConnectionMetadata>(`/connections/${props.connection.id}/metadata/${section}`) }
  catch (cause: any) { metadataError.value = cause.message || t('stats.metadataLoadError') }
  finally { loadingMetadata.value = false }
}
function selectSection(section: StatsSection) {
  activeSection.value = section
  filter.value = ''
  if (section === 'stats') loadStats()
  else loadMetadata(section)
}
function refresh() { loadMetadata() }

onMounted(loadStats)
watch(() => props.connection.id, () => {
  activeSection.value = 'stats'
  filter.value = ''
  stats.value = undefined
  statsError.value = ''
  metadataError.value = ''
  for (const key of Object.keys(metadata)) delete metadata[key as Exclude<StatsSection, 'stats'>]
  loadStats()
})
</script>

<template>
  <section class="scrollbar h-full overflow-auto">
    <div class="flex min-h-full w-full flex-col px-5 py-6 lg:px-7">
      <header class="flex flex-wrap items-start justify-between gap-4 border-b border-line pb-5">
        <div>
          <div class="flex items-center gap-2"><i class="h-2.5 w-2.5 rounded-full" :class="connection.status === 'connected' ? 'bg-emerald-500' : 'bg-muted'" /><h1 class="text-xl font-semibold">{{ t('stats.title', { name: connection.name }) }}</h1></div>
          <p class="mt-1 text-sm text-muted">{{ connection.host }}:{{ connection.port }} · {{ connection.initialDatabase || t('stats.allDatabases') }}</p>
        </div>
        <button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-60" :disabled="loadingStats || loadingMetadata" @click="refresh">{{ loadingStats || loadingMetadata ? t('stats.refreshing') : t('stats.refresh') }}</button>
      </header>

      <div class="mt-6 flex min-h-0 flex-1 flex-col gap-6 md:flex-row">
        <nav class="flex shrink-0 gap-1 overflow-x-auto border-b border-line pb-4 md:w-52 md:flex-col md:overflow-visible md:border-b-0 md:border-r md:pb-0 md:pr-5">
          <button v-for="section in sections" :key="section.id" type="button" class="settings-nav flex items-center gap-2 whitespace-nowrap" :class="activeSection === section.id ? 'settings-nav-active' : ''" @click="selectSection(section.id)"><Icon :name="section.icon" class="h-4 w-4 shrink-0" aria-hidden="true" />{{ t(section.label) }}</button>
        </nav>

        <div class="min-w-0 flex-1 pb-8">
          <template v-if="activeSection === 'stats'">
            <div><h2 class="text-base font-semibold">{{ t('stats.section.stats') }}</h2><p class="mt-1 text-sm text-muted">{{ t('stats.description') }}</p></div>
            <p v-if="statsError" class="mt-5 rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-600 dark:border-rose-900 dark:bg-rose-950/30">{{ statsError }}</p>
            <template v-else-if="stats">
              <div class="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                <div class="rounded-xl border border-line bg-panel p-4"><p class="text-xs font-medium uppercase tracking-wide text-muted">{{ t('stats.queries') }}</p><p class="mt-2 text-2xl font-semibold">{{ number(stats.totalQueries) }}</p><p class="mt-1 text-xs text-muted">{{ t('stats.completed', { rate: successRate }) }}</p></div>
                <div class="rounded-xl border border-line bg-panel p-4"><p class="text-xs font-medium uppercase tracking-wide text-muted">{{ t('stats.averageTime') }}</p><p class="mt-2 text-2xl font-semibold">{{ number(stats.averageExecutionMs) }} <span class="text-sm font-medium text-muted">ms</span></p><p class="mt-1 text-xs text-muted">{{ t('stats.affectedRows', { count: number(stats.affectedRows) }) }}</p></div>
                <div class="rounded-xl border border-line bg-panel p-4"><p class="text-xs font-medium uppercase tracking-wide text-muted">{{ t('stats.databasesTables') }}</p><p class="mt-2 text-2xl font-semibold">{{ stats.schema.available ? number(stats.schema.tables) : '—' }}</p><p class="mt-1 text-xs text-muted">{{ stats.schema.available ? t('stats.visibleTables', { count: number(stats.schema.databases) }) : t('stats.schemaUnavailable') }}</p></div>
                <div class="rounded-xl border border-line bg-panel p-4"><p class="text-xs font-medium uppercase tracking-wide text-muted">{{ t('stats.connections') }}</p><p class="mt-2 text-2xl font-semibold">{{ stats.activeConnectionCount }} <span class="text-sm font-medium text-muted">{{ t('stats.active') }}</span></p><p class="mt-1 text-xs text-muted">{{ t('stats.savedWorkspace', { count: stats.savedConnectionCount }) }}</p></div>
              </div>

              <div class="mt-5 grid gap-5 xl:grid-cols-2">
                <article class="rounded-xl border border-line bg-panel p-5"><div class="flex items-center justify-between"><div><h3 class="font-semibold">{{ t('stats.queryActivity') }}</h3><p class="mt-1 text-xs text-muted">{{ t('stats.last7Days') }}</p></div><div class="flex gap-3 text-xs text-muted"><span><i class="mr-1 inline-block h-2 w-2 rounded-sm bg-accent" />{{ t('stats.success') }}</span><span><i class="mr-1 inline-block h-2 w-2 rounded-sm bg-rose-400" />{{ t('stats.error') }}</span></div></div><div class="mt-6 flex h-44 items-end justify-between gap-2 border-b border-line px-2"><div v-for="day in stats.queriesByDay" :key="day.date" class="flex h-full flex-1 flex-col justify-end gap-0.5"><div class="rounded-t-sm bg-rose-400" :style="{ height: barHeight(day.failed) }" :title="t('stats.errors', { count: day.failed })" /><div class="rounded-t-sm bg-accent" :style="{ height: barHeight(day.success) }" :title="t('stats.successes', { count: day.success })" /></div></div><div class="mt-2 flex justify-between px-2 text-[11px] text-muted"><span v-for="day in stats.queriesByDay" :key="`${day.date}-label`" class="flex-1 text-center">{{ dayLabel(day.date) }}</span></div></article>
                <article class="rounded-xl border border-line bg-panel p-5"><h3 class="font-semibold">{{ t('stats.mostUsed') }}</h3><p class="mt-1 text-xs text-muted">{{ t('stats.distribution') }}</p><div v-if="stats.queriesByOperation.length" class="mt-6 space-y-4"><div v-for="operation in stats.queriesByOperation" :key="operation.operation"><div class="mb-1 flex justify-between text-sm"><span class="font-mono text-xs">{{ operation.operation }}</span><span class="text-muted">{{ number(operation.count) }}</span></div><div class="h-2 overflow-hidden rounded-full bg-canvas"><div class="h-full rounded-full bg-accent" :style="{ width: operationWidth(operation.count) }" /></div></div></div><p v-else class="mt-10 text-center text-sm text-muted">{{ t('stats.noOperations') }}</p></article>
              </div>

              <div class="mt-5 grid gap-5 md:grid-cols-3"><article class="rounded-xl border border-line bg-panel p-4"><p class="text-xs font-medium uppercase tracking-wide text-muted">{{ t('stats.lastQuery') }}</p><p class="mt-2 text-sm font-medium">{{ lastQuery(stats.lastQueryAt) }}</p></article><article class="rounded-xl border border-line bg-panel p-4"><p class="text-xs font-medium uppercase tracking-wide text-muted">{{ t('stats.failures') }}</p><p class="mt-2 text-sm font-medium" :class="stats.failedQueries ? 'text-rose-500' : 'text-emerald-600'">{{ t('stats.ofQueries', { failed: number(stats.failedQueries), total: number(stats.totalQueries) }) }}</p></article><article class="rounded-xl border border-line bg-panel p-4"><p class="text-xs font-medium uppercase tracking-wide text-muted">{{ t('stats.security') }}</p><p class="mt-2 text-sm font-medium">{{ connection.sslEnabled ? t('stats.sslEnabled') : t('stats.sslDisabled') }}</p></article></div>
              <p v-if="stats.schema.error" class="mt-3 text-xs text-muted">{{ t('stats.schemaReadError', { error: stats.schema.error }) }}</p>
            </template>
            <div v-else-if="loadingStats" class="grid h-64 place-items-center text-sm text-muted">{{ t('stats.loading') }}</div>
          </template>

          <template v-else>
            <div class="flex flex-wrap items-end justify-between gap-4"><div><h2 class="text-base font-semibold">{{ t(activeSectionDetails.label) }}</h2><p class="mt-1 text-sm text-muted">{{ t(activeSectionDetails.description) }}</p></div><label class="grid gap-1 text-xs font-medium text-muted"><span>{{ t('stats.filter') }}</span><input v-model="filter" class="field h-9 w-56" :placeholder="t('stats.filterPlaceholder')" ></label></div>
            <p v-if="metadataError" class="mt-5 rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-600 dark:border-rose-900 dark:bg-rose-950/30">{{ metadataError }}</p>
            <div v-else-if="loadingMetadata && !activeMetadata" class="grid h-64 place-items-center text-sm text-muted">{{ t('stats.loadingMetadata') }}</div>
            <div v-else-if="activeMetadata" class="mt-5 overflow-hidden rounded-lg border border-line bg-panel"><div class="scrollbar overflow-auto"><table class="min-w-full text-left text-sm"><thead class="sticky top-0 bg-panel text-xs uppercase tracking-wide text-muted"><tr><th v-for="column in activeMetadata.columns" :key="column" class="border-b border-line px-4 py-3 font-medium">{{ column }}</th></tr></thead><tbody><tr v-for="(row, index) in filteredRows" :key="index" class="border-b border-line last:border-b-0 hover:bg-canvas"><td v-for="(value, columnIndex) in row" :key="columnIndex" class="max-w-[36rem] whitespace-pre-wrap break-words px-4 py-2.5 font-mono text-xs">{{ value || '—' }}</td></tr></tbody></table></div><p v-if="!filteredRows.length" class="px-4 py-8 text-center text-sm text-muted">{{ filter ? t('stats.noMatches') : t('table.empty') }}</p></div>
          </template>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.settings-nav { @apply rounded-md px-3 py-2 text-left text-sm text-muted hover:bg-canvas hover:text-ink; }
.settings-nav-active { @apply bg-accent/10 font-medium text-accent hover:bg-accent/10 hover:text-accent; }
</style>
