<script setup lang="ts">
import type { Connection, DatabaseInfo, TableInfo } from '~/types/database'

const props = defineProps<{ connections: Connection[]; activeConnectionId?: string; width?: number }>()
const emit = defineEmits<{ choose: [id: string]; table: [connection: Connection, database: string, table: string]; edit: [connection: Connection]; stats: [connection: Connection]; newQuery: [connection: Connection]; refresh: []; add: []; home: []; saved: []; smart: []; settings: [] }>()
const api = useApi()
const workspace = useWorkspaceStore()
const { t } = useI18n()
const search = ref('')
const databases = reactive<Record<string, DatabaseInfo[]>>({})
const tables = reactive<Record<string, TableInfo[]>>({})
const expanded = reactive(new Set<string>())
const loading = reactive(new Set<string>())
const toast = ref('')
const actionMenuFor = ref<string>()
const changingConnectionId = ref<string>()
let toastTimeout: ReturnType<typeof setTimeout> | undefined
const filteredConnections = computed(() => props.connections.filter((item) => item.name.toLowerCase().includes(search.value.toLowerCase())))

function showError(cause: unknown) {
  toast.value = cause instanceof Error ? cause.message : t('tree.connectionError')
  if (toastTimeout) clearTimeout(toastTimeout)
  toastTimeout = setTimeout(() => { toast.value = '' }, 5000)
}
onBeforeUnmount(() => { if (toastTimeout) clearTimeout(toastTimeout) })

async function toggleConnection(connection: Connection) {
  emit('choose', connection.id)
  if (connection.status !== 'connected') return
  const key = `c:${connection.id}`
  if (expanded.has(key)) { expanded.delete(key); return }
  expanded.add(key)
  loading.add(key)
  try {
    if (!databases[connection.id]) databases[connection.id] = await api<DatabaseInfo[]>(`/connections/${connection.id}/databases`)
  } catch (cause: unknown) { showError(cause) }
  finally { loading.delete(key) }
}
async function connectConnection(connection: Connection) {
  emit('choose', connection.id)
  if (connection.status === 'connected' || loading.has(`c:${connection.id}`)) return
  const key = `c:${connection.id}`
  loading.add(key)
  try { await workspace.connectConnection(connection.id) }
  catch (cause: unknown) { showError(cause) }
  finally { loading.delete(key) }
}
async function disconnect(connection: Connection) {
  actionMenuFor.value = undefined
  changingConnectionId.value = connection.id
  try { await workspace.disconnectConnection(connection.id) }
  catch (cause: unknown) { showError(cause) }
  finally { changingConnectionId.value = undefined }
}
async function revalidate(connection: Connection) {
  actionMenuFor.value = undefined
  changingConnectionId.value = connection.id
  try { await workspace.revalidateConnection(connection.id) }
  catch (cause: unknown) { showError(cause) }
  finally { changingConnectionId.value = undefined }
}
async function toggleDatabase(connection: Connection, database: string) {
  const key = `d:${connection.id}:${database}`
  if (expanded.has(key)) { expanded.delete(key); return }
  expanded.add(key)
  if (!tables[key]) {
    loading.add(key)
    try { tables[key] = await api<TableInfo[]>(`/connections/${connection.id}/databases/${encodeURIComponent(database)}/tables`) }
    catch (cause: unknown) { showError(cause) }
    finally { loading.delete(key) }
  }
}
</script>

<template>
  <aside class="flex h-full shrink-0 flex-col border-r border-line bg-panel" :style="{ width: `${width ?? 288}px` }">
    <div class="flex shrink-0 items-center justify-between border-b border-line px-3 py-3"><div class="flex items-center gap-2"><img class="h-8 w-8 rounded-lg border border-line bg-white object-contain p-0.5" src="/branding/favicon/android-chrome-192x192.png" alt="DBfock" /><span class="font-semibold tracking-tight">DBfock</span></div><div class="flex items-center gap-1"><button class="focus-ring grid h-9 w-9 place-items-center rounded-md text-muted hover:bg-canvas hover:text-ink" :title="t('tree.home')" :aria-label="t('tree.home')" @click="$emit('home')"><Icon name="lucide:house" class="h-4 w-4" aria-hidden="true" /></button><button class="focus-ring grid h-9 w-9 place-items-center rounded-md text-muted hover:bg-canvas hover:text-ink" :title="t('tree.savedQueries')" :aria-label="t('tree.savedQueries')" @click="$emit('saved')"><Icon name="lucide:bookmark" class="h-4 w-4" aria-hidden="true" /></button><button class="focus-ring grid h-9 w-9 place-items-center rounded-md text-muted hover:bg-canvas hover:text-ink" :title="t('tree.smartQueries')" :aria-label="t('tree.smartQueries')" @click="$emit('smart')"><Icon name="lucide:sparkles" class="h-4 w-4" aria-hidden="true" /></button><button class="focus-ring grid h-9 w-9 place-items-center rounded-md text-muted hover:bg-canvas hover:text-ink" :title="t('tree.settings')" :aria-label="t('tree.settings')" @click="$emit('settings')"><Icon name="lucide:settings-2" class="h-4 w-4" aria-hidden="true" /></button></div></div>
    <div class="flex items-center gap-2 border-b border-line p-3"><input v-model="search" class="focus-ring h-8 min-w-0 flex-1 rounded-md border border-line bg-canvas px-2 text-sm" :placeholder="t('tree.searchConnections')"><button class="grid rounded p-1 text-muted hover:bg-canvas" :title="t('tree.refreshConnections')" :aria-label="t('tree.refreshConnections')" @click="$emit('refresh')"><Icon name="lucide:refresh-cw" class="h-4 w-4" aria-hidden="true" /></button></div>
    <div class="scrollbar flex-1 overflow-auto px-2 py-3">
      <div class="mb-2 flex items-center justify-between px-2"><p class="text-[11px] font-semibold uppercase tracking-wider text-muted">{{ t('tree.connections') }}</p><button class="grid rounded p-1.5 text-muted hover:bg-canvas hover:text-ink" :title="t('tree.newConnection')" :aria-label="t('tree.newConnection')" @click="$emit('add')"><Icon name="lucide:plus" class="h-4 w-4" aria-hidden="true" /></button></div>
      <div v-for="connection in filteredConnections" :key="connection.id">
        <div class="group relative flex items-center gap-1 rounded-md px-1 py-1 hover:bg-canvas" :class="activeConnectionId === connection.id ? 'bg-canvas' : ''">
          <button class="grid w-5 place-items-center text-muted disabled:opacity-40" :disabled="connection.status !== 'connected'" @click="toggleConnection(connection)"><Icon :name="expanded.has(`c:${connection.id}`) ? 'lucide:chevron-down' : 'lucide:chevron-right'" class="h-3.5 w-3.5" aria-hidden="true" /></button>
          <button class="flex min-w-0 flex-1 items-center gap-2 text-left text-sm" @click="toggleConnection(connection)" @dblclick="connectConnection(connection)"><i class="h-2.5 w-2.5 rounded-full ring-2 ring-panel" :style="{ backgroundColor: connection.color }" /><i class="h-1.5 w-1.5 rounded-full" :class="connection.status === 'connected' ? 'bg-emerald-500' : 'bg-muted'" /><span class="truncate">{{ connection.name }}</span><span v-if="connection.environment === 'production'" class="rounded bg-amber-500/15 px-1 py-0.5 text-[9px] font-semibold uppercase text-amber-700 dark:text-amber-300">{{ t('connection.production') }}</span></button>
          <button class="grid rounded p-1 text-muted hover:bg-line disabled:opacity-60" :aria-expanded="actionMenuFor === connection.id" :aria-label="t('tree.connectionActions')" :disabled="changingConnectionId === connection.id" @click.stop="actionMenuFor = actionMenuFor === connection.id ? undefined : connection.id"><Icon name="lucide:ellipsis" class="h-4 w-4" aria-hidden="true" /></button>
          <div v-if="actionMenuFor === connection.id" class="absolute right-0 top-8 z-20 w-40 rounded-md border border-line bg-panel p-1 shadow-panel"><button class="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-canvas" @click.stop="actionMenuFor = undefined; $emit('newQuery', connection)">{{ t('home.newQuery') }}</button><button class="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-canvas" @click.stop="actionMenuFor = undefined; $emit('edit', connection)">{{ t('tree.edit') }}</button><button class="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-canvas" @click.stop="$emit('stats', connection); actionMenuFor = undefined">{{ t('stats.label') }}</button><button v-if="connection.status === 'connected'" class="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-canvas" @click.stop="disconnect(connection)">{{ t('tree.disconnect') }}</button><button class="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-canvas" @click.stop="revalidate(connection)">{{ t('tree.revalidate') }}</button></div>
        </div>
        <div v-if="expanded.has(`c:${connection.id}`)" class="ml-3 border-l border-line pl-2"><p v-if="loading.has(`c:${connection.id}`)" class="px-2 py-1 text-xs text-muted">{{ t('tree.loadingDatabases') }}</p><template v-for="database in databases[connection.id]" :key="database.name"><div class="flex cursor-pointer items-center gap-1 rounded px-1 py-1 hover:bg-canvas" @click="toggleDatabase(connection,database.name)"><button class="grid w-5 place-items-center text-muted" @click.stop="toggleDatabase(connection,database.name)"><Icon :name="expanded.has(`d:${connection.id}:${database.name}`) ? 'lucide:chevron-down' : 'lucide:chevron-right'" class="h-3.5 w-3.5" aria-hidden="true" /></button><button class="flex min-w-0 items-center gap-1.5 truncate text-left text-sm"><Icon name="lucide:database" class="h-3.5 w-3.5 shrink-0 text-muted" aria-hidden="true" />{{ database.name }}</button></div><div v-if="expanded.has(`d:${connection.id}:${database.name}`)" class="ml-3 border-l border-line pl-2"><p v-if="loading.has(`d:${connection.id}:${database.name}`)" class="px-2 py-1 text-xs text-muted">{{ t('tree.loadingTables') }}</p><button v-for="table in tables[`d:${connection.id}:${database.name}`]" :key="table.name" class="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-sm text-muted hover:bg-canvas hover:text-ink" @click="$emit('table', connection, database.name, table.name)"><Icon name="lucide:table-2" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />{{ table.name }}</button></div></template></div>
      </div>
      <div v-if="!filteredConnections.length" class="whitespace-pre-line px-2 py-8 text-center text-sm text-muted">{{ t('tree.empty') }}</div>
    </div>
  </aside>
  <div v-if="toast" role="alert" class="fixed bottom-4 right-4 z-50 flex max-w-sm items-start gap-3 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700 shadow-panel dark:border-rose-900 dark:bg-rose-950 dark:text-rose-200"><Icon name="lucide:circle-alert" class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" /><span class="min-w-0 flex-1">{{ toast }}</span><button class="-mr-1 -mt-1 rounded p-1 leading-none hover:bg-rose-100 dark:hover:bg-rose-900" :aria-label="t('common.close')" @click="toast = ''"><Icon name="lucide:x" class="h-4 w-4" aria-hidden="true" /></button></div>
</template>
