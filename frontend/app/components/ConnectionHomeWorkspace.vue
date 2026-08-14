<script setup lang="ts">
import type { Connection, DatabaseInfo } from '~/types/database'

type ConnectionSection = 'databases' | 'users' | 'tools'
const props = defineProps<{ connection: Connection }>()
const emit = defineEmits<{ edit: [connection: Connection]; newQuery: [connection: Connection]; stats: [connection: Connection]; database: [connection: Connection, database: string] }>()
const api = useApi()
const { saveTextFile } = useFileSave()
const { fetchDump, dumpFileName } = useDatabaseDump()
const { t } = useI18n()
const { error: notifyError, success: notifySuccess } = useToast()
const activeSection = ref<ConnectionSection>('databases')
const databases = ref<DatabaseInfo[]>()
const databasesError = ref('')
const loadingDatabases = ref(false)
const filter = ref('')
const filterInput = ref<HTMLInputElement>()
const isActive = ref(false)
const changing = ref(false)
const showCreateDatabase = ref(false)
const creatingDatabase = ref(false)
const selectedDumpDatabases = ref<string[]>([])
const dumping = ref(false)
const sections: { id: ConnectionSection; label: string; icon: string }[] = [
  { id: 'databases', label: 'connectionHome.databases', icon: 'lucide:database' },
  { id: 'users', label: 'connectionUsers.title', icon: 'lucide:users-round' },
  { id: 'tools', label: 'connectionHome.tools', icon: 'lucide:wrench' },
]

const filteredDatabases = computed(() => {
  const list = databases.value ?? []
  const query = filter.value.trim().toLowerCase()
  return query ? list.filter((database) => database.name.toLowerCase().includes(query)) : list
})
function errorMessage(cause: unknown) { return cause instanceof Error ? cause.message : '' }
async function loadDatabases() {
  if (props.connection.status !== 'connected') { databases.value = undefined; return }
  loadingDatabases.value = true
  databasesError.value = ''
  try {
    databases.value = await api<DatabaseInfo[]>(`/connections/${props.connection.id}/databases`)
    if (isActive.value) focusDatabaseFilterInput()
  }
  catch (cause: unknown) { databasesError.value = errorMessage(cause) || t('tree.connectionError') }
  finally { loadingDatabases.value = false }
}
async function connect() { changing.value = true; try { await useWorkspaceStore().connectConnection(props.connection.id) } catch (cause: unknown) { notifyError(errorMessage(cause)) } finally { changing.value = false } }
async function disconnect() { changing.value = true; try { await useWorkspaceStore().disconnectConnection(props.connection.id) } catch (cause: unknown) { notifyError(errorMessage(cause)) } finally { changing.value = false } }
async function revalidate() { changing.value = true; try { await useWorkspaceStore().revalidateConnection(props.connection.id) } catch (cause: unknown) { notifyError(errorMessage(cause)) } finally { changing.value = false } }
function quote(name: string) { return `\`${name.replaceAll('`', '``')}\`` }
async function createDatabase(rawName: string) {
  const name = rawName.trim()
  if (!name || creatingDatabase.value) return
  creatingDatabase.value = true
  try {
    await api(`/connections/${props.connection.id}/query`, { method: 'POST', body: { sql: `CREATE DATABASE ${quote(name)}`, historySql: `Create database ${name}` } })
    showCreateDatabase.value = false
    notifySuccess(t('connectionHome.databaseCreated', { name }))
    await loadDatabases()
    emit('database', props.connection, name)
  } catch (cause: unknown) { notifyError(errorMessage(cause)) } finally { creatingDatabase.value = false }
}
async function createDump() {
  if (!selectedDumpDatabases.value.length || dumping.value) return
  dumping.value = true
  try {
    for (const database of selectedDumpDatabases.value) {
      const script = await fetchDump(props.connection.id, database)
      if (!await saveTextFile(dumpFileName(database), script, 'application/sql;charset=utf-8')) return
    }
    notifySuccess(t('connectionHome.dumpCreated', { count: selectedDumpDatabases.value.length }))
  } catch (cause: unknown) { notifyError(errorMessage(cause)) } finally { dumping.value = false }
}

function focusDatabaseFilterInput() {
  if (activeSection.value !== 'databases') return
  nextTick(() => filterInput.value?.focus())
}
function focusDatabaseFilter(event: KeyboardEvent) {
  if (!(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== 'f' || activeSection.value !== 'databases' || !filterInput.value) return
  event.preventDefault()
  focusDatabaseFilterInput()
  filterInput.value.select()
}

onMounted(() => {
  loadDatabases()
})
onActivated(() => { isActive.value = true; window.addEventListener('keydown', focusDatabaseFilter); focusDatabaseFilterInput() })
onDeactivated(() => { isActive.value = false; window.removeEventListener('keydown', focusDatabaseFilter) })
onBeforeUnmount(() => window.removeEventListener('keydown', focusDatabaseFilter))
watch(() => props.connection.id, () => { activeSection.value = 'databases'; filter.value = ''; loadDatabases() })
watch(() => props.connection.status, () => loadDatabases())
watch(activeSection, (section) => { if (isActive.value && section === 'databases') focusDatabaseFilterInput() })
</script>

<template>
  <section class="scrollbar h-full overflow-auto">
    <div class="flex min-h-full w-full flex-col px-5 py-6 lg:px-7">
      <header class="flex flex-wrap items-start justify-between gap-4 border-b border-line pb-5"><div><div class="flex items-center gap-2"><i class="h-3 w-3 shrink-0 rounded-full ring-2 ring-panel" :style="{ backgroundColor: connection.color }" /><h1 class="text-xl font-semibold">{{ connection.name }}</h1><span v-if="connection.environment === 'production'" class="rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-amber-700 dark:text-amber-300">{{ t('connection.production') }}</span></div><p class="mt-1 flex flex-wrap items-center gap-1.5 text-sm text-muted"><i class="h-1.5 w-1.5 rounded-full" :class="connection.status === 'connected' ? 'bg-emerald-500' : 'bg-muted'" /><span>{{ connection.status === 'connected' ? t('connectionHome.connected') : t('connectionHome.disconnected') }}</span><span>· {{ connection.host }}:{{ connection.port }} · {{ connection.username }}</span><span v-if="connection.initialDatabase">· {{ connection.initialDatabase }}</span></p></div><div class="flex flex-wrap gap-2"><button v-if="connection.status === 'connected'" type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-60" :disabled="changing" @click="disconnect">{{ t('tree.disconnect') }}</button><button v-else type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-60" :disabled="changing" @click="connect">{{ t('connectionHome.connect') }}</button><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-60" :disabled="changing" @click="revalidate">{{ t('tree.revalidate') }}</button><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas" @click="emit('edit', connection)">{{ t('tree.edit') }}</button><button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas" @click="emit('stats', connection)">{{ t('stats.label') }}</button><button type="button" class="rounded-md bg-accent px-3 py-2 text-sm font-medium text-white" @click="emit('newQuery', connection)">{{ t('home.newQuery') }}</button></div></header>

      <div class="mt-6 flex min-h-0 flex-1 flex-col gap-6 md:flex-row"><nav class="flex shrink-0 gap-1 overflow-x-auto border-b border-line pb-4 md:w-52 md:flex-col md:overflow-visible md:border-b-0 md:border-r md:pb-0 md:pr-5"><button v-for="section in sections" :key="section.id" type="button" class="connection-nav flex items-center gap-2 whitespace-nowrap" :class="activeSection === section.id ? 'connection-nav-active' : ''" @click="activeSection = section.id"><Icon :name="section.icon" class="h-4 w-4 shrink-0" aria-hidden="true" />{{ t(section.label) }}</button></nav>
        <div class="min-w-0 flex-1 pb-8"><template v-if="activeSection === 'databases'"><div class="flex flex-wrap items-end justify-between gap-4"><div><h2 class="text-base font-semibold">{{ t('connectionHome.databases') }}</h2><p class="mt-1 text-sm text-muted">{{ t('connectionHome.databasesDescription') }}</p></div><div class="flex flex-1 flex-wrap items-center justify-end gap-2"><label v-if="databases?.length" class="flex min-w-56 flex-1 items-center gap-2 rounded-lg border border-line bg-panel px-3 py-2 text-muted shadow-sm sm:max-w-xs"><Icon name="lucide:search" class="h-4 w-4 shrink-0" aria-hidden="true" /><span class="sr-only">{{ t('stats.filter') }}</span><input ref="filterInput" v-model="filter" class="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted" :placeholder="t('stats.filterPlaceholder')"></label><button v-if="connection.status === 'connected'" type="button" class="flex shrink-0 items-center gap-1.5 rounded-md bg-accent px-3 py-2 text-sm font-medium text-white disabled:opacity-50" :disabled="creatingDatabase" @click="showCreateDatabase = true"><Icon name="lucide:plus" class="h-4 w-4" aria-hidden="true" />{{ creatingDatabase ? t('connectionHome.creatingDatabase') : t('connectionHome.createDatabase') }}</button></div></div><p v-if="connection.status !== 'connected'" class="mt-4 rounded-md border border-line bg-canvas px-3 py-2 text-sm text-muted">{{ t('connectionHome.connectPrompt') }}</p><p v-else-if="databasesError" class="mt-4 rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-600 dark:border-rose-900 dark:bg-rose-950/30">{{ databasesError }}</p><div v-else-if="loadingDatabases && !databases" class="grid h-24 place-items-center text-sm text-muted">{{ t('tree.loadingDatabases') }}</div><template v-else><div v-if="filteredDatabases.length" class="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3"><button v-for="database in filteredDatabases" :key="database.name" type="button" class="group flex items-center justify-between gap-2 rounded-lg border border-line bg-panel p-3 text-left hover:border-accent/40 hover:bg-canvas" @click="emit('database', connection, database.name)"><span class="flex min-w-0 items-center gap-2 text-sm font-medium"><Icon name="lucide:database" class="h-4 w-4 shrink-0 text-muted" aria-hidden="true" /><span class="truncate">{{ database.name }}</span></span><Icon name="lucide:arrow-right" class="h-3.5 w-3.5 shrink-0 text-muted opacity-0 group-hover:opacity-100" aria-hidden="true" /></button></div><p v-else class="mt-4 rounded-md border border-line bg-canvas px-3 py-8 text-center text-sm text-muted">{{ t('connectionHome.noDatabases') }}</p></template></template><ConnectionUsersWorkspace v-else-if="activeSection === 'users'" :connection="connection" :databases="databases || []" />
            <template v-else>
              <div><h2 class="text-base font-semibold">{{ t('connectionHome.tools') }}</h2><p class="mt-1 text-sm text-muted">{{ t('connectionHome.toolsDescription') }}</p></div>
              <p v-if="connection.status !== 'connected'" class="mt-4 rounded-md border border-line bg-canvas px-3 py-2 text-sm text-muted">{{ t('connectionHome.toolsConnectPrompt') }}</p>
              <div v-else class="mt-5 max-w-2xl">
                <section class="rounded-lg border border-line bg-panel p-4">
                  <h3 class="text-sm font-semibold">{{ t('connectionHome.dumpTitle') }}</h3>
                  <p class="mt-1 text-sm text-muted">{{ t('connectionHome.dumpDescription') }}</p>
                  <label class="mt-4 grid gap-1.5 text-sm font-medium">{{ t('connectionHome.dumpDatabases') }}<AppMultiSelect v-model="selectedDumpDatabases" :options="(databases || []).map((database) => ({ value: database.name, label: database.name }))" :placeholder="t('connectionHome.dumpDatabasesPlaceholder')" /></label>
                  <button type="button" class="mt-4 flex items-center gap-1.5 rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-50" :disabled="dumping || !selectedDumpDatabases.length" @click="createDump"><Icon name="lucide:download" class="h-4 w-4" aria-hidden="true" />{{ dumping ? t('connectionHome.creatingDump') : t('connectionHome.createDump') }}</button>
                </section>
              </div>
            </template>
          </div>
      </div>
    </div>
    <QuerySaveDialog v-model="showCreateDatabase" initial-value="" :title="t('connectionHome.createDatabase')" :description="t('connectionHome.createDatabaseDescription')" :label="t('connectionHome.databaseName')" :confirm-label="creatingDatabase ? t('connectionHome.creatingDatabase') : t('connectionHome.createDatabase')" :cancel-label="t('connection.cancel')" @confirm="createDatabase" />
  </section>
</template>

<style scoped>
.connection-nav { @apply rounded-md px-3 py-2 text-left text-sm text-muted hover:bg-canvas hover:text-ink; }
.connection-nav-active { @apply bg-accent/10 font-medium text-accent hover:bg-accent/10 hover:text-accent; }
</style>
