<script setup lang="ts">
import type { Connection, DatabaseInfo, TableInfo } from '~/types/database'

const props = defineProps<{ connections: Connection[]; activeConnectionId?: string; width?: number }>()
const emit = defineEmits<{ choose: [id: string]; table: [connection: Connection, database: string, table: string, section?: 'data' | 'structure' | 'tools']; database: [connection: Connection, database: string, section?: 'tables' | 'diagram' | 'tools']; connectionHome: [connection: Connection]; newQuery: [connection: Connection, database?: string, table?: string]; createDatabase: [connection: Connection]; edit: [connection: Connection]; stats: [connection: Connection]; add: []; home: []; saved: []; smart: []; history: []; settings: [] }>()
const api = useApi()
const workspace = useWorkspaceStore()
const { t } = useI18n()
const search = ref('')
const databases = reactive<Record<string, DatabaseInfo[]>>({})
const tables = reactive<Record<string, TableInfo[]>>({})
const expanded = reactive(new Set<string>())
const loading = reactive(new Set<string>())
const toast = ref('')
let toastTimeout: ReturnType<typeof setTimeout> | undefined
const filteredConnections = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (!query) return props.connections
  return props.connections.filter((connection) => {
    if (connection.name.toLowerCase().includes(query)) return true
    return Object.entries(tables).some(([key, list]) => key.startsWith(`d:${connection.id}:`) && list.some((table) => table.name.toLowerCase().includes(query)))
  })
})
const searchResults = computed(() => {
  const query = search.value.trim().toLowerCase()
  if (!query) return []
  const results: { connection: Connection; database: string; table: string }[] = []
  for (const connection of props.connections) {
    for (const [key, list] of Object.entries(tables)) {
      if (!key.startsWith(`d:${connection.id}:`)) continue
      const database = key.slice(`d:${connection.id}:`.length)
      for (const table of list) if (table.name.toLowerCase().includes(query)) results.push({ connection, database, table: table.name })
    }
  }
  return results
})
function visibleTables(connectionId: string, database: string) {
  const list = tables[`d:${connectionId}:${database}`] ?? []
  const query = search.value.trim().toLowerCase()
  if (!query) return list
  return list.filter((table) => table.name.toLowerCase().includes(query))
}

async function ensureDatabasesForSearch(connection: Connection) {
  const key = `c:${connection.id}`
  if (databases[connection.id] || loading.has(key)) return
  loading.add(key)
  try { databases[connection.id] = await api<DatabaseInfo[]>(`/connections/${connection.id}/databases`) }
  catch { /* background prefetch, surfaced only if the user expands the connection */ }
  finally { loading.delete(key) }
}
async function ensureTablesForSearch(connection: Connection, database: string) {
  const key = `d:${connection.id}:${database}`
  if (tables[key] || loading.has(key)) return
  loading.add(key)
  try { tables[key] = await api<TableInfo[]>(`/connections/${connection.id}/databases/${encodeURIComponent(database)}/tables`) }
  catch { /* background prefetch, surfaced only if the user expands the database */ }
  finally { loading.delete(key) }
}
async function ensureSearchDataLoaded() {
  await Promise.all(props.connections.filter((connection) => connection.status === 'connected').map(async (connection) => {
    await ensureDatabasesForSearch(connection)
    await Promise.all((databases[connection.id] ?? []).map((database) => ensureTablesForSearch(connection, database.name)))
  }))
}
watch(search, (value) => { if (value.trim()) ensureSearchDataLoaded() })

function showError(cause: unknown) {
  toast.value = cause instanceof Error ? cause.message : t('tree.connectionError')
  if (toastTimeout) clearTimeout(toastTimeout)
  toastTimeout = setTimeout(() => { toast.value = '' }, 5000)
}
onBeforeUnmount(() => { if (toastTimeout) clearTimeout(toastTimeout) })

async function loadDatabases(connection: Connection) {
  const key = `c:${connection.id}`
  loading.add(key)
  try { databases[connection.id] = await api<DatabaseInfo[]>(`/connections/${connection.id}/databases`) }
  catch (cause: unknown) { showError(cause) }
  finally { loading.delete(key) }
}
async function toggleConnection(connection: Connection) {
  emit('choose', connection.id)
  if (connection.status !== 'connected') return
  const key = `c:${connection.id}`
  if (expanded.has(key)) { expanded.delete(key); return }
  expanded.add(key)
  if (!databases[connection.id]) await loadDatabases(connection)
}
function clearConnectionCache(connectionId: string) {
  delete databases[connectionId]
  expanded.delete(`c:${connectionId}`)
  for (const key of Object.keys(tables)) if (key.startsWith(`d:${connectionId}:`)) tables[key] = []
  for (const key of [...expanded]) if (key.startsWith(`d:${connectionId}:`)) expanded.delete(key)
}
async function refreshConnection(connectionId: string, expand = false) {
  const connection = props.connections.find((item) => item.id === connectionId)
  if (!connection || connection.status !== 'connected') return
  if (expand) expanded.add(`c:${connectionId}`)
  for (const key of Object.keys(tables)) if (key.startsWith(`d:${connectionId}:`)) delete tables[key]
  await loadDatabases(connection)
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
function handleConnectionDoubleClick(connection: Connection) {
  if (connection.status === 'connected') { emit('connectionHome', connection); return }
  return connectConnection(connection)
}
const lastConnectionStatus = reactive<Record<string, Connection['status']>>({})
watch(() => props.connections.map((connection) => ({ id: connection.id, status: connection.status })), (list) => {
  for (const { id, status } of list) {
    if (lastConnectionStatus[id] === 'connected' && status !== 'connected') clearConnectionCache(id)
    lastConnectionStatus[id] = status
  }
}, { immediate: true, deep: true })
type MenuAction = { key: string; icon: string; label: string; disabled?: boolean; run: () => unknown }
const menu = ref<{ title: string; actions: MenuAction[]; x: number; y: number }>()
function connectionMenuActions(connection: Connection): MenuAction[] {
  const connected = connection.status === 'connected'
  return [
    { key: 'newQuery', icon: 'lucide:file-plus-2', label: t('home.newQuery'), run: () => emit('newQuery', connection) },
    connected
      ? { key: 'disconnect', icon: 'lucide:unplug', label: t('tree.disconnect'), run: () => disconnectConnection(connection) }
      : { key: 'connect', icon: 'lucide:plug', label: t('connectionHome.connect'), run: () => connectConnection(connection) },
      { key: 'revalidate', icon: 'lucide:refresh-cw', label: t('tree.revalidate'), disabled: !connected, run: () => revalidateConnection(connection) },
      { key: 'edit', icon: 'lucide:pencil', label: t('tree.edit'), run: () => emit('edit', connection) },
      { key: 'createDatabase', icon: 'lucide:database', label: t('connectionHome.createDatabase'), disabled: !connected, run: () => emit('createDatabase', connection) },
    { key: 'stats', icon: 'lucide:chart-column', label: t('stats.label'), run: () => emit('stats', connection) },
  ]
}
function databaseMenuActions(connection: Connection, database: string): MenuAction[] {
  return [
    { key: 'newQuery', icon: 'lucide:file-plus-2', label: t('home.newQuery'), run: () => emit('newQuery', connection, database) },
    { key: 'tables', icon: 'lucide:table-2', label: t('database.viewTables'), run: () => emit('database', connection, database, 'tables') },
    { key: 'diagram', icon: 'lucide:git-fork', label: t('database.viewDiagram'), run: () => emit('database', connection, database, 'diagram') },
    { key: 'tools', icon: 'lucide:wrench', label: t('database.viewTools'), run: () => emit('database', connection, database, 'tools') },
  ]
}
function tableMenuActions(connection: Connection, database: string, table: string): MenuAction[] {
  return [
    { key: 'newQuery', icon: 'lucide:file-plus-2', label: t('home.newQuery'), run: () => emit('newQuery', connection, database, table) },
    { key: 'data', icon: 'lucide:table-2', label: t('table.data'), run: () => emit('table', connection, database, table, 'data') },
    { key: 'structure', icon: 'lucide:columns-3', label: t('table.structure'), run: () => emit('table', connection, database, table, 'structure') },
    { key: 'tools', icon: 'lucide:wrench', label: t('table.tools.title'), run: () => emit('table', connection, database, table, 'tools') },
  ]
}
function openMenu(event: MouseEvent, title: string, actions: MenuAction[]) {
  const width = 200
  const height = actions.length * 30 + 34
  menu.value = { title, actions, x: Math.min(event.clientX, window.innerWidth - width - 8), y: Math.min(event.clientY, window.innerHeight - height - 8) }
}
function openConnectionMenu(event: MouseEvent, connection: Connection) {
  emit('choose', connection.id)
  openMenu(event, connection.name, connectionMenuActions(connection))
}
function openDatabaseMenu(event: MouseEvent, connection: Connection, database: string) {
  emit('choose', connection.id)
  openMenu(event, database, databaseMenuActions(connection, database))
}
function openTableMenu(event: MouseEvent, connection: Connection, database: string, table: string) {
  emit('choose', connection.id)
  openMenu(event, `${database}.${table}`, tableMenuActions(connection, database, table))
}
function closeMenu() { menu.value = undefined }
function runMenuAction(action: { disabled?: boolean; run: () => unknown }) {
  closeMenu()
  if (!action.disabled) void action.run()
}
async function disconnectConnection(connection: Connection) {
  const key = `c:${connection.id}`
  loading.add(key)
  try { await workspace.disconnectConnection(connection.id) }
  catch (cause: unknown) { showError(cause) }
  finally { loading.delete(key) }
}
async function revalidateConnection(connection: Connection) {
  const key = `c:${connection.id}`
  loading.add(key)
  try { await workspace.revalidateConnection(connection.id) }
  catch (cause: unknown) { showError(cause) }
  finally { loading.delete(key) }
  await refreshConnection(connection.id)
}
function handleMenuKeydown(event: KeyboardEvent) { if (event.key === 'Escape') closeMenu() }
onMounted(() => { window.addEventListener('keydown', handleMenuKeydown, true); window.addEventListener('resize', closeMenu) })
onBeforeUnmount(() => { window.removeEventListener('keydown', handleMenuKeydown, true); window.removeEventListener('resize', closeMenu) })

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
defineExpose({ refreshConnection })
</script>

<template>
  <aside class="flex h-full shrink-0 flex-col border-r border-line bg-panel" :style="{ width: `${width ?? 288}px` }">
    <div class="flex shrink-0 items-center justify-between border-b border-line px-3 py-3"><div class="flex items-center gap-2"><img class="h-8 w-8 rounded-lg border border-line bg-white object-contain p-0.5" src="/branding/favicon/android-chrome-192x192.png" alt="DBfock" ><span class="font-semibold tracking-tight">DBfock</span></div><div class="flex items-center gap-1"><button class="focus-ring grid h-9 w-9 place-items-center rounded-md text-muted hover:bg-canvas hover:text-ink" :title="t('tree.home')" :aria-label="t('tree.home')" @click="$emit('home')"><Icon name="lucide:house" class="h-4 w-4" aria-hidden="true" /></button><button class="focus-ring grid h-9 w-9 place-items-center rounded-md text-muted hover:bg-canvas hover:text-ink" :title="t('tree.savedQueries')" :aria-label="t('tree.savedQueries')" @click="$emit('saved')"><Icon name="lucide:bookmark" class="h-4 w-4" aria-hidden="true" /></button><button class="focus-ring grid h-9 w-9 place-items-center rounded-md text-muted hover:bg-canvas hover:text-ink" :title="t('tree.queryHistory')" :aria-label="t('tree.queryHistory')" @click="$emit('history')"><Icon name="lucide:history" class="h-4 w-4" aria-hidden="true" /></button><button class="focus-ring grid h-9 w-9 place-items-center rounded-md text-muted hover:bg-canvas hover:text-ink" :title="t('tree.smartQueries')" :aria-label="t('tree.smartQueries')" @click="$emit('smart')"><Icon name="lucide:sparkles" class="h-4 w-4" aria-hidden="true" /></button><button class="focus-ring grid h-9 w-9 place-items-center rounded-md text-muted hover:bg-canvas hover:text-ink" :title="t('tree.settings')" :aria-label="t('tree.settings')" @click="$emit('settings')"><Icon name="lucide:settings-2" class="h-4 w-4" aria-hidden="true" /></button></div></div>
    <div class="border-b border-line p-3"><div class="relative"><input v-model="search" class="focus-ring h-8 w-full min-w-0 rounded-md border border-line bg-canvas px-2 pr-7 text-sm" :placeholder="t('tree.search')"><button v-if="search" class="focus-ring absolute right-1 top-1/2 grid h-5 w-5 -translate-y-1/2 place-items-center rounded text-muted hover:bg-line hover:text-ink" :title="t('tree.clearSearch')" :aria-label="t('tree.clearSearch')" @click="search = ''"><Icon name="lucide:x" class="h-3.5 w-3.5" aria-hidden="true" /></button></div></div>
    <div class="scrollbar flex-1 overflow-auto px-2 py-3">
      <div v-if="search.trim()" class="mb-3">
        <p class="mb-2 px-2 text-[11px] font-semibold uppercase tracking-wider text-muted">{{ t('tree.searchResults') }}</p>
        <button v-for="result in searchResults" :key="`${result.connection.id}:${result.database}:${result.table}`" class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-canvas" @click="$emit('table', result.connection, result.database, result.table)" @contextmenu.prevent.stop="openTableMenu($event, result.connection, result.database, result.table)">
          <Icon name="lucide:table-2" class="h-3.5 w-3.5 shrink-0 text-muted" aria-hidden="true" />
          <span class="min-w-0 flex-1 truncate">{{ result.table }}</span>
          <span class="shrink-0 truncate text-xs text-muted">{{ result.database }} · {{ result.connection.name }}</span>
        </button>
        <p v-if="!searchResults.length" class="px-2 py-2 text-xs text-muted">{{ t('tree.noSearchResults') }}</p>
      </div>
      <div class="mb-2 flex items-center justify-between px-2"><p class="text-[11px] font-semibold uppercase tracking-wider text-muted">{{ t('tree.connections') }}</p><button class="grid rounded p-1.5 text-muted hover:bg-canvas hover:text-ink" :title="t('tree.newConnection')" :aria-label="t('tree.newConnection')" @click="$emit('add')"><Icon name="lucide:plus" class="h-4 w-4" aria-hidden="true" /></button></div>
      <div v-for="connection in filteredConnections" :key="connection.id">
        <div class="group relative flex items-center gap-1 rounded-md px-1 py-1 hover:bg-canvas" :class="activeConnectionId === connection.id ? 'bg-canvas' : ''" @contextmenu.prevent.stop="openConnectionMenu($event, connection)">
          <button type="button" class="grid w-5 place-items-center text-muted disabled:opacity-40" :disabled="connection.status !== 'connected'" @click.stop="toggleConnection(connection)"><Icon :name="expanded.has(`c:${connection.id}`) ? 'lucide:chevron-down' : 'lucide:chevron-right'" class="h-3.5 w-3.5" aria-hidden="true" /></button>
          <button type="button" class="flex min-w-0 flex-1 items-center gap-2 text-left text-sm" @click="emit('choose', connection.id)" @dblclick="handleConnectionDoubleClick(connection)"><i class="h-2.5 w-2.5 rounded-full ring-2 ring-panel" :style="{ backgroundColor: connection.color }" /><i class="h-1.5 w-1.5 rounded-full" :class="connection.status === 'connected' ? 'bg-emerald-500' : 'bg-muted'" /><span class="truncate">{{ connection.name }}</span><span v-if="connection.environment === 'production'" class="rounded bg-amber-500/15 px-1 py-0.5 text-[9px] font-semibold uppercase text-amber-700 dark:text-amber-300">{{ t('connection.production') }}</span></button>
          <button type="button" class="grid h-6 w-6 shrink-0 place-items-center rounded text-muted opacity-0 hover:bg-line hover:text-ink group-hover:opacity-100" :title="t('home.newQuery')" :aria-label="t('home.newQuery')" @click.stop="$emit('newQuery', connection)"><Icon name="lucide:file-plus-2" class="h-3.5 w-3.5" aria-hidden="true" /></button>
          <button type="button" class="grid h-6 w-6 shrink-0 place-items-center rounded text-muted opacity-0 hover:bg-line hover:text-ink group-hover:opacity-100" :title="t('tree.viewConnection')" :aria-label="t('tree.viewConnection')" @click.stop="$emit('connectionHome', connection)"><Icon name="lucide:eye" class="h-3.5 w-3.5" aria-hidden="true" /></button>
        </div>
        <div v-if="expanded.has(`c:${connection.id}`)" class="ml-3 border-l border-line pl-2"><p v-if="loading.has(`c:${connection.id}`)" class="px-2 py-1 text-xs text-muted">{{ t('tree.loadingDatabases') }}</p><template v-for="database in databases[connection.id]" :key="database.name"><div class="group flex items-center gap-1 rounded px-1 py-1 hover:bg-canvas" @contextmenu.prevent.stop="openDatabaseMenu($event, connection, database.name)"><button type="button" class="grid w-5 place-items-center text-muted" @click="toggleDatabase(connection,database.name)"><Icon :name="expanded.has(`d:${connection.id}:${database.name}`) ? 'lucide:chevron-down' : 'lucide:chevron-right'" class="h-3.5 w-3.5" aria-hidden="true" /></button><button type="button" class="flex min-w-0 flex-1 items-center gap-1.5 truncate text-left text-sm" @dblclick="$emit('database', connection, database.name)"><Icon name="lucide:database" class="h-3.5 w-3.5 shrink-0 text-muted" aria-hidden="true" />{{ database.name }}</button><button type="button" class="grid h-5 w-5 shrink-0 place-items-center rounded text-muted opacity-0 hover:bg-line hover:text-ink group-hover:opacity-100" :title="t('tree.viewDatabase')" :aria-label="t('tree.viewDatabase')" @click.stop="$emit('database', connection, database.name)"><Icon name="lucide:eye" class="h-3.5 w-3.5" aria-hidden="true" /></button></div><div v-if="expanded.has(`d:${connection.id}:${database.name}`)" class="ml-3 border-l border-line pl-2"><p v-if="loading.has(`d:${connection.id}:${database.name}`)" class="px-2 py-1 text-xs text-muted">{{ t('tree.loadingTables') }}</p><div v-for="table in visibleTables(connection.id, database.name)" :key="table.name" class="group flex items-center gap-1 rounded px-1 py-1 text-sm text-muted hover:bg-canvas hover:text-ink" @contextmenu.prevent.stop="openTableMenu($event, connection, database.name, table.name)"><button type="button" class="flex min-w-0 flex-1 items-center gap-2 truncate text-left" @dblclick="$emit('table', connection, database.name, table.name)"><Icon name="lucide:table-2" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />{{ table.name }}</button><button type="button" class="grid h-5 w-5 shrink-0 place-items-center rounded opacity-0 hover:bg-line hover:text-ink group-hover:opacity-100" :title="t('tree.viewTable')" :aria-label="t('tree.viewTable')" @click.stop="$emit('table', connection, database.name, table.name)"><Icon name="lucide:eye" class="h-3.5 w-3.5" aria-hidden="true" /></button></div></div></template></div>
      </div>
      <div v-if="!filteredConnections.length" class="whitespace-pre-line px-2 py-8 text-center text-sm text-muted">{{ t('tree.empty') }}</div>
    </div>
  </aside>
  <Teleport v-if="menu" to="body">
    <div class="fixed inset-0 z-50" @click="closeMenu" @contextmenu.prevent="closeMenu" @wheel="closeMenu">
      <div role="menu" :aria-label="t('tree.connectionActions')" class="absolute w-[200px] overflow-hidden rounded-lg border border-line bg-panel py-1 shadow-panel" :style="{ left: `${menu.x}px`, top: `${menu.y}px` }" @click.stop>
        <p class="truncate px-3 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted">{{ menu.title }}</p>
        <button v-for="action in menu.actions" :key="action.key" type="button" role="menuitem" class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-ink hover:bg-canvas disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent" :disabled="action.disabled" @click="runMenuAction(action)">
          <Icon :name="action.icon" class="h-3.5 w-3.5 shrink-0 text-muted" aria-hidden="true" />
          <span class="truncate">{{ action.label }}</span>
        </button>
      </div>
    </div>
  </Teleport>
  <div v-if="toast" role="alert" class="fixed bottom-4 right-4 z-50 flex max-w-sm items-start gap-3 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700 shadow-panel dark:border-rose-900 dark:bg-rose-950 dark:text-rose-200"><Icon name="lucide:circle-alert" class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" /><span class="min-w-0 flex-1">{{ toast }}</span><button class="-mr-1 -mt-1 rounded p-1 leading-none hover:bg-rose-100 dark:hover:bg-rose-900" :aria-label="t('common.close')" @click="toast = ''"><Icon name="lucide:x" class="h-4 w-4" aria-hidden="true" /></button></div>
</template>
