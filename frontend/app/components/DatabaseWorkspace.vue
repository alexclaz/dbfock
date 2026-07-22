<script setup lang="ts">
import type { TableInfo } from '~/types/database'

const props = defineProps<{ connectionId: string; database: string }>()
const emit = defineEmits<{ table: [table: string] }>()
const api = useApi()
const { t } = useI18n()
const { error: notifyError } = useToast()
const tables = ref<TableInfo[]>()
const loading = ref(true)
const filter = ref('')

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

watch(() => [props.connectionId, props.database], load, { immediate: true })
</script>

<template>
  <section class="scrollbar h-full overflow-auto">
    <div class="flex min-h-full w-full flex-col px-5 py-6 lg:px-7">
      <header class="flex flex-wrap items-end justify-between gap-4 border-b border-line pb-5">
        <div>
          <div class="flex items-center gap-2"><Icon name="lucide:database" class="h-5 w-5 text-muted" aria-hidden="true" /><h1 class="text-xl font-semibold">{{ database }}</h1></div>
          <p class="mt-1 text-sm text-muted">{{ t('database.tablesCount', { count: tables?.length ?? 0 }) }}</p>
        </div>
        <label class="grid gap-1 text-xs font-medium text-muted"><span>{{ t('stats.filter') }}</span><input v-model="filter" class="field h-9 w-56" :placeholder="t('database.filterPlaceholder')" /></label>
      </header>

      <div v-if="loading" class="grid h-64 place-items-center text-sm text-muted">{{ t('tree.loadingTables') }}</div>
      <div v-else class="mt-5 overflow-hidden rounded-lg border border-line bg-panel">
        <div class="scrollbar overflow-auto">
          <table class="min-w-full text-left text-sm">
            <thead class="sticky top-0 bg-panel text-xs uppercase tracking-wide text-muted"><tr><th class="border-b border-line px-4 py-3 font-medium">{{ t('table.table') }}</th><th class="border-b border-line px-4 py-3 font-medium">{{ t('table.type') }}</th><th class="w-10 border-b border-line px-4 py-3" /></tr></thead>
            <tbody>
              <tr v-for="table in filteredTables" :key="table.name" class="cursor-pointer border-b border-line last:border-b-0 hover:bg-canvas" @dblclick="emit('table', table.name)">
                <td class="px-4 py-2.5 font-medium"><span class="flex items-center gap-2"><Icon name="lucide:table-2" class="h-3.5 w-3.5 shrink-0 text-muted" aria-hidden="true" />{{ table.name }}</span></td>
                <td class="px-4 py-2.5 text-muted">{{ table.type }}</td>
                <td class="px-4 py-2.5 text-right"><button type="button" class="grid h-6 w-6 place-items-center rounded text-muted hover:bg-line hover:text-ink" :title="t('tree.viewTable')" :aria-label="t('tree.viewTable')" @click.stop="emit('table', table.name)"><Icon name="lucide:eye" class="h-3.5 w-3.5" aria-hidden="true" /></button></td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!filteredTables.length" class="px-4 py-8 text-center text-sm text-muted">{{ filter ? t('stats.noMatches') : t('database.noTables') }}</p>
      </div>
    </div>
  </section>
</template>
