<script setup lang="ts">
const { toasts, dismiss } = useToast()
const { t } = useI18n()

function toneClass(tone: 'success' | 'error' | 'info') {
  return tone === 'success' ? 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-100' : tone === 'error' ? 'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-200' : 'border-sky-200 bg-sky-50 text-sky-800 dark:border-sky-900 dark:bg-sky-950 dark:text-sky-100'
}
</script>

<template>
  <Teleport to="body"><div class="pointer-events-none fixed bottom-4 right-4 z-[80] flex w-full max-w-sm flex-col gap-2"><div v-for="toast in toasts" :key="toast.id" role="status" class="pointer-events-auto flex items-start gap-3 rounded-lg border px-4 py-3 text-sm shadow-panel" :class="toneClass(toast.tone)"><Icon :name="toast.tone === 'success' ? 'lucide:circle-check' : toast.tone === 'error' ? 'lucide:circle-alert' : 'lucide:info'" class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" /><span class="min-w-0 flex-1">{{ toast.message }}</span><button type="button" class="-mr-1 -mt-1 rounded p-1 leading-none hover:bg-black/5 dark:hover:bg-white/10" :aria-label="t('common.close')" @click="dismiss(toast.id)"><Icon name="lucide:x" class="h-4 w-4" aria-hidden="true" /></button></div></div></Teleport>
</template>
