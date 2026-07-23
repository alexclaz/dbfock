<script setup lang="ts">
import type { SchemaDiagram, TableInfo } from '~/types/database'

type DatabaseSection = 'tables' | 'diagram'

const props = defineProps<{ connectionId: string; database: string; activeSection?: DatabaseSection }>()
const emit = defineEmits<{ table: [table: string]; 'update:activeSection': [value: DatabaseSection] }>()
const api = useApi()
const { t } = useI18n()
const { error: notifyError } = useToast()
const tables = ref<TableInfo[]>()
const loading = ref(true)
const filter = ref('')
const section = ref<DatabaseSection>(props.activeSection === 'diagram' ? 'diagram' : 'tables')
const diagram = ref<SchemaDiagram>()
const diagramLoading = ref(false)

const filteredTables = computed(() => {
  const list = tables.value ?? []
  const query = filter.value.trim().toLowerCase()
  return query ? list.filter((table) => table.name.toLowerCase().includes(query)) : list
})

async function load() {
  loading.value = true
  try { tables.value = await api<TableInfo[]>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/tables`) }
  catch (cause: any) { notifyError(cause.message) }
  finally { loading.value = false }
}
async function loadDiagram() {
  diagramLoading.value = true
  try { diagram.value = await api<SchemaDiagram>(`/connections/${props.connectionId}/databases/${encodeURIComponent(props.database)}/diagram`) }
  catch (cause: any) { notifyError(cause.message) }
  finally { diagramLoading.value = false }
}
function selectSection(next: DatabaseSection) {
  section.value = next
  emit('update:activeSection', next)
  if (next === 'diagram' && !diagram.value) void loadDiagram()
}

watch(() => [props.connectionId, props.database], () => { load(); diagram.value = undefined }, { immediate: true })
</script>

<template>
  <section class="flex h-full min-h-0 flex-col">
    <header class="space-y-3 border-b border-line px-5 py-4 lg:px-7">
      <div class="flex items-center gap-2"><Icon name="lucide:database" class="h-5 w-5 text-muted" aria-hidden="true" /><h1 class="text-xl font-semibold">{{ database }}</h1></div>
      <div class="inline-flex rounded-md border border-line p-0.5 text-xs">
          <button type="button" class="rounded px-2.5 py-1" :class="section === 'tables' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('tables')">{{ t('database.viewTables') }}</button>
          <button type="button" class="rounded px-2.5 py-1" :class="section === 'diagram' ? 'bg-canvas text-ink' : 'text-muted'" @click="selectSection('diagram')">{{ t('database.viewDiagram') }}</button>
      </div>
    </header>
    <div v-if="section === 'tables'" class="border-b border-line bg-canvas/40 px-5 py-3 lg:px-7">
      <label class="flex w-full max-w-md items-center gap-2 rounded-lg border border-line bg-panel px-3 py-2 text-muted shadow-sm">
        <Icon name="lucide:search" class="h-4 w-4 shrink-0" aria-hidden="true" />
        <span class="sr-only">{{ t('stats.filter') }}</span>
        <input v-model="filter" class="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted" :placeholder="t('database.filterPlaceholder')" />
      </label>
    </div>

    <div v-if="section === 'tables'" class="scrollbar min-h-0 flex-1 overflow-auto px-5 py-6 lg:px-7">
      <div v-if="loading" class="grid h-64 place-items-center text-sm text-muted">{{ t('tree.loadingTables') }}</div>
      <div v-else class="overflow-hidden rounded-lg border border-line bg-panel">
        <div class="scrollbar overflow-auto">
          <table class="min-w-full text-left text-sm">
            <thead class="sticky top-0 bg-panel text-xs uppercase tracking-wide text-muted"><tr><th class="border-b border-line px-4 py-3 font-medium">{{ t('table.table') }}</th><th class="border-b border-line px-4 py-3 font-medium">{{ t('table.columns') }}</th><th class="w-10 border-b border-line px-4 py-3" /></tr></thead>
            <tbody>
              <tr v-for="table in filteredTables" :key="table.name" class="cursor-pointer border-b border-line last:border-b-0 hover:bg-canvas" @dblclick="emit('table', table.name)">
                <td class="px-4 py-2.5 font-medium"><span class="flex items-center gap-2"><Icon name="lucide:table-2" class="h-3.5 w-3.5 shrink-0 text-muted" aria-hidden="true" />{{ table.name }}</span></td>
                <td class="px-4 py-2.5 text-muted">{{ table.columnCount }}</td>
                <td class="px-4 py-2.5 text-right"><button type="button" class="grid h-6 w-6 place-items-center rounded text-muted hover:bg-line hover:text-ink" :title="t('tree.viewTable')" :aria-label="t('tree.viewTable')" @click.stop="emit('table', table.name)"><Icon name="lucide:eye" class="h-3.5 w-3.5" aria-hidden="true" /></button></td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!filteredTables.length" class="px-4 py-8 text-center text-sm text-muted">{{ filter ? t('stats.noMatches') : t('database.noTables') }}</p>
      </div>
    </div>
    <div v-else class="min-h-0 flex-1"><ErDiagram :tables="diagram?.tables ?? []" :loading="diagramLoading" @open-table="emit('table', $event)" /></div>
  </section>
</template>
