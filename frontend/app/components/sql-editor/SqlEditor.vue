<script setup lang="ts">
import { autocompletion, completionKeymap, startCompletion, type Completion, type CompletionContext } from '@codemirror/autocomplete'
import { defaultKeymap, history, historyKeymap, indentWithTab, toggleComment } from '@codemirror/commands'
import { sql, MySQL } from '@codemirror/lang-sql'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { EditorState, StateEffect, StateField } from '@codemirror/state'
import { Decoration, drawSelection, EditorView, keymap, lineNumbers, type DecorationSet } from '@codemirror/view'
import { tags } from '@lezer/highlight'
import type { ColumnInfo, Connection, DatabaseInfo, TableInfo } from '~/types/database'

const props = withDefaults(defineProps<{ modelValue: string; connectionId: string; connectionName: string; connections?: Connection[]; executionConnectionId?: string; initialDatabase?: string; running?: boolean; split?: boolean; width?: number | string; queryActions?: boolean; production?: boolean }>(), { connections: () => [] })
const emit = defineEmits<{ 'update:modelValue': [value: string]; 'update:executionConnectionId': [value?: string]; execute: [sql: string, newResultTab?: boolean]; explain: [sql: string]; improve: [sql: string]; createSmartQuery: [sql: string]; sendToChat: [sql: string]; newQuery: []; saveQuery: [] }>()
const api = useApi()
const { t } = useI18n()
const editorHost = ref<HTMLElement>()
const metadataState = ref<'idle' | 'loading' | 'ready' | 'error'>('idle')
const databases = ref<DatabaseInfo[]>([])
const tablesByDatabase = new Map<string, TableInfo[]>()
const columnsByTable = new Map<string, ColumnInfo[]>()
let view: EditorView | undefined
let syncing = false
let searchControls: HTMLDivElement | undefined
const selectedSQL = ref('')
const contextMenu = ref<{ x: number; y: number }>()
const copied = ref(false)
const searchQuery = ref('')
const searchMatchIndex = ref(-1)
const searchInput = ref<HTMLInputElement>()
const searchHighlightMark = Decoration.mark({ class: 'cm-searchMatch' })
const setSearchHighlights = StateEffect.define<DecorationSet>()
const searchHighlights = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update: (decorations, transaction) => {
    for (const effect of transaction.effects) if (effect.is(setSearchHighlights)) return effect.value
    return decorations.map(transaction.changes)
  },
  provide: (field) => EditorView.decorations.from(field),
})
const selectedExecutionConnectionId = computed(() => props.executionConnectionId === 'auto' ? props.connectionId : props.executionConnectionId || props.connectionId)
const connectionMenuOpen = ref(false)
const selectedExecutionConnection = computed(() => props.connections.find((connection) => connection.id === selectedExecutionConnectionId.value))

const keywords = ['SELECT', 'FROM', 'WHERE', 'JOIN', 'LEFT JOIN', 'RIGHT JOIN', 'INNER JOIN', 'ORDER BY', 'GROUP BY', 'HAVING', 'LIMIT', 'INSERT INTO', 'UPDATE', 'DELETE FROM', 'CREATE TABLE', 'ALTER TABLE', 'DROP TABLE', 'SHOW TABLES', 'DESCRIBE']

function updateExecutionConnection(connectionId: string | number) {
  emit('update:executionConnectionId', String(connectionId))
  connectionMenuOpen.value = false
}

async function loadDatabases() {
  if (!props.connectionId) { databases.value = []; metadataState.value = 'idle'; return }
  if (metadataState.value === 'loading' || metadataState.value === 'ready') return
  metadataState.value = 'loading'
  try { databases.value = (await api<DatabaseInfo[]>(`/connections/${props.connectionId}/databases`)) ?? []; metadataState.value = 'ready' }
  catch { metadataState.value = 'error' }
}
async function getTables(database: string) {
  if (!props.connectionId) return []
  if (tablesByDatabase.has(database)) return tablesByDatabase.get(database) || []
  const tables = (await api<TableInfo[]>(`/connections/${props.connectionId}/databases/${encodeURIComponent(database)}/tables`)) ?? []
  tablesByDatabase.set(database, tables)
  return tables
}
async function getColumns(database: string, table: string) {
  if (!props.connectionId) return []
  const key = `${database}.${table}`
  if (columnsByTable.has(key)) return columnsByTable.get(key) || []
  const structure = await api<{ columns: ColumnInfo[] }>(`/connections/${props.connectionId}/databases/${encodeURIComponent(database)}/tables/${encodeURIComponent(table)}/structure`)
  const columns = structure.columns ?? []
  columnsByTable.set(key, columns)
  return columns
}
function tableReferences(sqlText: string) {
  const references: { database: string; table: string; alias: string }[] = []
  const expression = /\b(?:from|join|update|into)\s+`?([A-Za-z_][\w$]*)(?:`?\s*\.\s*`?([A-Za-z_][\w$]*))?`?(?:\s+(?:as\s+)?([A-Za-z_][\w$]*))?/gi
  for (const match of sqlText.matchAll(expression)) {
    const database = match[2] ? match[1] : props.initialDatabase
    const table = match[2] || match[1]
    if (database && table) references.push({ database, table, alias: match[3] || table })
  }
  return references
}
async function completionSource(context: CompletionContext) {
  const word = context.matchBefore(/[A-Za-z0-9_$]*/)
  if (!context.explicit && !word?.text) return null
  const from = word?.from ?? context.pos
  const text = context.state.doc.toString()
  // Resolve the qualifier from the text *before* the word being typed, not up to the
  // cursor: once you've typed past the dot (e.g. "geral.USU"), a cursor-inclusive slice
  // never ends in "alias." and the qualifier would never be recognised.
  const before = text.slice(0, from)
  // Aliases/tables are only resolved from the current statement block (same blank-line
  // boundaries used to execute a statement), so unrelated queries elsewhere in the editor
  // don't leak their aliases into suggestions.
  const { from: blockStart, to: blockEnd } = statementRange(text, context.pos)
  const statementText = text.slice(blockStart, blockEnd)
  const options: Completion[] = keywords.map((label) => ({ label, type: 'keyword', boost: 2 }))
  await loadDatabases()
  options.push(...databases.value.map((database) => ({ label: database.name, type: 'namespace', detail: 'database', apply: `\`${database.name}\`` })))
  const qualifier = before.match(/([A-Za-z_][\w$]*)\.$/)?.[1]
  if (qualifier) {
    const reference = tableReferences(statementText).find((item) => item.alias === qualifier || item.table === qualifier)
    if (reference) {
      try { options.push(...(await getColumns(reference.database, reference.table)).map((column) => ({ label: column.name, type: 'property', detail: column.columnType }))) } catch { /* leave the normal suggestions available */ }
    } else {
      try { options.push(...(await getTables(qualifier)).map((table) => ({ label: table.name, type: 'class', detail: `${qualifier} table`, apply: `\`${table.name}\`` }))) } catch { /* leave the normal suggestions available */ }
    }
  } else {
    const database = props.initialDatabase || databases.value[0]?.name
    if (database) {
      try { options.push(...(await getTables(database)).map((table) => ({ label: table.name, type: 'class', detail: `${database} table`, apply: `\`${table.name}\`` }))) } catch { /* leave the normal suggestions available */ }
    }
    for (const reference of tableReferences(statementText)) {
      try { options.push(...(await getColumns(reference.database, reference.table)).map((column) => ({ label: `${reference.alias}.${column.name}`, type: 'property', detail: `${reference.table} · ${column.columnType}` }))) } catch { /* leave the normal suggestions available */ }
    }
  }
  return { from, options, validFor: /^[A-Za-z0-9_$]*$/ }
}
type ExecutionBlock = { from: number; to: number; sql: string }

// A blank line deliberately separates executable queries. Semicolons remain
// part of a block, so multi-statement scripts can still be executed together.
const blockSeparators = /\r?\n[\t ]*\r?\n+/g

function statementRange(source: string, cursor: number): { from: number; to: number } {
  let start = 0
  let end = source.length
  for (const match of source.matchAll(blockSeparators)) {
    const separatorStart = match.index!
    const separatorEnd = separatorStart + match[0].length
    if (separatorEnd <= cursor) {
      start = separatorEnd
      continue
    }
    if (separatorStart >= cursor || cursor < separatorEnd) {
      end = separatorStart
      break
    }
  }
  return { from: start, to: end }
}

function currentExecutionBlock(): ExecutionBlock | undefined {
  if (!view) {
    const sql = props.modelValue.trim()
    return sql ? { from: 0, to: props.modelValue.length, sql } : undefined
  }
  const cursor = view.state.selection.main.head
  const source = view.state.doc.toString()
  const { from: start, to: end } = statementRange(source, cursor)
  const raw = source.slice(start, end)
  const leading = raw.search(/\S/)
  if (leading < 0) return undefined
  const trailing = raw.length - raw.trimEnd().length
  const from = start + leading
  const to = end - trailing
  return { from, to, sql: source.slice(from, to) }
}
function runCurrentStatement(newResultTab = false) { const block = currentExecutionBlock(); if (block) emit('execute', block.sql, newResultTab) }
function toggleCurrentBlockComment() {
  const block = currentExecutionBlock()
  if (!view || !block) return false
  view.dispatch({ selection: { anchor: block.from, head: block.to } })
  return toggleComment(view)
}
function updateSelectedSQL() {
  if (!view) return
  const selection = view.state.selection.main
  selectedSQL.value = selection.empty ? '' : view.state.doc.sliceString(selection.from, selection.to).trim()
}
function closeContextMenu() { contextMenu.value = undefined }
function runSelectionAction(action: 'explain' | 'improve') {
  if (selectedSQL.value) {
    if (action === 'explain') emit('explain', selectedSQL.value)
    else emit('improve', selectedSQL.value)
  }
  closeContextMenu()
}
function createSmartQueryFromSelection() {
  if (selectedSQL.value) emit('createSmartQuery', selectedSQL.value)
  closeContextMenu()
}
function pasteSelectionToChat() {
  if (selectedSQL.value) emit('sendToChat', selectedSQL.value)
  closeContextMenu()
}
async function copySelection() {
  if (!selectedSQL.value || !navigator.clipboard) return
  try {
    await navigator.clipboard.writeText(selectedSQL.value)
    copied.value = true
    window.setTimeout(() => copied.value = false, 1500)
    return
  } catch { /* Clipboard access can be denied by the browser. */ }
  closeContextMenu()
}
async function pasteFromClipboard() {
  if (!view || !navigator.clipboard) return
  try {
    const text = await navigator.clipboard.readText()
    if (!text) return
    const selection = view.state.selection.main
    view.dispatch({ changes: { from: selection.from, to: selection.to, insert: text }, selection: { anchor: selection.from + text.length } })
    view.focus()
  } catch { /* Clipboard access can be denied by the browser. */ }
  closeContextMenu()
}
function insertSQL(sql: string) {
  if (!view || !sql.trim()) return
  const document = view.state.doc
  const query = sql.trim()
  const end = document.length
  const existing = document.sliceString(0, end)
  const separator = !existing ? '' : existing.endsWith('\n\n') ? '' : existing.endsWith('\n') ? '\n' : '\n\n'
  const inserted = separator + query
  const insertedFrom = end + separator.length

  view.dispatch({
    changes: { from: end, to: end, insert: inserted },
    selection: { anchor: insertedFrom, head: insertedFrom + query.length },
    effects: EditorView.scrollIntoView(insertedFrom, { y: 'center' }),
  })
  view.focus()
}
defineExpose({ insertSQL })
const clausePattern = /\b(LEFT OUTER JOIN|RIGHT OUTER JOIN|LEFT JOIN|RIGHT JOIN|INNER JOIN|CROSS JOIN|ORDER BY|GROUP BY|INSERT INTO|DELETE FROM|UNION ALL|SELECT|FROM|WHERE|HAVING|LIMIT|OFFSET|JOIN|UNION|UPDATE|SET|VALUES)\b/gi
function formatBlock(block: string) {
  const compact = block.trim().replace(/\s+/g, ' ').replace(/\s*,\s*/g, ', ')
  const withClauses = compact.replace(clausePattern, (_, clause: string) => `\n${clause.toUpperCase()}`).trim()
  return withClauses.replace(/^SELECT\s+(.+?)(?=\n(?:FROM|INTO)\b|$)/m, (_, columns: string) => {
    const items = columns.split(', ').map((column) => column.trim()).filter(Boolean)
    return items.length > 1 ? `SELECT\n  ${items.join(',\n  ')}` : `SELECT ${columns.trim()}`
  })
}
function format() {
  const source = view?.state.doc.toString() ?? props.modelValue
  const formatted = source.split(blockSeparators).map(formatBlock).filter(Boolean).join('\n\n')
  if (!view || view.state.doc.toString() === formatted) return
  view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: formatted } })
}

function searchMatches() {
  if (!view || !searchQuery.value) return []
  const source = view.state.doc.toString().toLocaleLowerCase()
  const query = searchQuery.value.toLocaleLowerCase()
  const matches: number[] = []
  let from = 0
  while (from < source.length) {
    const match = source.indexOf(query, from)
    if (match < 0) break
    matches.push(match)
    from = match + Math.max(query.length, 1)
  }
  return matches
}
function selectSearchMatch(index: number) {
  const matches = searchMatches()
  if (!view || !matches.length) { searchMatchIndex.value = -1; return }
  searchMatchIndex.value = (index + matches.length) % matches.length
  const from = matches[searchMatchIndex.value]!
  const to = from + searchQuery.value.length
  view.dispatch({ selection: { anchor: from, head: to }, effects: EditorView.scrollIntoView(from, { y: 'center' }) })
}
function updateSearchHighlights() {
  if (!view) return
  const decorations = searchMatches().map((from) => searchHighlightMark.range(from, from + searchQuery.value.length))
  view.dispatch({ effects: setSearchHighlights.of(Decoration.set(decorations, true)) })
}
function updateSearch() {
  searchMatchIndex.value = -1
  updateSearchHighlights()
  if (searchQuery.value) selectSearchMatch(0)
}
function focusSearch() { nextTick(() => searchInput.value?.focus()) }
function clearSearch() {
  searchQuery.value = ''
  searchMatchIndex.value = -1
  updateSearchHighlights()
  view?.focus()
}
const searchMatchLabel = computed(() => {
  const matches = searchMatches()
  return searchQuery.value ? `${matches.length ? searchMatchIndex.value + 1 : 0}/${matches.length}` : ''
})
function updateSearchControls() {
  if (!searchControls) return
  const count = searchControls.querySelector('[data-search-count]')
  const previous = searchControls.querySelector<HTMLButtonElement>('[data-search-previous]')
  const next = searchControls.querySelector<HTMLButtonElement>('[data-search-next]')
  if (count) count.textContent = searchMatchLabel.value
  if (previous) previous.disabled = !searchQuery.value
  if (next) next.disabled = !searchQuery.value
}
function mountSearchControls() {
  const actions = editorHost.value?.previousElementSibling?.lastElementChild
  if (!(actions instanceof HTMLElement)) return
  const controls = document.createElement('div')
  controls.className = 'flex h-7 items-center rounded border border-[rgb(var(--editor-line))] bg-[rgb(var(--editor-canvas))] text-xs focus-within:border-accent'
  const icon = document.createElement('span')
  icon.className = 'pl-2 text-[rgb(var(--editor-muted))]'
  icon.textContent = '⌕'
  const input = document.createElement('input')
  input.type = 'search'
  input.placeholder = t('editor.search')
  input.setAttribute('aria-label', t('editor.search'))
  input.className = 'w-24 bg-transparent px-1.5 text-xs text-[rgb(var(--editor-ink))] outline-none placeholder:text-[rgb(var(--editor-muted))] sm:w-32'
  const count = document.createElement('span')
  count.dataset.searchCount = ''
  count.className = 'text-[10px] tabular-nums text-[rgb(var(--editor-muted))]'
  const previous = document.createElement('button')
  previous.type = 'button'
  previous.dataset.searchPrevious = ''
  previous.className = 'px-1 text-[rgb(var(--editor-muted))] hover:text-[rgb(var(--editor-ink))] disabled:opacity-40'
  previous.textContent = '↑'
  previous.title = t('editor.previousMatch')
  const next = document.createElement('button')
  next.type = 'button'
  next.dataset.searchNext = ''
  next.className = 'px-1.5 text-[rgb(var(--editor-muted))] hover:text-[rgb(var(--editor-ink))] disabled:opacity-40'
  next.textContent = '↓'
  next.title = t('editor.nextMatch')
  input.addEventListener('input', () => { searchQuery.value = input.value; updateSearch(); updateSearchControls() })
  input.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') { event.preventDefault(); selectSearchMatch(searchMatchIndex.value + (event.shiftKey ? -1 : 1)); updateSearchControls() }
    if (event.key === 'Escape') { event.preventDefault(); input.value = ''; clearSearch(); updateSearchControls() }
  })
  previous.addEventListener('click', () => { selectSearchMatch(searchMatchIndex.value - 1); updateSearchControls() })
  next.addEventListener('click', () => { selectSearchMatch(searchMatchIndex.value + 1); updateSearchControls() })
  controls.append(icon, input, count, previous, next)
  actions.prepend(controls)
  searchControls = controls
  searchInput.value = input
  updateSearchControls()
}

const theme = EditorView.theme({
  '&': { height: '100%', backgroundColor: 'rgb(var(--editor-canvas))', color: 'rgb(var(--editor-ink))', fontSize: 'var(--ide-editor-font-size, 13px)' },
  '.cm-scroller': { fontFamily: 'JetBrains Mono, SFMono-Regular, Consolas, monospace', lineHeight: 'var(--ide-editor-line-height, 22px)' },
  '.cm-content': { padding: '10px 0' },
  '.cm-gutters': { backgroundColor: 'rgb(var(--editor-panel))', color: 'rgb(var(--editor-muted))', border: 'none' },
  '.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'rgb(var(--editor-active))' },
  '.cm-cursor': { borderLeftColor: 'rgb(var(--accent))' },
  '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': { backgroundColor: 'rgb(var(--editor-selection))' },
  '.cm-searchMatch': { backgroundColor: 'rgb(var(--accent) / 0.28)', borderRadius: '2px' },
  '.cm-tooltip': { backgroundColor: 'rgb(var(--editor-tooltip))', border: '1px solid rgb(var(--editor-line))' },
  '.cm-completionLabel': { color: 'rgb(var(--editor-ink))' },
}, { dark: true })
const highlights = HighlightStyle.define([
  { tag: tags.keyword, color: 'rgb(var(--syntax-key))', fontWeight: '700' },
  { tag: [tags.string, tags.special(tags.string)], color: 'rgb(var(--syntax-string))' },
  { tag: tags.number, color: 'rgb(var(--syntax-number))' },
  { tag: tags.comment, color: 'rgb(var(--syntax-comment))', fontStyle: 'italic' },
  { tag: tags.operator, color: 'rgb(var(--syntax-operator))' },
])

const contextMenuHandler = EditorView.domEventHandlers({
  contextmenu(event, editor) {
    const selection = editor.state.selection.main
    const sql = selection.empty ? '' : editor.state.doc.sliceString(selection.from, selection.to).trim()
    event.preventDefault()
    selectedSQL.value = sql
    contextMenu.value = { x: event.clientX, y: event.clientY }
    return true
  },
})
function closeContextMenuOnPointerDown(event: PointerEvent) {
  if (!(event.target as HTMLElement).closest('[data-sql-context-menu]')) closeContextMenu()
  if (!(event.target as HTMLElement).closest('[data-connection-menu]')) connectionMenuOpen.value = false
}
function closeContextMenuOnKeyDown(event: KeyboardEvent) { if (event.key === 'Escape') { closeContextMenu(); connectionMenuOpen.value = false } }

watch(() => props.modelValue, (value) => { if (!syncing && view && view.state.doc.toString() !== value) view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } }) })
watch(() => props.connectionId, () => { databases.value = []; tablesByDatabase.clear(); columnsByTable.clear(); metadataState.value = 'idle'; loadDatabases() })
onMounted(() => {
  view = new EditorView({ state: EditorState.create({ doc: props.modelValue, extensions: [lineNumbers(), searchHighlights, history(), drawSelection(), sql({ dialect: MySQL }), syntaxHighlighting(highlights), theme, contextMenuHandler, autocompletion({ override: [completionSource], activateOnTyping: true }), keymap.of([{ key: 'Mod-f', run: () => { focusSearch(); return true } }, { key: 'Mod-Enter', run: () => { runCurrentStatement(); return true } }, { key: 'Mod-\\', run: () => { runCurrentStatement(true); return true } }, { key: 'Mod-/', run: toggleCurrentBlockComment }, { key: 'Mod-Space', run: startCompletion }, indentWithTab, ...completionKeymap, ...historyKeymap, ...defaultKeymap]), EditorView.updateListener.of((update) => { if (update.docChanged) { syncing = true; emit('update:modelValue', update.state.doc.toString()); nextTick(() => { syncing = false; if (searchQuery.value) { updateSearch(); updateSearchControls() } }); updateSearchControls() }; if (update.selectionSet || update.docChanged) { updateSelectedSQL(); closeContextMenu() } })] }), parent: editorHost.value! })
  mountSearchControls()
  document.addEventListener('pointerdown', closeContextMenuOnPointerDown)
  document.addEventListener('keydown', closeContextMenuOnKeyDown)
  loadDatabases()
})
onBeforeUnmount(() => { view?.destroy(); searchControls?.remove(); document.removeEventListener('pointerdown', closeContextMenuOnPointerDown); document.removeEventListener('keydown', closeContextMenuOnKeyDown) })
</script>

<template><section class="flex min-h-0 flex-col bg-[rgb(var(--editor-panel))] text-[rgb(var(--editor-ink))]" :class="split ? 'h-full shrink-0 border-r border-line' : 'h-[46%] min-h-64 w-full shrink-0 border-b border-line'" :style="split ? { width: typeof width === 'number' ? `${width}%` : width ?? '50%' } : undefined"><div class="flex h-10 items-center justify-between border-b border-[rgb(var(--editor-line))] px-3"><div class="flex min-w-0 items-center gap-2 text-xs text-[rgb(var(--editor-muted))]"><div v-if="connections.length" data-connection-menu class="relative min-w-0"><button type="button" class="flex max-w-52 items-center gap-1.5 rounded border border-transparent bg-transparent px-1.5 py-1 text-xs text-[rgb(var(--editor-ink))] hover:border-[rgb(var(--editor-line))] focus:border-accent focus:outline-none" :aria-label="t('editor.connectionLabel')" :aria-expanded="connectionMenuOpen" aria-haspopup="listbox" @click="connectionMenuOpen = !connectionMenuOpen"><span class="truncate">{{ selectedExecutionConnection?.name || connectionName }}</span><Icon name="lucide:chevron-down" class="h-3.5 w-3.5 shrink-0 text-[rgb(var(--editor-muted))] transition-transform" :class="connectionMenuOpen ? 'rotate-180' : ''" aria-hidden="true" /></button><div v-if="connectionMenuOpen" class="absolute left-0 top-full z-30 mt-1 min-w-52 overflow-hidden rounded-md border border-[rgb(var(--editor-line))] bg-[rgb(var(--editor-panel))] p-1 shadow-lg" role="listbox"><button v-for="connection in connections" :key="connection.id" type="button" class="flex w-full items-center justify-between gap-3 rounded px-2 py-1.5 text-left text-xs text-[rgb(var(--editor-ink))] hover:bg-[rgb(var(--editor-active))]" :class="connection.id === selectedExecutionConnectionId ? 'font-medium text-accent' : ''" role="option" :aria-selected="connection.id === selectedExecutionConnectionId" @click="updateExecutionConnection(connection.id)"><span class="truncate">{{ connection.name }}</span><Icon v-if="connection.id === selectedExecutionConnectionId" name="lucide:check" class="h-3.5 w-3.5" aria-hidden="true" /></button></div></div><span v-else>{{ connectionName }}</span><span v-if="production" class="rounded bg-amber-500/15 px-1 py-0.5 text-[9px] font-semibold uppercase text-amber-700 dark:text-amber-300">{{ t('transaction.production') }}</span><span>· {{ metadataState === 'ready' ? t('editor.schemaReady') : t('editor.loadingSchema') }}</span></div><div class="flex gap-2"><template v-if="queryActions !== false"><button class="rounded px-2 py-1 text-xs text-[rgb(var(--editor-ink))] hover:bg-[rgb(var(--editor-active))]" @click="$emit('newQuery')">{{ t('editor.newQuery') }}</button><button class="rounded px-2 py-1 text-xs text-[rgb(var(--editor-ink))] hover:bg-[rgb(var(--editor-active))]" @click="$emit('saveQuery')">{{ t('editor.saveQuery') }}</button></template><button class="rounded px-2 py-1 text-xs text-[rgb(var(--editor-ink))] hover:bg-[rgb(var(--editor-active))]" @click="format">{{ t('editor.format') }}</button><button class="rounded bg-accent px-2.5 py-1 text-xs font-medium text-white disabled:opacity-50" :disabled="running" @click="() => runCurrentStatement()">{{ running ? t('editor.running') : t('editor.run') }}</button></div></div><div ref="editorHost" class="min-h-0 flex-1" /><div v-if="contextMenu" data-sql-context-menu class="fixed z-50 min-w-44 overflow-hidden rounded-md border border-line bg-panel py-1 shadow-lg" :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }" @contextmenu.prevent><button type="button" class="block w-full px-3 py-2 text-left text-sm text-ink hover:bg-canvas disabled:cursor-not-allowed disabled:opacity-50" :disabled="!selectedSQL" @click="runSelectionAction('explain')">{{ t('editor.explainQuery') }}</button><button type="button" class="block w-full px-3 py-2 text-left text-sm text-ink hover:bg-canvas disabled:cursor-not-allowed disabled:opacity-50" :disabled="!selectedSQL" @click="createSmartQueryFromSelection">{{ t('editor.createSmartQuery') }}</button><button type="button" class="block w-full px-3 py-2 text-left text-sm text-ink hover:bg-canvas disabled:cursor-not-allowed disabled:opacity-50" :disabled="!selectedSQL" @click="runSelectionAction('improve')">{{ t('editor.improveQuery') }}</button><div class="my-1 border-t border-line" /><button type="button" class="block w-full px-3 py-2 text-left text-sm text-ink hover:bg-canvas disabled:cursor-not-allowed disabled:opacity-50" :disabled="!selectedSQL" @click="copySelection">{{ copied ? t('editor.copied') : t('editor.copy') }}</button><button type="button" class="block w-full px-3 py-2 text-left text-sm text-ink hover:bg-canvas" @click="pasteFromClipboard">{{ t('editor.paste') }}</button><button type="button" class="block w-full px-3 py-2 text-left text-sm text-ink hover:bg-canvas disabled:cursor-not-allowed disabled:opacity-50" :disabled="!selectedSQL" @click="pasteSelectionToChat">{{ t('editor.pasteQueryToChat') }}</button></div></section></template>
