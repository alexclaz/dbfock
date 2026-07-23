<script setup lang="ts">
import type { Connection, QueryHistory, QueryResult, SavedQuery, SmartQuery, TableStructure, TransactionStatus, WorkspaceTab } from '~/types/database'
import { queryResultAsCSV, queryResultAsJSON, queryResultAsTSV, queryResultEdits } from '~/utils/queryResult'

type EditableResultSource = { connectionId: string; database: string; table: string; columns: string[]; primaryKey: string[] }

type ResultTab = {
  id: string
  title: string
  result?: QueryResult
  view: 'table' | 'json' | 'csv'
  copied: boolean
  editing: boolean
  sources?: EditableResultSource[]
}
type SmartResultTab = ResultTab & { connectionId: string; smartQueryId: string }
type AISettings = { configured: boolean }
type PendingSave = { tab: WorkspaceTab; resolve: (saved: boolean) => void }
type PendingConfirmation = { title: string; description: string; confirmLabel: string; cancelLabel?: string; tone?: 'default' | 'danger'; resolve: (confirmed: boolean) => void }

const workspace = useWorkspaceStore()
const api = useApi()
const { t } = useI18n()
const { success: notifySuccess, error: notifyError } = useToast()
const showConnection = ref(false)
const editing = ref<Connection>()
const running = ref(false)
const loadingMoreRows = ref(false)
const queryError = ref('')
const showGlobalSearch = ref(false)
const history = ref<QueryHistory[]>([])
const selectedSavedQueryId = ref('')
const sqlEditor = ref<{ insertSQL: (sql: string) => void }>()
const aiAgent = ref<{ ask: (prompt: string) => Promise<void>; pasteQuery: (sql: string) => void }>()
const aiConfigured = ref(false)
const aiVisible = ref(true)
const editorHeight = ref(46)
const editorWidth = ref(50)
const connectionsWidth = ref(288)
const { adjustTextScale, restoreTextScale } = useTextScale()
const transactionStatus = reactive<Record<string, TransactionStatus>>({})
const commitConnectionId = ref<string>()
const committing = ref(false)
const queryPageSize = 200
const resultTabs = reactive<Record<string, ResultTab[]>>({})
const activeResultTabIds = reactive<Record<string, string | undefined>>({})
const pagedQueries = reactive<Record<string, { connectionId: string; sql: string; requestId: string } | undefined>>({})
const smartResultTabs = reactive<SmartResultTab[]>([])
const activeSmartResultTabIds = reactive<Record<string, string | undefined>>({})
const recentlyClosedTabs = ref<WorkspaceTab[]>([])
const pendingSave = ref<PendingSave>()
const pendingConfirmation = ref<PendingConfirmation>()
let nextResultTabId = 0
const smartQueryGenerations = new Set<string>()
const smartQueryRunning = ref(false)
const smartQueryError = ref('')
const smartQueryErrorConnectionId = ref<string>()

const activeTab = computed<WorkspaceTab>(() => workspace.tabs.find((tab) => tab.id === workspace.activeTabId) ?? workspace.tabs[0] ?? { id: 'empty', title: '', type: 'empty' })
const activeResultTabs = computed(() => resultTabs[activeTab.value.id] || [])
const activeResultTab = computed(() => activeResultTabs.value.find((tab) => tab.id === activeResultTabIds[activeTab.value.id]))
const activeResultSummary = computed(() => activeResultTab.value?.result ? `${t('query.rows', { count: `${activeResultTab.value.result.rowCount}${activeResultTab.value.result.hasMore ? '+' : ''}` })} · ${activeResultTab.value.result.executionTimeMs} ms · ${t('query.affected', { count: activeResultTab.value.result.affectedRows })}` : t('query.results'))
const showAIAgent = computed(() => aiConfigured.value && aiVisible.value)
const globalTabTypes = new Set<WorkspaceTab['type']>(['welcome', 'saved', 'smart', 'settings'])
const visibleTabs = computed(() => workspace.tabs.filter((tab) => globalTabTypes.has(tab.type) || tab.connectionId === workspace.activeConnectionId))
const queryConnectionId = computed(() => activeTab.value.executionConnectionId === 'auto' ? activeTab.value.connectionId : activeTab.value.executionConnectionId ?? activeTab.value.connectionId)
const queryConnection = computed(() => workspace.connections.find((connection) => connection.id === queryConnectionId.value))
const activeTransactionConnectionId = computed(() => queryConnectionId.value ?? activeTab.value.connectionId ?? workspace.activeConnectionId)
const activeTransaction = computed(() => activeTransactionConnectionId.value ? transactionStatus[activeTransactionConnectionId.value] : undefined)
const activeTransactionConnection = computed(() => workspace.connections.find((connection) => connection.id === activeTransactionConnectionId.value))
const connectionSavedQueries = computed(() => workspace.savedQueries.filter((query) => query.connectionId === workspace.activeConnectionId))
const connectionSmartQueries = computed(() => workspace.smartQueries.filter((query) => query.connectionId === workspace.activeConnectionId))
const connectionSmartResultTabs = computed(() => smartResultTabs.filter((tab) => tab.connectionId === workspace.activeConnectionId))
const activeSmartResultTabId = computed(() => {
  const activeId = workspace.activeConnectionId ? activeSmartResultTabIds[workspace.activeConnectionId] : undefined
  return connectionSmartResultTabs.value.some((tab) => tab.id === activeId) ? activeId : connectionSmartResultTabs.value.at(-1)?.id
})
const activeSmartQueryError = computed(() => smartQueryErrorConnectionId.value === workspace.activeConnectionId ? smartQueryError.value : '')

onMounted(async () => {
  workspace.restoreWorkspace()
  try { await Promise.all([loadAISettings(), workspace.refreshConnections(), workspace.syncWorkspaceQueries()]); await Promise.all(workspace.connections.filter((connection) => connection.environment === 'production').map((connection) => loadTransactionStatus(connection.id))); await loadHistory() }
  catch (error: any) { queryError.value = error.message }
})

function newSQL() { openSQLForConnection(workspace.activeConnectionId) }
function openSQLForConnection(connectionId?: string) {
  const connection = connectionId ? workspace.connections.find((item) => item.id === connectionId) : workspace.activeConnection
  if (!connection) { showConnection.value = true; return }
  workspace.activeConnectionId = connection.id
  workspace.openTab({ id: `sql:${connection.id}:${Date.now()}`, title: t('query.defaultTitle'), type: 'sql', connectionId: connection.id, executionConnectionId: connection.id, sql: 'SELECT * FROM users LIMIT 100;' })
}
function openSettings(section?: 'appearance' | 'shortcuts' | 'connections' | 'ai' | 'audit' | 'backup') {
  const existing = workspace.tabs.find((tab) => tab.id === 'settings')
  if (existing) existing.settingsSection = section || 'appearance'
  else workspace.openTab({ id: 'settings', title: t('settings.title'), type: 'settings', settingsSection: section || 'appearance' })
  workspace.activeTabId = 'settings'
}
function openHome() { workspace.openTab({ id: 'welcome', title: t('search.home'), type: 'welcome' }) }
function openConnectionHome(connection: Connection) {
  workspace.activeConnectionId = connection.id
  workspace.openTab({ id: `connection-home:${connection.id}`, title: connection.name, type: 'connection-home', connectionId: connection.id })
}
function openSavedQueries() { workspace.openTab({ id: 'saved-queries', title: t('savedQueries.title'), type: 'saved' }) }
function openSmartQueries() { workspace.openTab({ id: 'smart-queries', title: t('smartQueries.title'), type: 'smart' }) }

function openSavedQuery() {
  const query = workspace.savedQueries.find((item) => item.id === selectedSavedQueryId.value)
  selectedSavedQueryId.value = ''
  if (!query) return
  workspace.activeConnectionId = query.connectionId
  const existingTab = workspace.tabs.find((tab) => tab.savedQueryId === query.id)
  if (existingTab) {
    workspace.activeTabId = existingTab.id
    return
  }
  workspace.openTab({ id: `sql:${query.connectionId}:${Date.now()}`, title: query.name, type: 'sql', connectionId: query.connectionId, executionConnectionId: query.connectionId, sql: query.sql, savedQueryId: query.id })
}

function openSavedQueryById(id: string) {
  selectedSavedQueryId.value = id
  openSavedQuery()
}
function openSmartQueryById(id: string) {
  const query = workspace.smartQueries.find((item) => item.id === id)
  if (query) openSmartQueryInEditor(query)
}
async function removeSavedQuery(id: string) {
  const query = workspace.savedQueries.find((item) => item.id === id)
  if (!query || !await confirmAction({ title: t('savedQueries.deleteTitle'), description: t('savedQueries.deleteConfirm', { name: query.name }), confirmLabel: t('savedQueries.delete'), tone: 'danger' })) return
  await workspace.removeSavedQuery(id)
}
async function removeSmartQuery(id: string) {
  const query = workspace.smartQueries.find((item) => item.id === id)
  if (!query || !await confirmAction({ title: t('smartQueries.deleteTitle'), description: t('smartQueries.deleteConfirm', { name: query.title }), confirmLabel: t('smartQueries.delete'), tone: 'danger' })) return
  await workspace.removeSmartQuery(id)
}
function savedQueryLabel(query: SavedQuery) {
  const connection = workspace.connections.find((item) => item.id === query.connectionId)
  return `${query.name} · ${connection?.name || t('savedQueries.connectionUnavailable')}`
}

async function persistQuery(tab: WorkspaceTab, name: string) {
  const connectionId = tab.executionConnectionId === 'auto' ? tab.connectionId : tab.executionConnectionId ?? tab.connectionId
  if (!connectionId || !tab.sql?.trim()) return false
  const query = await workspace.saveQuery({ name: name.trim(), connectionId, sql: tab.sql }, tab.savedQueryId)
  tab.title = query.name
  tab.savedQueryId = query.id
  tab.dirty = false
  return true
}
function saveQuery(tab = activeTab.value): Promise<boolean> {
  if (tab.type !== 'sql' || !(tab.executionConnectionId === 'auto' ? tab.connectionId : tab.executionConnectionId ?? tab.connectionId) || !tab.sql?.trim()) return Promise.resolve(false)
  if (tab.savedQueryId) return persistQuery(tab, tab.title)
  return new Promise((resolve) => { pendingSave.value = { tab, resolve } })
}
async function resolveSave(name?: string) {
  const pending = pendingSave.value
  if (!pending) return
  pendingSave.value = undefined
  pending.resolve(name ? await persistQuery(pending.tab, name) : false)
}
function confirmAction(options: Omit<PendingConfirmation, 'resolve'>) {
  return new Promise<boolean>((resolve) => { pendingConfirmation.value = { ...options, resolve } })
}
function resolveConfirmation(confirmed: boolean) {
  const pending = pendingConfirmation.value
  if (!pending) return
  pendingConfirmation.value = undefined
  pending.resolve(confirmed)
}
function saveTabById(id: string) {
  const tab = workspace.tabs.find((item) => item.id === id)
  if (tab) saveQuery(tab)
}
async function requestCloseTabs(targets: WorkspaceTab[]) {
  if (!targets.length) return

  for (const tab of targets) {
    const needsSavePrompt = tab.type === 'sql' && tab.sql?.trim() && (!tab.savedQueryId || tab.dirty)
    if (needsSavePrompt && await confirmAction({ title: t('tabs.saveChangesTitle'), description: t('tabs.saveBeforeClose', { name: tab.title }), confirmLabel: t('tabs.saveAndClose'), cancelLabel: t('tabs.closeOnly') })) {
      if (!await saveQuery(tab)) return
    }
  }
  recentlyClosedTabs.value = [...recentlyClosedTabs.value, ...targets.filter((tab) => tab.id !== 'welcome').map((tab) => ({ ...tab }))].slice(-20)
  workspace.closeTabs(new Set(targets.map((tab) => tab.id)))
}
function reopenLastClosedTab() {
  const tab = recentlyClosedTabs.value.pop()
  if (!tab) return
  if (tab.connectionId) workspace.activeConnectionId = tab.connectionId
  workspace.openTab(tab)
}
function requestCloseTab(id: string) {
  const tab = workspace.tabs.find((item) => item.id === id)
  if (tab) requestCloseTabs([tab])
}
function requestCloseTabsToRight(id: string) {
  const index = workspace.tabs.findIndex((tab) => tab.id === id)
  if (index >= 0) requestCloseTabs(workspace.tabs.slice(index + 1))
}
function requestCloseOtherTabs(id: string) {
  requestCloseTabs(workspace.tabs.filter((tab) => tab.id !== id))
}

function openTable(connection: Connection, database: string, table: string) {
  workspace.activeConnectionId = connection.id
  workspace.openTab({ id: `table:${connection.id}:${database}:${table}`, title: table, type: 'table', connectionId: connection.id, database, table })
}
function openDatabase(connection: Connection, database: string) {
  workspace.activeConnectionId = connection.id
  workspace.openTab({ id: `database:${connection.id}:${database}`, title: database, type: 'database', connectionId: connection.id, database })
}
function openDatabaseTable(table: string) {
  const connection = workspace.connections.find((item) => item.id === activeTab.value.connectionId)
  if (connection && activeTab.value.database) openTable(connection, activeTab.value.database, table)
}
function openDatabaseFromTable(database: string) {
  const connection = workspace.connections.find((item) => item.id === activeTab.value.connectionId)
  if (connection) openDatabase(connection, database)
}
function openStats(connection: Connection) {
  workspace.activeConnectionId = connection.id
  workspace.openTab({ id: `stats:${connection.id}`, title: t('stats.title', { name: connection.name }), type: 'stats', connectionId: connection.id })
}

async function connect() {
  if (!workspace.activeConnection) return
  try { await workspace.connectConnection(workspace.activeConnection.id) }
  catch (error: any) { queryError.value = error.message }
}

function createResultTab(workspaceTabId: string) {
  const tabs = resultTabs[workspaceTabId] || (resultTabs[workspaceTabId] = [])
  const resultTab: ResultTab = { id: `result:${workspaceTabId}:${Date.now()}:${nextResultTabId++}`, title: t('query.resultTab', { count: tabs.length + 1 }), view: 'table', copied: false, editing: false }
  tabs.push(resultTab)
  const reactiveResultTab = tabs.at(-1)!
  activeResultTabIds[workspaceTabId] = reactiveResultTab.id
  return reactiveResultTab
}
function currentOrNewResultTab(workspaceTabId: string) {
  return (resultTabs[workspaceTabId] || []).find((tab) => tab.id === activeResultTabIds[workspaceTabId]) || createResultTab(workspaceTabId)
}
function closeResultTab(workspaceTabId: string, resultTabId: string) {
  const tabs = resultTabs[workspaceTabId]
  if (!tabs) return
  const index = tabs.findIndex((tab) => tab.id === resultTabId)
  if (index < 0) return
  tabs.splice(index, 1)
  delete pagedQueries[resultTabId]
  if (activeResultTabIds[workspaceTabId] === resultTabId) activeResultTabIds[workspaceTabId] = tabs[Math.max(0, index - 1)]?.id
}

async function execute(tab: WorkspaceTab, sql = tab.sql, newResultTab = false) {
  const connectionId = tab.executionConnectionId === 'auto' ? tab.connectionId : tab.executionConnectionId ?? tab.connectionId
  if (!connectionId || !sql?.trim()) return
  const connection = workspace.connections.find((item) => item.id === connectionId)
  if (!connection || connection.status !== 'connected') {
    queryError.value = t('query.connectBeforeRunning', { name: connection?.name || t('query.database') })
    return
  }
  const resultTab = newResultTab ? createResultTab(tab.id) : currentOrNewResultTab(tab.id)
  running.value = true
  queryError.value = ''
  delete pagedQueries[resultTab.id]
  try {
    const pageable = /^\s*select\b/i.test(sql)
    const result = await api<QueryResult>(`/connections/${connectionId}/query`, { method: 'POST', body: pageable ? { sql: pagedSQL(sql, 0), historySql: sql, requestId: resultTab.id } : { sql, requestId: resultTab.id } })
    resultTab.result = pageable ? pageResult(result) : result
    resultTab.sources = await editableSources(sql, connection, resultTab.result)
    if (pageable) pagedQueries[resultTab.id] = { connectionId, sql, requestId: resultTab.id }
    updateTransactionStatus(connectionId, resultTab.result.transactionPending, resultTab.result.pendingStatements)
    await loadHistory()
  } catch (error: any) { queryError.value = error.message }
  finally { running.value = false }
}

async function createSmartQuery(connectionId: string, sql: string) {
  const sourceSql = sql.trim()
  const generationKey = `${connectionId}:${sourceSql}`
  if (smartQueryGenerations.has(generationKey)) return
  smartQueryGenerations.add(generationKey)
  try {
    const query = await api<Omit<SmartQuery, 'id' | 'createdAt'>>('/ai/smart-queries', { method: 'POST', body: { connectionId, sql } })
    const smartQuery = await workspace.addSmartQuery({ ...query, connectionId, sourceSql })
    notifySuccess(t('smartQueries.created', { name: smartQuery.title }))
  } catch (error: any) { queryError.value = t('smartQueries.createError', { message: error.message || t('smartQueries.unknownError') }) }
  finally { smartQueryGenerations.delete(generationKey) }
}

function smartQuerySQL(query: SmartQuery, values: Record<string, string>) {
  return query.parameters.reduce((sql, parameter) => {
    const value = (values[parameter.key] ?? parameter.defaultValue).trim()
    if (!value) throw new Error(t('smartQueries.requiredValue', { name: parameter.key }))
    const quote = (item: string) => `'${item.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`
    const placeholder = new RegExp(`:${parameter.key}\\b`, 'g')
    const inPlaceholder = new RegExp(`\\bIN\\s*\\(\\s*:${parameter.key}\\b`, 'i')
    if (inPlaceholder.test(sql)) {
      const values = value.split(',').map((item) => item.trim()).filter(Boolean)
      if (!values.length) throw new Error(t('smartQueries.requiredValue', { name: parameter.key }))
      return sql.replace(placeholder, values.map(quote).join(', '))
    }
    return sql.replace(placeholder, quote(value))
  }, query.sql)
}
function createSmartResultTab(query: SmartQuery) {
  const { connectionId, id: smartQueryId, title } = query
  const tabsForConnection = smartResultTabs.filter((tab) => tab.connectionId === connectionId)
  const resultTab: SmartResultTab = { id: `smart-result:${connectionId}:${Date.now()}:${nextResultTabId++}`, title: title || t('query.resultTab', { count: tabsForConnection.length + 1 }), connectionId, smartQueryId, view: 'table', copied: false, editing: false }
  smartResultTabs.push(resultTab)
  const reactiveResultTab = smartResultTabs.at(-1)!
  activeSmartResultTabIds[connectionId] = reactiveResultTab.id
  return reactiveResultTab
}
function currentOrNewSmartResultTab(query: SmartQuery) {
  const resultTab = smartResultTabs.find((tab) => tab.connectionId === query.connectionId && tab.smartQueryId === query.id)
  if (resultTab) {
    activeSmartResultTabIds[query.connectionId] = resultTab.id
    return resultTab
  }
  return createSmartResultTab(query)
}
function closeSmartResultTab(id: string) {
  const index = smartResultTabs.findIndex((tab) => tab.id === id)
  if (index < 0) return
  const [tab] = smartResultTabs.splice(index, 1)
  if (!tab) return
  delete pagedQueries[id]
  if (activeSmartResultTabIds[tab.connectionId] === id) activeSmartResultTabIds[tab.connectionId] = smartResultTabs.filter((item) => item.connectionId === tab.connectionId).at(-1)?.id
}
function selectSmartResultTab(id: string) {
  const tab = smartResultTabs.find((item) => item.id === id)
  if (tab) activeSmartResultTabIds[tab.connectionId] = tab.id
}
async function copySmartResult(id: string) {
  const tab = smartResultTabs.find((item) => item.id === id)
  if (tab) await copyResult(tab)
}
async function saveSmartResultEdits(id: string, edited: QueryResult) {
  const tab = smartResultTabs.find((item) => item.id === id)
  if (tab) await saveResultEdits(tab, edited)
}
async function loadMoreSmartRows() {
  const id = activeSmartResultTabId.value
  const resultTab = smartResultTabs.find((tab) => tab.id === id)
  if (!resultTab) return
  const page = pagedQueries[resultTab.id]
  const current = resultTab.result
  if (!page || !current?.hasMore || loadingMoreRows.value || smartQueryRunning.value) return
  loadingMoreRows.value = true
  smartQueryError.value = ''
  try {
    const offset = current.rows.length
    const result = pageResult(await api<QueryResult>(`/connections/${page.connectionId}/query`, { method: 'POST', body: { sql: pagedSQL(page.sql, offset), historySql: page.sql, requestId: `${page.requestId}:page:${offset}`, skipHistory: true } }))
    if (resultTab.result !== current || pagedQueries[resultTab.id] !== page) return
    resultTab.result = { ...current, rows: [...current.rows, ...result.rows], rowCount: current.rows.length + result.rows.length, hasMore: result.hasMore }
  } catch (error: any) { smartQueryError.value = error.message }
  finally { loadingMoreRows.value = false }
}
async function runSmartQuery(query: SmartQuery, values: Record<string, string>, newTab: boolean) {
  try {
    const sql = smartQuerySQL(query, values)
    workspace.activeConnectionId = query.connectionId
    const connection = workspace.connections.find((item) => item.id === query.connectionId)
    smartQueryErrorConnectionId.value = query.connectionId
    if (!connection || connection.status !== 'connected') {
      smartQueryError.value = t('query.connectBeforeRunning', { name: connection?.name || t('query.database') })
      return
    }
    const resultTab = newTab ? createSmartResultTab(query) : currentOrNewSmartResultTab(query)
    resultTab.title = query.title
    smartQueryRunning.value = true
    smartQueryError.value = ''
    delete pagedQueries[resultTab.id]
    const pageable = /^\s*select\b/i.test(sql)
    const result = await api<QueryResult>(`/connections/${query.connectionId}/query`, { method: 'POST', body: pageable ? { sql: pagedSQL(sql, 0), historySql: sql, requestId: resultTab.id } : { sql, requestId: resultTab.id } })
    resultTab.result = pageable ? pageResult(result) : result
    resultTab.sources = await editableSources(sql, connection, resultTab.result)
    if (pageable) pagedQueries[resultTab.id] = { connectionId: query.connectionId, sql, requestId: resultTab.id }
    updateTransactionStatus(query.connectionId, resultTab.result.transactionPending, resultTab.result.pendingStatements)
    await loadHistory()
  } catch (error: any) { smartQueryError.value = error.message }
  finally { smartQueryRunning.value = false }
}
async function updateSmartQuery(id: string, changes: Pick<SmartQuery, 'title' | 'description' | 'sql'>) { await workspace.updateSmartQuery(id, changes) }
function openSmartQueryInEditor(query: SmartQuery) {
  workspace.activeConnectionId = query.connectionId
  workspace.openTab({ id: `sql:${query.connectionId}:${Date.now()}`, title: query.title, type: 'sql', connectionId: query.connectionId, executionConnectionId: query.connectionId, sql: query.sql })
}

function pagedSQL(sql: string, offset: number) {
  const statement = sql.trim().replace(/;+\s*$/, '')
  return `SELECT * FROM (${statement}) AS \`dbfock_page\` LIMIT ${queryPageSize + 1} OFFSET ${offset}`
}
function pageResult(result: QueryResult): QueryResult {
  const hasMore = result.rows.length > queryPageSize
  const rows = result.rows.slice(0, queryPageSize)
  return { ...result, rows, rowCount: rows.length, hasMore }
}
async function editableSources(sql: string, connection: Connection, result: QueryResult): Promise<EditableResultSource[]> {
  if (!/^\s*select\b/i.test(sql) || /\b(union|intersect|except)\b/i.test(sql)) return []
  const references = [...sql.matchAll(/\b(?:from|join)\s+((?:`[^`]+`|[A-Za-z_][A-Za-z0-9_$]*)(?:\s*\.\s*(?:`[^`]+`|[A-Za-z_][A-Za-z0-9_$]*))?)/gi)]
    .map((match) => match[1]?.split('.').map((part) => part.trim().replace(/^`|`$/g, '')))
    .filter((parts): parts is string[] => Boolean(parts?.length))
    .map((parts) => ({ database: parts.length === 2 ? parts[0] : connection.initialDatabase, table: parts.length === 2 ? parts[1] : parts[0] }))
    .filter((source): source is { database: string; table: string } => Boolean(source.database && source.table))
  const uniqueReferences = [...new Map(references.map((source) => [`${source.database}.${source.table}`, source])).values()]
  if (!uniqueReferences.length) return []
  try {
    const sources = await Promise.all(uniqueReferences.map(async (reference) => ({ ...reference, structure: await api<TableStructure>(`/connections/${connection.id}/databases/${encodeURIComponent(reference.database)}/tables/${encodeURIComponent(reference.table)}/structure`) })))
    const counts = new Map<string, number>()
    for (const column of result.columns) counts.set(column.name, (counts.get(column.name) ?? 0) + 1)
    const owners = new Map<string, number>()
    for (const source of sources) for (const column of source.structure.columns) owners.set(column.name, (owners.get(column.name) ?? 0) + 1)
    return sources.flatMap(({ database, table, structure }) => {
      const primaryKey = structure.constraints.find((constraint) => constraint.type === 'PRIMARY KEY')?.columns ?? structure.columns.filter((column) => column.key === 'PRI').map((column) => column.name)
      if (!primaryKey.length || !primaryKey.every((column) => counts.get(column) === 1 && owners.get(column) === 1)) return []
      const columns = structure.columns.map((column) => column.name).filter((column) => counts.get(column) === 1 && owners.get(column) === 1)
      return columns.length ? [{ connectionId: connection.id, database, table, columns, primaryKey }] : []
    })
  } catch {
    // A result can still be viewed when its source metadata is unavailable.
    return []
  }
}
async function saveResultEdits(resultTab: ResultTab, edited: QueryResult) {
  if (!resultTab.result || !resultTab.sources?.length) { notifyError(t('grid.inlineEditUnsupported')); return }
  try {
    let transaction: QueryResult | undefined
    for (const source of resultTab.sources) {
      const updates = queryResultEdits(resultTab.result, edited, { editableColumns: source.columns, keyColumns: source.primaryKey })
      for (const update of updates) transaction = await api<QueryResult>(`/connections/${source.connectionId}/rows/update`, { method: 'POST', body: { database: source.database, table: source.table, ...update } })
    }
    resultTab.result = edited
    resultTab.editing = false
    if (transaction) updateTransactionStatus(resultTab.sources[0]!.connectionId, transaction.transactionPending, transaction.pendingStatements)
    await loadHistory()
  } catch (error: any) { notifyError(error.message) }
}
async function copyResult(resultTab: ResultTab) {
  if (!resultTab.result || !navigator.clipboard) return
  const contents = resultTab.view === 'json' ? queryResultAsJSON(resultTab.result) : resultTab.view === 'csv' ? queryResultAsCSV(resultTab.result) : queryResultAsTSV(resultTab.result)
  try {
    await navigator.clipboard.writeText(contents)
    resultTab.copied = true
    window.setTimeout(() => resultTab.copied = false, 1500)
  } catch { /* Clipboard access can be denied by the browser. */ }
}
async function copyActiveResult(id: string) {
  const resultTab = activeResultTabs.value.find((tab) => tab.id === id)
  if (resultTab) await copyResult(resultTab)
}
async function saveActiveResultEdits(id: string, edited: QueryResult) {
  const resultTab = activeResultTabs.value.find((tab) => tab.id === id)
  if (resultTab) await saveResultEdits(resultTab, edited)
}
async function loadMoreRows() {
  const resultTab = activeResultTab.value
  if (!resultTab) return
  const page = pagedQueries[resultTab.id]
  const current = resultTab.result
  if (!page || !current?.hasMore || loadingMoreRows.value || running.value) return
  loadingMoreRows.value = true
  queryError.value = ''
  try {
    const offset = current.rows.length
    const result = pageResult(await api<QueryResult>(`/connections/${page.connectionId}/query`, { method: 'POST', body: { sql: pagedSQL(page.sql, offset), historySql: page.sql, requestId: `${page.requestId}:page:${offset}`, skipHistory: true } }))
    if (resultTab.result !== current || pagedQueries[resultTab.id] !== page) return
    resultTab.result = { ...current, rows: [...current.rows, ...result.rows], rowCount: current.rows.length + result.rows.length, hasMore: result.hasMore }
  } catch (error: any) { queryError.value = error.message }
  finally { loadingMoreRows.value = false }
}

async function loadTransactionStatus(connectionId: string) {
  transactionStatus[connectionId] = await api<TransactionStatus>(`/connections/${connectionId}/transaction`)
}
async function requestCommit(connectionId: string) {
  queryError.value = ''
  try {
    await loadTransactionStatus(connectionId)
    if (transactionStatus[connectionId]?.pending) commitConnectionId.value = connectionId
  } catch (error: any) { queryError.value = error.message }
}
function updateTransactionStatus(connectionId: string, pending: boolean, pendingStatements: number) {
  transactionStatus[connectionId] = { pending, pendingStatements, statements: [] }
  if (pending) void loadTransactionStatus(connectionId).catch((error: any) => { queryError.value = error.message })
}
async function commitTransaction(statementIds: string[]) {
  const connectionId = commitConnectionId.value
  if (!connectionId) return
  committing.value = true
  queryError.value = ''
  try {
    transactionStatus[connectionId] = await api<TransactionStatus>(`/connections/${connectionId}/transaction/commit`, { method: 'POST', body: { statementIds } })
    if (!transactionStatus[connectionId]?.pending) commitConnectionId.value = undefined
    await loadHistory()
  }
  catch (error: any) { queryError.value = error.message }
  finally { committing.value = false }
}
async function rollbackTransaction(statementIds: string[]) {
  const connectionId = commitConnectionId.value
  if (!connectionId) return
  committing.value = true
  queryError.value = ''
  try {
    transactionStatus[connectionId] = await api<TransactionStatus>(`/connections/${connectionId}/transaction/rollback`, { method: 'POST', body: { statementIds } })
    if (!transactionStatus[connectionId]?.pending) commitConnectionId.value = undefined
  }
  catch (error: any) { queryError.value = error.message }
  finally { committing.value = false }
}

async function loadHistory() { history.value = (await api<QueryHistory[]>('/query-history')) ?? [] }
function updateSQL(sql: string) { const tab = workspace.tabs.find((item) => item.id === workspace.activeTabId); if (tab) { tab.sql = sql; tab.dirty = true } }
function updateExecutionConnection(connectionId?: string) {
  const tab = workspace.tabs.find((item) => item.id === workspace.activeTabId)
  if (!connectionId || !tab) return
  tab.connectionId = connectionId
  tab.executionConnectionId = connectionId
  workspace.activeConnectionId = connectionId
}
function selectTab(id: string) {
  const tab = workspace.tabs.find((item) => item.id === id)
  if (!tab) return
  workspace.activeTabId = id
  if (tab.connectionId) workspace.activeConnectionId = tab.connectionId
}
function updateTableSection(section: 'data' | 'structure' | 'constraints' | 'foreignKeys' | 'references' | 'triggers' | 'indexes' | 'ddl' | 'diagram' | 'tools') { const tab = workspace.tabs.find((item) => item.id === workspace.activeTabId); if (tab) { tab.tableSection = section; tab.dirty = true } }
function updateDatabaseSection(section: 'tables' | 'diagram') { const tab = workspace.tabs.find((item) => item.id === workspace.activeTabId); if (tab) { tab.databaseSection = section; tab.dirty = true } }
function explainSQL(sql: string) { aiAgent.value?.ask(`${t('query.explainPrompt', { sql })}\n\n${t('query.answerLanguage')}`) }
function improveSQL(sql: string) { aiAgent.value?.ask(`${t('query.improvePrompt', { sql })}\n\n${t('query.answerLanguage')}`) }
function updateAIStatus(tabId: string, status: 'running' | 'complete') { const tab = workspace.tabs.find((item) => item.id === tabId); if (tab) tab.aiStatus = status }
async function loadAISettings() {
  try { aiConfigured.value = (await api<AISettings>('/ai/settings')).configured === true }
  catch { aiConfigured.value = false }
}
function markAIConfigured() { aiConfigured.value = true; aiVisible.value = true }
function hideAIAgent() { aiVisible.value = false }
function showHiddenAIAgent() { aiVisible.value = true }

function resizeVertical(event: PointerEvent) {
  const host = (event.currentTarget as HTMLElement).parentElement
  if (!host) return
  const bounds = host.getBoundingClientRect()
  const move = (next: PointerEvent) => editorHeight.value = Math.min(75, Math.max(25, (next.clientY - bounds.top) / bounds.height * 100))
  const stop = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', stop) }
  window.addEventListener('pointermove', move); window.addEventListener('pointerup', stop)
}

function resizeHorizontal(event: PointerEvent) {
  const host = (event.currentTarget as HTMLElement).parentElement
  if (!host) return
  const bounds = host.getBoundingClientRect()
  const move = (next: PointerEvent) => editorWidth.value = Math.min(75, Math.max(25, (next.clientX - bounds.left) / bounds.width * 100))
  const stop = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', stop) }
  window.addEventListener('pointermove', move); window.addEventListener('pointerup', stop)
}

function resizeConnections(event: PointerEvent) {
  const host = (event.currentTarget as HTMLElement).parentElement
  if (!host) return
  const bounds = host.getBoundingClientRect()
  const move = (next: PointerEvent) => connectionsWidth.value = Math.min(480, Math.max(200, next.clientX - bounds.left))
  const stop = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', stop) }
  window.addEventListener('pointermove', move); window.addEventListener('pointerup', stop)
}

async function saved() { await workspace.refreshConnections(); editing.value = undefined }
async function deleted() { const deletedId = editing.value?.id; if (deletedId === workspace.activeConnectionId) workspace.activeConnectionId = undefined; if (deletedId) { workspace.closeTab(`stats:${deletedId}`); workspace.closeTab(`connection-home:${deletedId}`) } await workspace.refreshConnections(); editing.value = undefined }
function openSearch(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    showGlobalSearch.value = true
  }
}
function navigateTabs(direction: 1 | -1) {
  const index = workspace.tabs.findIndex((tab) => tab.id === workspace.activeTabId)
  if (index < 0 || workspace.tabs.length < 2) return
  const next = (index + direction + workspace.tabs.length) % workspace.tabs.length
  workspace.activeTabId = workspace.tabs[next]!.id
}
function handleGlobalShortcut(event: KeyboardEvent) {
  if (!(event.metaKey || event.ctrlKey)) return
  const key = event.key.toLowerCase()
  if (key === 'w') {
    event.preventDefault()
    requestCloseTab(workspace.activeTabId)
  } else if (key === 's' && activeTab.value.type === 'sql') {
    event.preventDefault()
    saveQuery()
  } else if (key === '9') {
    event.preventDefault()
    navigateTabs(-1)
  } else if (key === '0') {
    event.preventDefault()
    navigateTabs(1)
  } else if (event.key === '+' || event.key === '=') {
    event.preventDefault()
    adjustTextScale(0.1)
  } else if (event.key === '-') {
    event.preventDefault()
    adjustTextScale(-0.1)
  }
}

onMounted(() => {
  const height = Number(localStorage.getItem('dbfock.sql-editor-height'))
  const width = Number(localStorage.getItem('dbfock.sql-editor-width'))
  const savedConnectionsWidth = Number(localStorage.getItem('dbfock.connections-width'))
  if (height >= 25 && height <= 75) editorHeight.value = height
  if (width >= 25 && width <= 75) editorWidth.value = width
  if (savedConnectionsWidth >= 200 && savedConnectionsWidth <= 480) connectionsWidth.value = savedConnectionsWidth
  restoreTextScale()
})
onMounted(() => window.addEventListener('keydown', openSearch, true))
onMounted(() => window.addEventListener('keydown', handleGlobalShortcut, true))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', openSearch, true)
  window.removeEventListener('keydown', handleGlobalShortcut, true)
})
watch(queryError, (message) => { if (message) { notifyError(message); queryError.value = '' } })
watch(smartQueryError, (message) => { if (message) { notifyError(message); smartQueryError.value = '' } })
watch([editorHeight, editorWidth], ([height, width]) => {
  if (import.meta.client) { localStorage.setItem('dbfock.sql-editor-height', String(height)); localStorage.setItem('dbfock.sql-editor-width', String(width)) }
})
watch(connectionsWidth, (width) => {
  if (import.meta.client) localStorage.setItem('dbfock.connections-width', String(width))
})
watch(() => workspace.activeConnectionId, () => {
  if (visibleTabs.value.some((tab) => tab.id === workspace.activeTabId)) return
  workspace.activeTabId = visibleTabs.value.find((tab) => tab.connectionId === workspace.activeConnectionId)?.id || 'welcome'
})
</script>

<template>
  <div class="flex h-full">
    <DatabaseTree :connections="workspace.connections" :active-connection-id="workspace.activeConnectionId" :width="connectionsWidth" @choose="workspace.activeConnectionId = $event" @table="openTable" @database="openDatabase" @connection-home="openConnectionHome" @new-query="openSQLForConnection($event.id)" @add="editing = undefined; showConnection = true" @home="openHome" @saved="openSavedQueries" @smart="openSmartQueries" @settings="openSettings" />
    <div class="w-1.5 shrink-0 cursor-col-resize bg-line hover:bg-accent" @pointerdown="resizeConnections" />
    <section class="flex min-w-0 flex-1 flex-col">
      <WorkspaceTabs :tabs="visibleTabs" :active-id="workspace.activeTabId" :can-reopen="recentlyClosedTabs.length > 0" @select="selectTab" @close="requestCloseTab" @close-right="requestCloseTabsToRight" @close-others="requestCloseOtherTabs" @save="saveTabById" @reopen="reopenLastClosedTab" @reorder="workspace.moveTab" @new-query="newSQL" />
      <div v-if="activeTransaction?.pending && activeTransactionConnectionId" class="flex shrink-0 items-center justify-between gap-4 border-b border-amber-500/25 bg-amber-500/5 px-4 py-2 text-xs">
        <div class="flex min-w-0 items-center gap-2"><span class="truncate font-medium text-ink">{{ activeTransactionConnection?.name }}</span><span class="rounded bg-amber-500/20 px-1.5 py-0.5 font-medium text-ink">{{ t('transaction.pending', { count: activeTransaction.pendingStatements }) }}</span></div>
        <div class="flex shrink-0 gap-2"><button class="rounded px-2 py-1 text-rose-700 hover:bg-amber-500/10 dark:text-rose-300" :disabled="committing" @click="requestCommit(activeTransactionConnectionId)">{{ t('transaction.rollback') }}</button><button class="rounded bg-amber-600 px-2.5 py-1 font-medium text-white disabled:opacity-50" :disabled="committing" @click="requestCommit(activeTransactionConnectionId)">{{ committing ? t('transaction.committing') : t('transaction.commit') }}</button></div>
      </div>
      <div class="min-h-0 flex-1">
        <div v-if="activeTab.type === 'empty'" class="h-full" />
        <div v-else-if="activeTab.type === 'welcome'" class="grid h-full place-items-center p-8">
          <div class="max-w-md text-center">
            <label v-if="connectionSavedQueries.length" class="mb-5 block text-left">
              <span class="mb-1 block text-xs font-medium text-muted">{{ t('home.openSavedQuery') }}</span>
              <AppSelect v-model="selectedSavedQueryId" :placeholder="t('home.chooseSavedQuery')" :options="connectionSavedQueries.map((query) => ({ value: query.id, label: savedQueryLabel(query) }))" @change="openSavedQuery" />
            </label>
            <div class="mx-auto grid h-12 w-12 place-items-center rounded-xl bg-accent/10 text-accent"><Icon name="lucide:file-code-2" class="h-6 w-6" aria-hidden="true" /></div>
            <h1 class="mt-4 text-xl font-semibold">{{ t('home.title') }}</h1>
            <p class="mt-2 text-sm leading-6 text-muted">{{ t('home.description') }}</p>
            <div class="mt-5 flex justify-center gap-2"><button class="rounded-md bg-accent px-3 py-2 text-sm font-medium text-white" @click="showConnection = true">{{ t('home.createConnection') }}</button><button class="rounded-md border border-line px-3 py-2 text-sm" @click="newSQL">{{ t('home.newQuery') }}</button></div>
            <button v-if="workspace.activeConnection" class="mt-4 text-sm text-accent" @click="connect">{{ t('home.connectTo', { name: workspace.activeConnection.name }) }}</button>
          </div>
        </div>
        <SavedQueriesWorkspace v-else-if="activeTab.type === 'saved'" :queries="connectionSavedQueries" :connections="workspace.connections" @open="openSavedQueryById" @remove="removeSavedQuery" />
        <SmartQueriesWorkspace v-else-if="activeTab.type === 'smart'" :queries="connectionSmartQueries" :connections="workspace.connections" :result-tabs="connectionSmartResultTabs" :active-result-tab-id="activeSmartResultTabId" :loading="smartQueryRunning" :loading-more="loadingMoreRows" @run="runSmartQuery" @remove="removeSmartQuery" @update="updateSmartQuery" @open-editor="openSmartQueryInEditor" @select-result-tab="selectSmartResultTab" @close-result-tab="closeSmartResultTab" @copy-result="copySmartResult" @save-result="saveSmartResultEdits" @load-more="loadMoreSmartRows" />
        <TableWorkspace v-else-if="activeTab.type === 'table' && workspace.connections.find((connection) => connection.id === activeTab.connectionId)" :key="`table:${activeTab.id}`" :connection-id="activeTab.connectionId!" :database="activeTab.database!" :table="activeTab.table!" :active-section="activeTab.tableSection" @update:active-section="updateTableSection" @transaction-status="updateTransactionStatus" @open-database="openDatabaseFromTable" @open-table="openDatabaseTable" />
        <DatabaseWorkspace v-else-if="activeTab.type === 'database' && workspace.connections.find((connection) => connection.id === activeTab.connectionId)" :key="`database:${activeTab.id}`" :connection-id="activeTab.connectionId!" :database="activeTab.database!" :active-section="activeTab.databaseSection" @table="openDatabaseTable" @update:active-section="updateDatabaseSection" />
        <ConnectionHomeWorkspace v-else-if="activeTab.type === 'connection-home' && workspace.connections.find((connection) => connection.id === activeTab.connectionId)" :key="`connection-home:${activeTab.id}`" :connection="workspace.connections.find((connection) => connection.id === activeTab.connectionId)!" @edit="editing = $event; showConnection = true" @new-query="openSQLForConnection($event.id)" @stats="openStats" @database="openDatabase" />
        <ConnectionStatsWorkspace v-else-if="activeTab.type === 'stats' && activeTab.connectionId && workspace.connections.find((connection) => connection.id === activeTab.connectionId)" :key="`stats:${activeTab.id}`" :connection="workspace.connections.find((connection) => connection.id === activeTab.connectionId)!" />
        <SettingsWorkspace v-else-if="activeTab.type === 'settings'" :section="activeTab.settingsSection" @ai-configured="markAIConfigured" />
        <div v-else class="flex h-full min-h-0 flex-col">
          <div class="relative flex min-h-64 shrink-0 border-b border-line" :style="{ height: `${editorHeight}%` }">
            <SqlEditor ref="sqlEditor" split :width="showAIAgent ? editorWidth : 'calc(100% - 1.5rem)'" :model-value="activeTab.sql || ''" :connection-id="queryConnection?.id || ''" :connection-name="queryConnection?.name || ''" :connections="workspace.connections" :execution-connection-id="activeTab.executionConnectionId" :initial-database="queryConnection?.initialDatabase" :production="queryConnection?.environment === 'production'" :running="running" @update:model-value="updateSQL" @update:execution-connection-id="updateExecutionConnection" @execute="(sql, newResultTab) => execute(activeTab, sql, newResultTab)" @explain="explainSQL" @create-smart-query="queryConnectionId && createSmartQuery(queryConnectionId, $event)" @improve="improveSQL" @send-to-chat="aiAgent?.pasteQuery($event)" @new-query="newSQL" @save-query="saveQuery" />
            <div v-if="showAIAgent" class="w-1.5 shrink-0 cursor-col-resize bg-line hover:bg-accent" @pointerdown="resizeHorizontal" @dblclick="hideAIAgent" />
            <AIAgentPanel v-if="showAIAgent && queryConnectionId" ref="aiAgent" :tab-id="activeTab.id" :width="100 - editorWidth" :connection-id="queryConnectionId" :database="queryConnection?.initialDatabase" :query="activeTab.sql" @apply="sqlEditor?.insertSQL($event)" @status="updateAIStatus" />
            <button v-else-if="aiConfigured" type="button" class="absolute inset-y-0 right-0 z-10 w-6 border-l border-line bg-panel text-xs font-medium text-muted hover:bg-canvas hover:text-ink" :title="t('aiAgent.title')" :aria-label="t('aiAgent.title')" style="writing-mode: vertical-rl" @click="showHiddenAIAgent">{{ t('aiAgent.title') }}</button>
          </div>
          <div class="h-1.5 shrink-0 cursor-row-resize bg-line hover:bg-accent" @pointerdown="resizeVertical" />
          <QueryResults :result-tabs="activeResultTabs" :active-result-tab-id="activeResultTab?.id" :loading="running" :loading-more="loadingMoreRows" :can-create-tab="true" :summary="activeResultSummary" @select-tab="activeResultTabIds[activeTab.id] = $event" @close-tab="closeResultTab(activeTab.id, $event)" @create-tab="createResultTab(activeTab.id)" @copy="copyActiveResult" @save="saveActiveResultEdits" @load-more="loadMoreRows" />
        </div>
      </div>
      <div class="border-t border-line bg-panel px-4 py-2"><details><summary class="cursor-pointer text-xs font-medium text-muted">{{ t('query.history', { count: history.length }) }}</summary><div class="scrollbar mt-2 max-h-28 overflow-auto"><button v-for="item in history" :key="item.id" class="flex w-full items-center gap-1.5 truncate py-1 text-left font-mono text-xs text-muted hover:text-ink" @click="workspace.openTab({ id: `sql:${item.connectionId}:${Date.now()}`, title: t('query.historyTitle'), type: 'sql', connectionId: item.connectionId, executionConnectionId: item.connectionId, sql: item.sql })"><Icon :name="item.status === 'error' ? 'lucide:circle-x' : 'lucide:circle-check'" class="h-3.5 w-3.5 shrink-0" :class="item.status === 'error' ? 'text-rose-500' : 'text-emerald-500'" aria-hidden="true" /><span class="truncate">{{ item.sql }}</span></button></div></details></div>
    </section>
    <ConnectionModal v-model="showConnection" :connection="editing" @saved="saved" @deleted="deleted" />
    <TransactionCommitModal v-if="commitConnectionId" :model-value="true" :connection-name="workspace.connections.find((connection) => connection.id === commitConnectionId)?.name || ''" :statements="transactionStatus[commitConnectionId]?.statements || []" :committing="committing" @update:model-value="commitConnectionId = undefined" @commit="commitTransaction" @rollback="rollbackTransaction" />
    <GlobalSearch v-if="showGlobalSearch" :tabs="workspace.tabs" :active-tab-id="workspace.activeTabId" :saved-queries="connectionSavedQueries" :smart-queries="workspace.smartQueries" :connections="workspace.connections" @close="showGlobalSearch = false" @select-tab="workspace.activeTabId = $event" @open-saved-query="openSavedQueryById" @open-smart-query="openSmartQueryById" @new-query="newSQL" @open-settings="openSettings" />
    <QuerySaveDialog :model-value="Boolean(pendingSave)" :initial-value="pendingSave?.tab.title || ''" :title="t('query.saveTitle')" :description="t('query.saveDescription')" :label="t('query.nameLabel')" :confirm-label="t('query.saveAction')" :cancel-label="t('connection.cancel')" @update:model-value="(value) => { if (!value) resolveSave() }" @confirm="resolveSave" />
    <AppConfirmDialog :model-value="Boolean(pendingConfirmation)" :title="pendingConfirmation?.title || ''" :description="pendingConfirmation?.description || ''" :confirm-label="pendingConfirmation?.confirmLabel || ''" :cancel-label="pendingConfirmation?.cancelLabel || t('connection.cancel')" :tone="pendingConfirmation?.tone" @update:model-value="(value) => { if (!value) resolveConfirmation(false) }" @confirm="resolveConfirmation(true)" />
  </div>
</template>
