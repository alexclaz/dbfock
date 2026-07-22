import type { AIAgentChat, Connection, SavedQuery, SmartQuery, WorkspaceTab } from '~/types/database'

const defaultTabs = (): WorkspaceTab[] => [{ id: 'welcome', title: 'Home', type: 'welcome' }]
const homeTabId = 'welcome'
const pinnedTabIds = new Set(['welcome', 'saved-queries', 'smart-queries', 'settings'])
const validTabTypes = new Set<WorkspaceTab['type']>(['welcome', 'saved', 'smart', 'sql', 'table', 'database', 'connection-home', 'settings', 'stats'])
const validTableSections = new Set<NonNullable<WorkspaceTab['tableSection']>>(['data', 'structure', 'constraints', 'foreignKeys', 'references', 'triggers', 'indexes', 'ddl'])

type SavedWorkspace = {
  version?: number
  tabs?: WorkspaceTab[]
  activeTabId?: string
  activeConnectionId?: string
  savedQueries?: SavedQuery[]
  smartQueries?: SmartQuery[]
}

function isAIAgentChat(value: unknown): value is AIAgentChat {
  if (!value || typeof value !== 'object') return false
  const candidate = value as AIAgentChat
  return typeof candidate.draft === 'string'
    && Array.isArray(candidate.messages)
    && (candidate.includeEditorQuery === undefined || typeof candidate.includeEditorQuery === 'boolean')
    && candidate.messages.every((message) => message && typeof message === 'object' && (message.role === 'user' || message.role === 'assistant') && typeof message.content === 'string')
}

function savedTabs(value: unknown): WorkspaceTab[] | undefined {
  if (!Array.isArray(value)) return undefined

  const tabs = value.filter((tab): tab is WorkspaceTab => {
    if (!tab || typeof tab !== 'object') return false
    const candidate = tab as WorkspaceTab
    return typeof candidate.id === 'string'
      && typeof candidate.title === 'string'
      && validTabTypes.has(candidate.type)
      && (candidate.connectionId === undefined || typeof candidate.connectionId === 'string')
      && (candidate.executionConnectionId === undefined || typeof candidate.executionConnectionId === 'string')
      && (candidate.database === undefined || typeof candidate.database === 'string')
      && (candidate.table === undefined || typeof candidate.table === 'string')
      && (candidate.sql === undefined || typeof candidate.sql === 'string')
      && (candidate.tableSection === undefined || validTableSections.has(candidate.tableSection))
      && (candidate.settingsSection === undefined || candidate.settingsSection === 'appearance' || candidate.settingsSection === 'shortcuts' || candidate.settingsSection === 'connections' || candidate.settingsSection === 'ai' || candidate.settingsSection === 'audit' || candidate.settingsSection === 'backup')
      && (candidate.aiChat === undefined || isAIAgentChat(candidate.aiChat))
      && (candidate.aiJobId === undefined || typeof candidate.aiJobId === 'string')
      && (candidate.aiStatus === undefined || candidate.aiStatus === 'running' || candidate.aiStatus === 'complete')
      && (candidate.dirty === undefined || typeof candidate.dirty === 'boolean')
      && (candidate.savedQueryId === undefined || typeof candidate.savedQueryId === 'string')
  })

  return tabs
}

function orderedTabs(value: WorkspaceTab[]): WorkspaceTab[] {
  const pinnedTabs = [...pinnedTabIds].map((id) => value.find((tab) => tab.id === id)).filter((tab): tab is WorkspaceTab => Boolean(tab))
  const otherTabs = value.filter((tab) => !pinnedTabIds.has(tab.id))
  return [...pinnedTabs, ...otherTabs]
}

function parseSavedQueries(value: unknown): SavedQuery[] {
  if (!Array.isArray(value)) return []

  return value.filter((query): query is SavedQuery => {
    if (!query || typeof query !== 'object') return false
    const candidate = query as SavedQuery
    return typeof candidate.id === 'string'
      && typeof candidate.name === 'string'
      && typeof candidate.connectionId === 'string'
      && typeof candidate.sql === 'string'
      && typeof candidate.updatedAt === 'string'
  })
}

function parseSmartQueries(value: unknown): SmartQuery[] {
  if (!Array.isArray(value)) return []
  return value.filter((query): query is SmartQuery => {
    if (!query || typeof query !== 'object') return false
    const candidate = query as SmartQuery
    return typeof candidate.id === 'string' && typeof candidate.connectionId === 'string' && typeof candidate.title === 'string' && typeof candidate.description === 'string' && typeof candidate.sql === 'string' && (candidate.sourceSql === undefined || typeof candidate.sourceSql === 'string') && typeof candidate.createdAt === 'string' && Array.isArray(candidate.parameters) && candidate.parameters.every((parameter) => parameter && typeof parameter.key === 'string' && typeof parameter.defaultValue === 'string')
  })
}

export const useWorkspaceStore = defineStore('workspace', () => {
  const connections = ref<Connection[]>([])
  const activeConnectionId = ref<string>()
  const tabs = ref<WorkspaceTab[]>(defaultTabs())
  const activeTabId = ref('welcome')
  const savedQueries = ref<SavedQuery[]>([])
  const smartQueries = ref<SmartQuery[]>([])
  const activeConnection = computed(() => connections.value.find((item) => item.id === activeConnectionId.value))
  const api = useApi()
  const storageKey = 'dbfock.workspace-tabs'
  const restored = ref(false)

  function restoreWorkspace() {
    if (!import.meta.client || restored.value) return

    try {
      const saved = JSON.parse(localStorage.getItem(storageKey) || '{}') as SavedWorkspace
      const restoredTabs = savedTabs(saved.tabs)
      if (restoredTabs) {
        // A persisted job means the backend may still be working. Show the
        // tab indicator immediately; the panel will reconcile its final state.
        tabs.value = orderedTabs(restoredTabs.map((tab) => {
          const normalizedTab = tab.executionConnectionId === 'auto' ? { ...tab, executionConnectionId: tab.connectionId } : tab
          return normalizedTab.aiJobId ? { ...normalizedTab, aiStatus: 'running' } : normalizedTab
        }))
      }
      const homeTab = tabs.value.find((tab) => tab.id === homeTabId)
      if (homeTab) homeTab.title = 'Home'
      else tabs.value = orderedTabs([...tabs.value, ...defaultTabs()])
      savedQueries.value = parseSavedQueries(saved.savedQueries)
      smartQueries.value = parseSmartQueries(saved.smartQueries)
      if (saved.activeTabId && tabs.value.some((tab) => tab.id === saved.activeTabId)) activeTabId.value = saved.activeTabId
      if (typeof saved.activeConnectionId === 'string') activeConnectionId.value = saved.activeConnectionId
    } catch { /* Start with the default workspace when saved state is invalid. */ }
    finally { restored.value = true }
  }

  function persistWorkspace() {
    if (import.meta.client && restored.value) {
      localStorage.setItem(storageKey, JSON.stringify({
        version: 1,
        // The job ID is persisted so an in-flight backend job can be recovered after a reload.
        tabs: tabs.value.map(({ aiStatus: _aiStatus, ...tab }) => tab),
        activeTabId: activeTabId.value,
        activeConnectionId: activeConnectionId.value,
        savedQueries: savedQueries.value,
        smartQueries: smartQueries.value,
      }))
    }
  }

  watch([tabs, activeTabId, activeConnectionId, savedQueries, smartQueries], persistWorkspace, { deep: true, flush: 'sync' })

  async function refreshConnections() {
    connections.value = (await api<Connection[]>('/connections')) ?? []
    if (!connections.value.some((connection) => connection.id === activeConnectionId.value)) activeConnectionId.value = connections.value[0]?.id
  }
  async function reloadWorkspaceQueries() {
    const [saved, smart] = await Promise.all([api<SavedQuery[]>('/saved-queries'), api<SmartQuery[]>('/smart-queries')])
    savedQueries.value = saved ?? []
    smartQueries.value = smart ?? []
  }
  async function syncWorkspaceQueries() {
    const localSaved = [...savedQueries.value]
    const localSmart = [...smartQueries.value]
    const [remoteSaved, remoteSmart] = await Promise.all([api<SavedQuery[]>('/saved-queries'), api<SmartQuery[]>('/smart-queries')])
    if (remoteSaved.length === 0 && localSaved.length > 0) await Promise.all(localSaved.map((query) => api<SavedQuery>('/saved-queries', { method: 'POST', body: query })))
    if (remoteSmart.length === 0 && localSmart.length > 0) await Promise.all(localSmart.map((query) => api<SmartQuery>('/smart-queries', { method: 'POST', body: query })))
    await reloadWorkspaceQueries()
  }
  async function connectConnection(id: string) {
    try { await api(`/connections/${id}/connect`, { method: 'POST' }) }
    finally { await refreshConnections() }
  }
  async function disconnectConnection(id: string) {
    try { await api(`/connections/${id}/disconnect`, { method: 'POST' }) }
    finally { await refreshConnections() }
  }
  async function revalidateConnection(id: string) {
    try {
      await api(`/connections/${id}/disconnect`, { method: 'POST' })
      await api(`/connections/${id}/connect`, { method: 'POST' })
    } finally { await refreshConnections() }
  }
  function openTab(tab: WorkspaceTab) {
    const existing = tabs.value.find((item) => item.id === tab.id)
    if (!existing) tabs.value.push(tab)
    tabs.value = orderedTabs(tabs.value)
    activeTabId.value = tab.id
  }
  function closeTabs(ids: Set<string>) {
    const closableIds = new Set([...ids].filter((id) => id !== homeTabId))
    if (!closableIds.size) return

    const activeIndex = tabs.value.findIndex((tab) => tab.id === activeTabId.value)
    const activeWasClosed = closableIds.has(activeTabId.value)
    const fallbackTabId = activeIndex < 0
      ? tabs.value.find((tab) => !closableIds.has(tab.id))?.id || ''
      : [...tabs.value.slice(0, activeIndex)].reverse().find((tab) => !closableIds.has(tab.id))?.id || tabs.value.slice(activeIndex + 1).find((tab) => !closableIds.has(tab.id))?.id || ''
    tabs.value = tabs.value.filter((tab) => !closableIds.has(tab.id))

    if (activeWasClosed) activeTabId.value = fallbackTabId
  }
  function closeTab(id: string) { closeTabs(new Set([id])) }
  function closeTabsToRight(id: string) {
    const index = tabs.value.findIndex((tab) => tab.id === id)
    if (index < 0) return
    closeTabs(new Set(tabs.value.slice(index + 1).map((tab) => tab.id)))
  }
  function closeOtherTabs(id: string) {
    if (!tabs.value.some((tab) => tab.id === id)) return
    closeTabs(new Set(tabs.value.filter((tab) => tab.id !== id).map((tab) => tab.id)))
  }
  function moveTab(id: string, targetId: string) {
    if (pinnedTabIds.has(id) || pinnedTabIds.has(targetId) || id === targetId) return
    const from = tabs.value.findIndex((tab) => tab.id === id)
    const to = tabs.value.findIndex((tab) => tab.id === targetId)
    if (from < 0 || to < 0) return
    const [tab] = tabs.value.splice(from, 1)
    if (!tab) return
    tabs.value.splice(from < to ? to - 1 : to, 0, tab)
  }
  async function saveQuery(query: Omit<SavedQuery, 'id' | 'updatedAt'>, id?: string) {
    const existing = id ? savedQueries.value.findIndex((item) => item.id === id) : -1
    const draft: SavedQuery = { id: existing >= 0 ? id! : `saved:${Date.now()}`, ...query, updatedAt: new Date().toISOString() }
    const saved = await api<SavedQuery>('/saved-queries', { method: 'POST', body: draft })
    if (existing >= 0) savedQueries.value.splice(existing, 1, saved)
    else savedQueries.value.unshift(saved)
    return saved
  }
  async function removeSavedQuery(id: string) {
    await api(`/saved-queries/${encodeURIComponent(id)}`, { method: 'DELETE' })
    savedQueries.value = savedQueries.value.filter((query) => query.id !== id)
    tabs.value.forEach((tab) => {
      if (tab.savedQueryId === id) tab.savedQueryId = undefined
    })
  }
  async function addSmartQuery(query: Omit<SmartQuery, 'id' | 'createdAt'>) {
    const existing = smartQueries.value.find((item) => item.connectionId === query.connectionId && (item.sourceSql || item.sql) === (query.sourceSql || query.sql))
    if (existing) return existing
    const draft: SmartQuery = { id: `smart:${Date.now()}`, ...query, createdAt: new Date().toISOString() }
    const smart = await api<SmartQuery>('/smart-queries', { method: 'POST', body: draft })
    smartQueries.value.unshift(smart)
    return smart
  }
  async function updateSmartQuery(id: string, changes: Pick<SmartQuery, 'title' | 'description' | 'sql'>) {
    const index = smartQueries.value.findIndex((query) => query.id === id)
    if (index < 0) return
    const current = smartQueries.value[index]
    if (!current) return
    const parameterByKey = new Map(current.parameters.map((parameter) => [parameter.key, parameter]))
    const keys = [...changes.sql.matchAll(/:([A-Za-z][A-Za-z0-9_]*)\b/g)].flatMap((match) => match[1] ? [match[1]] : []).filter((key, position, all) => all.indexOf(key) === position)
    const updated: SmartQuery = {
      ...current,
      ...changes,
      parameters: keys.map((key) => parameterByKey.get(key) || { key, defaultValue: '' }),
    }
    const smart = await api<SmartQuery>('/smart-queries', { method: 'POST', body: updated })
    smartQueries.value.splice(index, 1, smart)
  }
  async function removeSmartQuery(id: string) {
    await api(`/smart-queries/${encodeURIComponent(id)}`, { method: 'DELETE' })
    smartQueries.value = smartQueries.value.filter((query) => query.id !== id)
  }
  return { connections, activeConnectionId, activeConnection, tabs, activeTabId, savedQueries, smartQueries, refreshConnections, reloadWorkspaceQueries, syncWorkspaceQueries, connectConnection, disconnectConnection, revalidateConnection, openTab, closeTabs, closeTab, closeTabsToRight, closeOtherTabs, moveTab, saveQuery, removeSavedQuery, addSmartQuery, updateSmartQuery, removeSmartQuery, restoreWorkspace }
})
