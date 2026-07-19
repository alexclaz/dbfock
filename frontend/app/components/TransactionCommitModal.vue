<script setup lang="ts">
const props = defineProps<{ modelValue: boolean; connectionName: string; pendingStatements: number; committing?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; confirm: [] }>()
const { t } = useI18n()
</script>

<template>
  <Teleport to="body">
    <div v-if="props.modelValue" class="fixed inset-0 z-50 grid place-items-center bg-slate-950/40 p-4" @mousedown.self="emit('update:modelValue', false)">
      <section class="w-full max-w-md rounded-xl border border-line bg-panel p-5 shadow-panel" role="dialog" aria-modal="true" :aria-label="t('transaction.confirmTitle')">
        <h2 class="font-semibold">{{ t('transaction.confirmTitle') }}</h2>
        <p class="mt-2 text-sm leading-6 text-muted">{{ t('transaction.confirmDescription', { count: pendingStatements, name: connectionName }) }}</p>
        <p class="mt-3 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200">{{ t('transaction.confirmWarning') }}</p>
        <div class="mt-5 flex justify-end gap-2"><button class="rounded-md px-3 py-2 text-sm hover:bg-canvas" :disabled="committing" @click="emit('update:modelValue', false)">{{ t('transaction.cancel') }}</button><button class="rounded-md bg-amber-600 px-3 py-2 text-sm font-medium text-white disabled:opacity-60" :disabled="committing" @click="emit('confirm')">{{ committing ? t('transaction.committing') : t('transaction.confirm') }}</button></div>
      </section>
    </div>
  </Teleport>
</template>
