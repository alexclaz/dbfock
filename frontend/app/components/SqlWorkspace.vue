<script setup lang="ts">
import type { Connection, QueryResult, WorkspaceTab } from '~/types/database'

type ResultTab = {
  id: string
  title: string
  result?: QueryResult
  view: 'table' | 'json' | 'csv'
  copied: boolean
  editing: boolean
  sources?: { columns: string[] }[]
}

const props = defineProps<{
  tab: WorkspaceTab
  connections: Connection[]
  queryConnection?: Connection
  queryConnectionId?: string
  showAIAgent: boolean
  aiConfigured: boolean
  editorHeight: number
  editorWidth: number
  running: boolean
  loadingMoreRows: boolean
  resultTabs: ResultTab[]
  activeResultTabId?: string
  resultSummary: string
}>()
const emit = defineEmits<{
  updateSql: [sql: string]
  updateExecutionConnection: [connectionId?: string]
  updateDefaultDatabase: [database?: string]
  viewport: [viewport: { top: number; left: number }]
  execute: [sql: string, newResultTab?: boolean]
  explain: [sql: string]
  createSmartQuery: [sql: string]
  improve: [sql: string]
  newQuery: []
  saveQuery: []
  aiStatus: [tabId: string, status: 'running' | 'complete']
  hideAIAgent: []
  showAIAgent: []
  updateEditorHeight: [height: number]
  updateEditorWidth: [width: number]
  selectResultTab: [id: string]
  closeResultTab: [id: string]
  createResultTab: []
  copyResult: [id: string]
  saveResult: [id: string, result: QueryResult]
  loadMore: []
  sortResult: [id: string, column: string, direction: 'asc' | 'desc']
}>()

const { t } = useI18n()
const sqlEditor = ref<{ insertSQL: (sql: string) => void }>()
const aiAgent = ref<{ ask: (prompt: string) => Promise<void>; pasteQuery: (sql: string) => void }>()

function ask(prompt: string) { return aiAgent.value?.ask(prompt) }
defineExpose({ ask })

function resizeVertical(event: PointerEvent) {
  const host = (event.currentTarget as HTMLElement).parentElement
  if (!host) return
  const bounds = host.getBoundingClientRect()
  const move = (next: PointerEvent) => emit('updateEditorHeight', Math.min(75, Math.max(25, (next.clientY - bounds.top) / bounds.height * 100)))
  const stop = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', stop) }
  window.addEventListener('pointermove', move); window.addEventListener('pointerup', stop)
}

function resizeHorizontal(event: PointerEvent) {
  const host = (event.currentTarget as HTMLElement).parentElement
  if (!host) return
  const bounds = host.getBoundingClientRect()
  const move = (next: PointerEvent) => emit('updateEditorWidth', Math.min(75, Math.max(25, (next.clientX - bounds.left) / bounds.width * 100)))
  const stop = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', stop) }
  window.addEventListener('pointermove', move); window.addEventListener('pointerup', stop)
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="relative flex min-h-64 shrink-0 border-b border-line" :style="{ height: `${editorHeight}%` }">
      <SqlEditor ref="sqlEditor" :tab-id="tab.id" split :width="showAIAgent ? editorWidth : 'calc(100% - 1.5rem)'" :model-value="tab.sql || ''" :connection-id="queryConnection?.id || ''" :connection-name="queryConnection?.name || ''" :connections="connections" :execution-connection-id="tab.executionConnectionId" :initial-database="queryConnection?.initialDatabase" :default-database="tab.database" :scroll-top="tab.sqlScrollTop" :scroll-left="tab.sqlScrollLeft" :production="queryConnection?.environment === 'production'" :running="running" @update:model-value="emit('updateSql', $event)" @update:execution-connection-id="emit('updateExecutionConnection', $event)" @update:default-database="emit('updateDefaultDatabase', $event)" @viewport="emit('viewport', $event)" @execute="(sql, newResultTab) => emit('execute', sql, newResultTab)" @explain="emit('explain', $event)" @create-smart-query="emit('createSmartQuery', $event)" @improve="emit('improve', $event)" @send-to-chat="aiAgent?.pasteQuery($event)" @new-query="emit('newQuery')" @save-query="emit('saveQuery')" />
      <div v-if="showAIAgent" class="w-1.5 shrink-0 cursor-col-resize bg-line hover:bg-accent" @pointerdown="resizeHorizontal" @dblclick="emit('hideAIAgent')" />
      <AIAgentPanel v-if="showAIAgent && queryConnectionId" ref="aiAgent" :tab-id="tab.id" :width="100 - editorWidth" :connection-id="queryConnectionId" :database="tab.database || queryConnection?.initialDatabase" :query="tab.sql" @apply="sqlEditor?.insertSQL($event)" @status="(tabId, status) => emit('aiStatus', tabId, status)" />
      <button v-else-if="aiConfigured" type="button" class="absolute inset-y-0 right-0 z-10 w-6 border-l border-line bg-panel text-xs font-medium text-muted hover:bg-canvas hover:text-ink" :title="t('aiAgent.title')" :aria-label="t('aiAgent.title')" style="writing-mode: vertical-rl" @click="emit('showAIAgent')">{{ t('aiAgent.title') }}</button>
    </div>
    <div class="h-1.5 shrink-0 cursor-row-resize bg-line hover:bg-accent" @pointerdown="resizeVertical" />
    <QueryResults :result-tabs="resultTabs" :active-result-tab-id="activeResultTabId" :loading="running" :loading-more="loadingMoreRows" :can-create-tab="true" :summary="resultSummary" @select-tab="emit('selectResultTab', $event)" @close-tab="emit('closeResultTab', $event)" @create-tab="emit('createResultTab')" @copy="emit('copyResult', $event)" @save="(id, result) => emit('saveResult', id, result)" @load-more="emit('loadMore')" @sort="(id, column, direction) => emit('sortResult', id, column, direction)" />
  </div>
</template>
