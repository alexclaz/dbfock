<script setup lang="ts">
import type { WorkspaceTab } from '~/types/database'
import { workspaceIcon } from '~/utils/icons'

const props = defineProps<{ tabs: WorkspaceTab[]; activeId: string; canReopen: boolean }>()
const emit = defineEmits<{ select: [id: string]; close: [id: string]; closeRight: [id: string]; closeOthers: [id: string]; save: [id: string]; reopen: []; reorder: [id: string, targetId: string]; newQuery: [] }>()
const { t } = useI18n()
const contextMenu = ref<{ tab: WorkspaceTab; x: number; y: number }>()

function title(tab: WorkspaceTab) { return tab.type === 'welcome' ? t('search.home') : tab.type === 'saved' ? t('savedQueries.title') : tab.type === 'smart' ? t('smartQueries.title') : tab.type === 'settings' ? t('settings.title') : tab.title }
function isHome(tab: WorkspaceTab) { return tab.id === 'welcome' }
function isPinned(tab: WorkspaceTab) { return tab.id === 'welcome' || tab.id === 'saved-queries' || tab.id === 'settings' }
function aiStatusLabel(tab: WorkspaceTab) { return tab.aiStatus === 'running' ? t('tabs.aiRunning') : t('tabs.aiComplete') }
function startDrag(event: DragEvent, tab: WorkspaceTab) {
  if (isPinned(tab) || !event.dataTransfer) return
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData('text/plain', tab.id)
}
function allowMoveDrop(event: DragEvent) {
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
}
function selectTab(id: string) {
  contextMenu.value = undefined
  emit('select', id)
}
function openContextMenu(event: MouseEvent, tab: WorkspaceTab) {
  contextMenu.value = { tab, x: event.clientX, y: event.clientY }
}
function closeContextMenu() { contextMenu.value = undefined }
function closeTab(id: string) {
  closeContextMenu()
  emit('close', id)
}
function saveTab() {
  if (!contextMenu.value) return
  emit('save', contextMenu.value.tab.id)
  closeContextMenu()
}
function closeTabsToRight() {
  if (!contextMenu.value) return
  emit('closeRight', contextMenu.value.tab.id)
  closeContextMenu()
}
function closeOtherTabs() {
  if (!contextMenu.value) return
  emit('closeOthers', contextMenu.value.tab.id)
  closeContextMenu()
}
function reopenTab() {
  closeContextMenu()
  emit('reopen')
}
function hasTabsToRight(tab: WorkspaceTab) {
  const index = props.tabs.findIndex((item) => item.id === tab.id)
  return props.tabs.slice(index + 1).length > 0
}
function hasOtherClosableTabs(tab: WorkspaceTab) {
  return props.tabs.some((item) => item.id !== tab.id)
}

function onKeydown(event: KeyboardEvent) { if (event.key === 'Escape') closeContextMenu() }
onMounted(() => {
  window.addEventListener('click', closeContextMenu)
  window.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  window.removeEventListener('click', closeContextMenu)
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="flex h-10 overflow-hidden border-b border-line bg-panel" @click="closeContextMenu">
    <button v-for="tab in tabs" :key="tab.id" :draggable="!isPinned(tab)" class="group relative -mb-px flex h-10 min-w-0 flex-1 items-center gap-1.5 overflow-hidden border-b-2 border-r border-line px-3 text-sm transition-colors" :class="activeId === tab.id ? 'border-b-accent bg-accent/10 font-semibold text-ink shadow-[inset_0_1px_0_rgba(59,130,246,0.2)]' : 'border-b-transparent text-muted hover:bg-canvas/60 hover:text-ink'" :aria-current="activeId === tab.id ? 'page' : undefined" @click.stop="selectTab(tab.id)" @contextmenu.prevent.stop="openContextMenu($event, tab)" @dragstart="startDrag($event, tab)" @dragover.prevent="allowMoveDrop" @drop.prevent="{ const id = $event.dataTransfer?.getData('text/plain'); if (id) emit('reorder', id, tab.id) }">
      <Icon v-if="!isPinned(tab)" name="lucide:grip-vertical" class="h-4 w-4 cursor-grab text-muted" :title="t('tabs.dragToReorder')" aria-hidden="true" />
      <span class="grid h-5 w-5 shrink-0 place-items-center"><Icon :name="workspaceIcon(tab.type)" class="h-4 w-4" :class="tab.type === 'smart' ? 'text-violet-500' : ''" aria-hidden="true" /></span>
      <span class="min-w-0 flex-1 truncate whitespace-nowrap">{{ title(tab) }}<b v-if="tab.dirty" class="ml-1 text-accent">•</b></span>
      <span v-if="tab.aiStatus" class="inline-flex shrink-0 items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-semibold" :class="tab.aiStatus === 'running' ? 'bg-violet-500/15 text-violet-600 dark:text-violet-300' : 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300'" :title="aiStatusLabel(tab)"><i class="h-1.5 w-1.5 rounded-full" :class="tab.aiStatus === 'running' ? 'animate-pulse bg-violet-500' : 'bg-emerald-500'" /><Icon :name="tab.aiStatus === 'running' ? 'lucide:sparkles' : 'lucide:circle-check'" class="h-3 w-3" aria-hidden="true" /><span class="sr-only">{{ aiStatusLabel(tab) }}</span></span>
      <button v-if="!isHome(tab)" type="button" class="ml-1 shrink-0 rounded p-1 text-muted opacity-0 group-hover:opacity-100 hover:bg-line" :aria-label="t('common.close')" @click.stop="closeTab(tab.id)"><Icon name="lucide:x" class="h-3.5 w-3.5" aria-hidden="true" /></button>
    </button>
    <button type="button" class="-mb-px grid h-10 w-10 shrink-0 place-items-center border-b-2 border-b-transparent text-muted transition-colors hover:bg-canvas/60 hover:text-ink" :title="t('home.newQuery')" :aria-label="t('home.newQuery')" @click.stop="closeContextMenu(); emit('newQuery')"><Icon name="lucide:plus" class="h-4 w-4" aria-hidden="true" /></button>
  </div>

  <Teleport to="body">
    <div v-if="contextMenu" class="fixed z-50 min-w-56 rounded-md border border-line bg-panel p-1 shadow-lg" :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }" role="menu" @click.stop>
      <button type="button" role="menuitem" class="block w-full rounded px-3 py-2 text-left text-sm text-ink hover:bg-canvas disabled:cursor-not-allowed disabled:opacity-50" :disabled="!canReopen" @click="reopenTab">{{ t('tabs.reopen') }}</button>
      <button type="button" role="menuitem" class="block w-full rounded px-3 py-2 text-left text-sm text-ink hover:bg-canvas disabled:cursor-not-allowed disabled:opacity-50" :disabled="contextMenu.tab.type !== 'sql'" @click="saveTab">{{ t('common.save') }}</button>
      <button type="button" role="menuitem" class="block w-full rounded px-3 py-2 text-left text-sm text-ink hover:bg-canvas disabled:cursor-not-allowed disabled:opacity-50" :disabled="!hasTabsToRight(contextMenu.tab)" @click="closeTabsToRight">{{ t('tabs.closeToRight') }}</button>
      <button type="button" role="menuitem" class="block w-full rounded px-3 py-2 text-left text-sm text-ink hover:bg-canvas disabled:cursor-not-allowed disabled:opacity-50" :disabled="!hasOtherClosableTabs(contextMenu.tab)" @click="closeOtherTabs">{{ t('tabs.closeOthers') }}</button>
    </div>
  </Teleport>
</template>
