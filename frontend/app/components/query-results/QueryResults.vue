<script setup lang="ts">
import type { QueryResult } from '~/types/database'

type ResultTab = {
  id: string
  title: string
  result?: QueryResult
  view: 'table' | 'json' | 'csv'
  copied: boolean
  editing: boolean
  sources?: { columns: string[] }[]
}

const props = withDefaults(defineProps<{ resultTabs: ResultTab[]; activeResultTabId?: string; loading?: boolean; loadingMore?: boolean; canCreateTab?: boolean; summary?: string }>(), { canCreateTab: false, summary: '' })
const emit = defineEmits<{ selectTab: [id: string]; closeTab: [id: string]; createTab: []; copy: [id: string]; save: [id: string, result: QueryResult]; loadMore: [] }>()
const { t } = useI18n()
const resultGrid = ref<{ save: () => boolean; cancel: () => void; canSave: boolean }>()
const activeResultTab = computed(() => props.resultTabs.find((tab) => tab.id === props.activeResultTabId))
</script>

<template>
  <div v-if="resultTabs.length" class="flex h-9 items-end gap-1 overflow-x-auto border-b border-line bg-panel px-2"><button v-for="resultTab in resultTabs" :key="resultTab.id" type="button" class="group flex h-8 shrink-0 items-center gap-1 rounded-t px-2 text-xs" :class="activeResultTab?.id === resultTab.id ? 'bg-canvas font-medium text-ink' : 'text-muted hover:bg-canvas/60'" @click="emit('selectTab', resultTab.id)"><span>{{ resultTab.title }}</span><span class="rounded p-1 opacity-0 group-hover:opacity-100 hover:bg-line" :aria-label="t('common.close')" @click.stop="emit('closeTab', resultTab.id)"><Icon name="lucide:x" class="h-3.5 w-3.5" aria-hidden="true" /></span></button><button v-if="canCreateTab" type="button" class="mb-1 grid h-6 w-6 shrink-0 place-items-center rounded text-muted hover:bg-canvas hover:text-ink" :title="t('query.newResultTab')" :aria-label="t('query.newResultTab')" @click="emit('createTab')"><Icon name="lucide:plus" class="h-4 w-4" aria-hidden="true" /></button></div>
  <div class="flex items-center justify-between border-b border-line px-4 py-2 text-xs text-muted"><span>{{ summary || t('query.results') }}</span><div v-if="activeResultTab" class="flex items-center gap-2"><button type="button" class="grid rounded p-1 hover:bg-canvas disabled:opacity-60" :title="activeResultTab.copied ? t('grid.copied') : t('grid.copy')" :aria-label="activeResultTab.copied ? t('grid.copied') : t('grid.copy')" :disabled="!activeResultTab.result" @click="emit('copy', activeResultTab.id)"><Icon :name="activeResultTab.copied ? 'lucide:check' : 'lucide:copy'" class="h-4 w-4" aria-hidden="true" /></button><div class="flex rounded-md border border-line p-0.5"><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'table' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'table'" @click="activeResultTab.view = 'table'">{{ t('grid.table') }}</button><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'json' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'json'" @click="activeResultTab.view = 'json'">JSON</button><button type="button" class="rounded px-2.5 py-1" :class="activeResultTab.view === 'csv' ? 'bg-canvas text-ink' : 'text-muted'" :aria-pressed="activeResultTab.view === 'csv'" @click="activeResultTab.view = 'csv'">CSV</button></div><template v-if="activeResultTab.editing"><button type="button" class="rounded-md bg-accent px-2.5 py-1 font-medium text-white disabled:cursor-not-allowed disabled:opacity-50" :disabled="!resultGrid?.canSave" @click="resultGrid?.save()">{{ t('grid.save') }}</button><button type="button" class="rounded-md border border-line px-2.5 py-1 text-ink" @click="activeResultTab.editing = false">{{ t('grid.cancel') }}</button></template></div></div>
  <div class="min-h-0 flex-1"><DataGrid ref="resultGrid" :result="activeResultTab?.result" :loading="loading" :loading-more="loadingMore" :view="activeResultTab?.view" :editing="activeResultTab?.editing" :editable="Boolean(activeResultTab?.sources?.length)" :editable-columns="activeResultTab?.sources?.flatMap((source) => source.columns)" :json-editable="!activeResultTab?.sources?.length" @load-more="emit('loadMore')" @start-edit="activeResultTab && (activeResultTab.editing = true)" @save="activeResultTab && emit('save', activeResultTab.id, $event)" @cancel="activeResultTab && (activeResultTab.editing = false)" /></div>
</template>
