<script setup lang="ts">
import type { Connection, SavedQuery, SmartQuery, WorkspaceTab } from '~/types/database'
import { workspaceIcon } from '~/utils/icons'

type SettingsSection = 'appearance' | 'shortcuts' | 'connections' | 'ai' | 'audit' | 'backup'
type Command = {
  id: string
  label: string
  description: string
  group: string
  icon: string
  keywords?: string
  run: () => void
}

const props = defineProps<{
  tabs: WorkspaceTab[]
  activeTabId: string
  savedQueries: SavedQuery[]
  smartQueries: SmartQuery[]
  connections: Connection[]
}>()
const emit = defineEmits<{
  close: []
  selectTab: [id: string]
  openSavedQuery: [id: string]
  openSmartQuery: [id: string]
  newQuery: []
  openSettings: [section?: SettingsSection]
}>()

const search = ref('')
const selectedIndex = ref(0)
const searchInput = ref<HTMLInputElement>()
const { t } = useI18n()
const groups = computed(() => [t('search.actions'), t('search.openTabs'), t('search.savedQueries'), t('search.smartQueries'), t('search.settings')])

const connectionName = (id?: string) => props.connections.find((connection) => connection.id === id)?.name
const commands = computed<Command[]>(() => [
  { id: 'new-query', label: t('search.newQuery'), description: t('search.newQueryDescription'), group: t('search.actions'), icon: 'lucide:plus', keywords: 'new sql query consulta', run: () => emit('newQuery') },
  ...props.tabs.map((tab) => ({
    id: `tab:${tab.id}`,
    label: tab.title,
    description: tab.id === props.activeTabId ? t('search.currentTab') : tab.type === 'sql' ? `${t('search.query')}${connectionName(tab.connectionId) ? ` · ${connectionName(tab.connectionId)}` : ''}` : tab.type === 'table' ? `${t('search.table')} · ${tab.database || ''}` : tab.type === 'settings' ? t('search.settings') : t('search.home'),
    group: t('search.openTabs'),
    icon: workspaceIcon(tab.type),
    keywords: `${tab.type} ${tab.database || ''} ${tab.table || ''}`,
    run: () => emit('selectTab', tab.id),
  })),
  ...props.savedQueries.map((query) => ({
    id: `saved:${query.id}`,
    label: query.name,
    description: `${t('search.savedQuery')}${connectionName(query.connectionId) ? ` · ${connectionName(query.connectionId)}` : ''}`,
    group: t('search.savedQueries'),
    icon: 'lucide:bookmark',
    keywords: query.sql,
    run: () => emit('openSavedQuery', query.id),
  })),
  ...props.smartQueries.map((query) => ({
    id: `smart:${query.id}`,
    label: query.title,
    description: `${t('search.smartQuery')}${connectionName(query.connectionId) ? ` · ${connectionName(query.connectionId)}` : ''}`,
    group: t('search.smartQueries'),
    icon: 'lucide:sparkles',
    keywords: `${query.description} ${query.sql} ${query.sourceSql || ''}`,
    run: () => emit('openSmartQuery', query.id),
  })),
  { id: 'settings', label: t('search.settings'), description: t('search.openSettings'), group: t('search.settings'), icon: 'lucide:settings-2', keywords: 'settings preferencias', run: () => emit('openSettings') },
  { id: 'settings-appearance', label: t('settings.appearance'), description: t('search.chooseTheme'), group: t('search.settings'), icon: 'lucide:palette', keywords: 'tema theme dark light auto idioma language', run: () => emit('openSettings', 'appearance') },
  { id: 'settings-connections', label: t('settings.connections'), description: t('search.manageConnections'), group: t('search.settings'), icon: 'lucide:database', keywords: 'conexões connections exportar importar export import backup', run: () => emit('openSettings', 'connections') },
  { id: 'settings-ai', label: t('settings.aiAgent'), description: t('search.configureAi'), group: t('search.settings'), icon: 'lucide:sparkles', keywords: 'ai ia openai anthropic ollama modelo api', run: () => emit('openSettings', 'ai') },
  { id: 'settings-ai-audit', label: t('settings.aiAudit'), description: t('search.openAiAudit'), group: t('search.settings'), icon: 'lucide:scroll-text', keywords: 'ai ia audit auditoria logs prompt resposta response', run: () => emit('openSettings', 'audit') },
  { id: 'settings-backup', label: t('settings.backup'), description: t('search.manageBackup'), group: t('search.settings'), icon: 'lucide:cloud', keywords: 'backup s3 bucket restore restaurar sql', run: () => emit('openSettings', 'backup') },
])

const filteredCommands = computed(() => {
  const terms = search.value.toLocaleLowerCase().trim().split(/\s+/).filter(Boolean)
  if (!terms.length) return commands.value
  return commands.value.filter((command) => {
    const content = `${command.label} ${command.description} ${command.keywords || ''}`.toLocaleLowerCase()
    return terms.every((term) => content.includes(term))
  })
})
const groupedCommands = computed(() => groups.value.map((group) => ({ group, commands: filteredCommands.value.filter((command) => command.group === group) })).filter(({ commands }) => commands.length))

watch(search, () => { selectedIndex.value = 0 })
watch(filteredCommands, (items) => {
  if (selectedIndex.value >= items.length) selectedIndex.value = Math.max(0, items.length - 1)
})

function choose(command: Command) { command.run(); emit('close') }
function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') { emit('close'); return }
  if (!filteredCommands.value.length) return
  if (event.key === 'ArrowDown') { event.preventDefault(); selectedIndex.value = (selectedIndex.value + 1) % filteredCommands.value.length }
  else if (event.key === 'ArrowUp') { event.preventDefault(); selectedIndex.value = (selectedIndex.value - 1 + filteredCommands.value.length) % filteredCommands.value.length }
  else if (event.key === 'Enter') { event.preventDefault(); choose(filteredCommands.value[selectedIndex.value]!) }
}

onMounted(() => nextTick(() => searchInput.value?.focus()))
</script>

<template>
  <div class="fixed inset-0 z-50 grid place-items-center bg-slate-950/45 p-4 backdrop-blur-[2px]" @mousedown.self="$emit('close')">
    <section class="w-full max-w-2xl overflow-hidden rounded-xl border border-line bg-panel shadow-2xl" role="dialog" aria-modal="true" :aria-label="t('search.global')" @keydown="onKeydown">
      <div class="flex items-center gap-3 border-b border-line px-4">
        <Icon name="lucide:search" class="h-5 w-5 shrink-0 text-muted" aria-hidden="true" />
        <input ref="searchInput" v-model="search" class="h-14 min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted" :placeholder="t('search.placeholder')" :aria-label="t('search.search')" >
        <kbd class="rounded border border-line bg-canvas px-1.5 py-0.5 text-[11px] text-muted">ESC</kbd>
      </div>
      <div class="scrollbar max-h-[min(60vh,32rem)] overflow-y-auto p-2">
        <template v-for="entry in groupedCommands" :key="entry.group">
          <p class="px-2 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wider text-muted">{{ entry.group }}</p>
          <button v-for="command in entry.commands" :key="command.id" class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left" :class="filteredCommands[selectedIndex]?.id === command.id ? 'bg-accent text-white' : 'hover:bg-canvas'" @mouseenter="selectedIndex = filteredCommands.findIndex((item) => item.id === command.id)" @click="choose(command)">
            <span class="grid h-7 w-7 shrink-0 place-items-center rounded-md bg-canvas/70" :class="filteredCommands[selectedIndex]?.id === command.id ? 'bg-white/15 text-white' : 'text-muted'"><Icon :name="command.icon" class="h-4 w-4" aria-hidden="true" /></span>
            <span class="min-w-0 flex-1"><span class="block truncate text-sm font-medium">{{ command.label }}</span><span class="block truncate text-xs" :class="filteredCommands[selectedIndex]?.id === command.id ? 'text-white/75' : 'text-muted'">{{ command.description }}</span></span>
            <span v-if="filteredCommands[selectedIndex]?.id === command.id" class="text-xs text-white/80">↵</span>
          </button>
        </template>
        <p v-if="!filteredCommands.length" class="px-3 py-10 text-center text-sm text-muted">{{ t('search.noResults', { query: search }) }}</p>
      </div>
      <footer class="flex items-center gap-4 border-t border-line px-4 py-2 text-[11px] text-muted"><span><kbd class="rounded border border-line px-1">↑↓</kbd> {{ t('search.navigate') }}</span><span><kbd class="rounded border border-line px-1">↵</kbd> {{ t('search.open') }}</span><span class="ml-auto">{{ t('search.openShortcut') }}</span></footer>
    </section>
  </div>
</template>
