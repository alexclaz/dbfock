<script setup lang="ts">
withDefaults(defineProps<{
  modelValue: boolean
  title: string
  description: string
  confirmLabel: string
  cancelLabel: string
  tone?: 'default' | 'danger'
}>(), { tone: 'default' })

const emit = defineEmits<{ 'update:modelValue': [value: boolean]; confirm: [] }>()
const confirmButton = ref<HTMLButtonElement>()

onMounted(() => nextTick(() => confirmButton.value?.focus()))
</script>

<template>
  <Teleport to="body">
    <div v-if="modelValue" class="fixed inset-0 z-[60] grid place-items-center bg-slate-950/55 p-4 backdrop-blur-sm" @mousedown.self="emit('update:modelValue', false)">
      <section class="w-full max-w-md overflow-hidden rounded-2xl border border-line bg-panel shadow-2xl" role="dialog" aria-modal="true" :aria-label="title" @keydown.esc.prevent="emit('update:modelValue', false)">
        <div class="p-6">
          <h2 class="text-base font-semibold tracking-tight">{{ title }}</h2>
          <p class="mt-2 text-sm leading-6 text-muted">{{ description }}</p>
        </div>
        <div class="flex flex-col-reverse gap-2 border-t border-line bg-canvas/40 px-5 py-4 sm:flex-row sm:justify-end">
          <button type="button" class="rounded-lg px-3.5 py-2 text-sm font-medium text-muted hover:bg-panel hover:text-ink focus-ring" @click="emit('update:modelValue', false)">{{ cancelLabel }}</button>
          <button ref="confirmButton" type="button" class="rounded-lg px-3.5 py-2 text-sm font-semibold text-white shadow-sm focus-ring" :class="tone === 'danger' ? 'bg-rose-600 hover:bg-rose-700' : 'bg-accent hover:bg-accent/90'" @click="emit('confirm')">{{ confirmLabel }}</button>
        </div>
      </section>
    </div>
  </Teleport>
</template>
