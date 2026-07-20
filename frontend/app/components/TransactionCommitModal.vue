<script setup lang="ts">
import type { PendingTransactionStatement } from '~/types/database'

const props = withDefaults(defineProps<{ modelValue: boolean; connectionName: string; statements: PendingTransactionStatement[]; committing?: boolean }>(), { statements: () => [] })
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; commit: [statementIds: string[]]; rollback: [statementIds: string[]] }>()
const { t } = useI18n()
const selected = ref<string[]>([])
const selectedCount = computed(() => selected.value.length)
const allSelected = computed(() => props.statements.length > 0 && selected.value.length === props.statements.length)

watch(() => props.statements, (statements) => { selected.value = selected.value.filter((id) => statements.some((statement) => statement.id === id)) }, { deep: true })
function toggleAll() { selected.value = allSelected.value ? [] : props.statements.map((statement) => statement.id) }
function toggleStatement(statementId: string) {
  if (props.committing) return
  selected.value = selected.value.includes(statementId)
    ? selected.value.filter((id) => id !== statementId)
    : [...selected.value, statementId]
}
function commit(statementIds: string[]) { if (statementIds.length) emit('commit', statementIds) }
function rollback(statementIds: string[]) { if (statementIds.length) emit('rollback', statementIds) }
</script>

<template>
  <Teleport to="body">
    <div v-if="props.modelValue" class="fixed inset-0 z-50 grid place-items-center bg-slate-950/40 p-4" @mousedown.self="emit('update:modelValue', false)">
      <section class="flex max-h-[calc(100vh-2rem)] w-full max-w-3xl flex-col rounded-xl border border-line bg-panel p-5 shadow-panel" role="dialog" aria-modal="true" :aria-label="t('transaction.confirmTitle')">
        <h2 class="font-semibold">{{ t('transaction.confirmTitle') }}</h2>
        <p class="mt-2 text-sm leading-6 text-muted">{{ t('transaction.confirmDescription', { count: statements.length, name: connectionName }) }}</p>
        <p class="mt-3 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200">{{ t('transaction.confirmWarning') }}</p>

        <div class="mt-4 min-h-0 overflow-auto rounded-lg border border-line">
          <label class="sticky top-0 flex cursor-pointer items-center gap-3 border-b border-line bg-panel px-3 py-2 text-xs font-medium text-muted">
            <input type="checkbox" :checked="allSelected" :disabled="committing || !statements.length" :aria-label="t('transaction.selectAll')" @change="toggleAll">
            <span>{{ t('transaction.pendingQueries') }}</span>
          </label>
          <div v-for="(statement, index) in statements" :key="statement.id" class="flex cursor-pointer gap-3 border-b border-line p-3 last:border-b-0" :class="{ 'cursor-not-allowed opacity-60': committing }" @click="toggleStatement(statement.id)">
            <input v-model="selected" type="checkbox" :value="statement.id" :disabled="committing" :aria-label="t('transaction.selectStatement', { count: index + 1 })" @click.stop>
            <div class="min-w-0 flex-1">
              <p class="mb-1 text-xs font-medium text-muted">{{ t('transaction.statement', { count: index + 1 }) }}</p>
              <pre class="overflow-x-auto whitespace-pre-wrap break-words rounded bg-canvas p-2 font-mono text-xs text-ink">{{ statement.sql }}</pre>
            </div>
          </div>
        </div>

        <div class="mt-4 flex flex-wrap items-center justify-between gap-3">
          <span class="text-xs text-muted">{{ t('transaction.selected', { count: selectedCount }) }}</span>
          <div class="flex flex-wrap justify-end gap-2">
            <button class="rounded-md border border-rose-300 px-3 py-2 text-sm text-rose-700 hover:bg-rose-500/10 disabled:opacity-60 dark:border-rose-900 dark:text-rose-300" :disabled="committing || !selectedCount" @click="rollback(selected)">{{ t('transaction.rollbackSelected') }}</button>
            <button class="rounded-md border border-line px-3 py-2 text-sm hover:bg-canvas disabled:opacity-60" :disabled="committing || !statements.length" @click="rollback(statements.map((statement) => statement.id))">{{ t('transaction.rollbackAll') }}</button>
            <button class="rounded-md bg-amber-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-60" :disabled="committing || !selectedCount" @click="commit(selected)">{{ committing ? t('transaction.committing') : t('transaction.confirmSelected') }}</button>
            <button class="rounded-md bg-amber-700 px-3 py-2 text-sm font-medium text-white disabled:opacity-60" :disabled="committing || !statements.length" @click="commit(statements.map((statement) => statement.id))">{{ committing ? t('transaction.committing') : t('transaction.confirm') }}</button>
            <button class="rounded-md px-3 py-2 text-sm hover:bg-canvas" :disabled="committing" @click="emit('update:modelValue', false)">{{ t('transaction.cancel') }}</button>
          </div>
        </div>
      </section>
    </div>
  </Teleport>
</template>
