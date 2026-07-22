<script setup lang="ts">
import type { Connection, DatabaseInfo } from '~/types/database'

const props = defineProps<{ connection: Connection }>()
const emit = defineEmits<{ edit: [connection: Connection]; newQuery: [connection: Connection]; stats: [connection: Connection]; database: [connection: Connection, database: string] }>()
const api = useApi()
const { t } = useI18n()
const { error: notifyError } = useToast()
const databases = ref<DatabaseInfo[]>()
const databasesError = ref('')
const loadingDatabases = ref(false)
const filter = ref('')
const changing = ref(false)

const filteredDatabases = computed(() => {
  const list = databases.value ?? []
  const query = filter.value.trim().toLowerCase()
  return query ? list.filter((database) => database.name.toLowerCase().includes(query)) : list
})

async function loadDatabases() {
  if (props.connection.status !== 'connected') { databases.value = undefined; return }
  loadingDatabases.value = true
  databasesError.value = ''
  try { databases.value = await api<DatabaseInfo[]>(`/connections/${props.connection.id}/databases`) }
  catch (cause: any) { databasesError.value = cause.message || t('tree.connectionError') }
  finally { loadingDatabases.value = false }
}

async function connect() {
  changing.value = true
  try { await useWorkspaceStore().connectConnection(props.connection.id) }
  catch (cause: any) { notifyError(cause.message) }
  finally { changing.value = false }
}
async function disconnect() {
  changing.value = true
  try { await useWorkspaceStore().disconnectConnection(props.connection.id) }
  catch (cause: any) { notifyError(cause.message) }
  finally { changing.value = false }
}
async function revalidate() {
  changing.value = true
  try { await useWorkspaceStore().revalidateConnection(props.connection.id) }
  catch (cause: any) { notifyError(cause.message) }
  finally { changing.value = false }
}

onMounted(loadDatabases)
watch(() => props.connection.id, () => { filter.value = ''; loadDatabases() })
watch(() => props.connection.status, () => loadDatabases())
</script>

<template>
  <section class="scrollbar h-full overflow-auto">
    <div class="flex min-h-full w-full flex-col px-5 py-6 lg:px-7">
      <header class="flex flex-wrap items-start justify-between gap-4 border-b border-line pb-5">
        <div>
          <div class="flex items-center gap-2">
            <i class="h-3 w-3 shrink-0 rounded-full ring-2 ring-panel" :style="{ backgroundColor: connection.color }" />
            <h1 class="text-xl font-semibold">{{ connection.name }}</h1>
            <span v-if="connection.environment === 'production'" class="rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-amber-700 dark:text-amber-300">{{ t('connection.production') }}</span>
          </div>
          <p class="mt-1 flex flex-wrap items-center gap-1.5 text-sm text-muted">
            <i class="h-1.5 w-1.5 rounded-full" :class="connection.status === 'connected' ? 'bg-emerald-500' : 'bg-muted'" />
            <span>{{ connection.status === 'connected' ? t('connectionHome.connected') : t('connectionHome.disconnected') }}</span>
            <span>· {{ connection.host }}:{{ connection.port }} · {{ connection.username }}</span>
            <span v-if="connection.initialDatabase">· {{ connection.initialDatabase }}</span>
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button v-if="connection.status === 'connected'" type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-60" :disabled="changing" @click="disconnect">{{ t('tree.disconnect') }}</button>
          <button v-else type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-60" :disabled="changing" @click="connect">{{ t('connectionHome.connect') }}</button>
          <button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-60" :disabled="changing" @click="revalidate">{{ t('tree.revalidate') }}</button>
          <button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas" @click="emit('edit', connection)">{{ t('tree.edit') }}</button>
          <button type="button" class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas" @click="emit('stats', connection)">{{ t('stats.label') }}</button>
          <button type="button" class="rounded-md bg-accent px-3 py-2 text-sm font-medium text-white" @click="emit('newQuery', connection)">{{ t('home.newQuery') }}</button>
        </div>
      </header>

      <div class="mt-5 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h2 class="text-base font-semibold">{{ t('connectionHome.databases') }}</h2>
          <p class="mt-1 text-sm text-muted">{{ t('connectionHome.databasesDescription') }}</p>
        </div>
        <label v-if="databases?.length" class="grid gap-1 text-xs font-medium text-muted"><span>{{ t('stats.filter') }}</span><input v-model="filter" class="field h-9 w-48" :placeholder="t('stats.filterPlaceholder')" /></label>
      </div>

      <p v-if="connection.status !== 'connected'" class="mt-4 rounded-md border border-line bg-canvas px-3 py-2 text-sm text-muted">{{ t('connectionHome.connectPrompt') }}</p>
      <p v-else-if="databasesError" class="mt-4 rounded-md border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-600 dark:border-rose-900 dark:bg-rose-950/30">{{ databasesError }}</p>
      <div v-else-if="loadingDatabases && !databases" class="grid h-24 place-items-center text-sm text-muted">{{ t('tree.loadingDatabases') }}</div>
      <template v-else>
        <div v-if="filteredDatabases.length" class="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <button v-for="database in filteredDatabases" :key="database.name" type="button" class="group flex items-center justify-between gap-2 rounded-lg border border-line bg-panel p-3 text-left hover:border-accent/40 hover:bg-canvas" @click="emit('database', connection, database.name)">
            <span class="flex min-w-0 items-center gap-2 text-sm font-medium"><Icon name="lucide:database" class="h-4 w-4 shrink-0 text-muted" aria-hidden="true" /><span class="truncate">{{ database.name }}</span></span>
            <Icon name="lucide:arrow-right" class="h-3.5 w-3.5 shrink-0 text-muted opacity-0 group-hover:opacity-100" aria-hidden="true" />
          </button>
        </div>
        <p v-else class="mt-4 rounded-md border border-line bg-canvas px-3 py-8 text-center text-sm text-muted">{{ t('connectionHome.noDatabases') }}</p>
      </template>
    </div>
  </section>
</template>
