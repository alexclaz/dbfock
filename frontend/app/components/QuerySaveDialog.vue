<script setup lang="ts">
const props = defineProps<{
  modelValue: boolean
  initialValue: string
  title: string
  description: string
  label: string
  confirmLabel: string
  cancelLabel: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; confirm: [name: string] }>()

const name = ref(props.initialValue)
const input = ref<HTMLInputElement>()

watch(() => props.modelValue, (open) => {
  if (!open) return
  name.value = props.initialValue
  nextTick(() => input.value?.focus())
}, { immediate: true })

function submit() {
  const value = name.value.trim()
  if (value) emit('confirm', value)
}
</script>

<template>
  <Teleport to="body">
    <div v-if="modelValue" class="fixed inset-0 z-[60] grid place-items-center bg-slate-950/55 p-4 backdrop-blur-sm" @mousedown.self="emit('update:modelValue', false)">
      <form class="w-full max-w-md overflow-hidden rounded-2xl border border-line bg-panel shadow-2xl" role="dialog" aria-modal="true" :aria-label="title" @submit.prevent="submit" @keydown.esc.prevent="emit('update:modelValue', false)">
        <div class="p-6">
          <h2 class="text-base font-semibold tracking-tight">{{ title }}</h2>
          <p class="mt-2 text-sm leading-6 text-muted">{{ description }}</p>
          <label class="mt-5 grid gap-1.5 text-sm font-medium text-ink">
            {{ label }}
            <input ref="input" v-model="name" class="h-11 rounded-lg border border-line bg-canvas px-3 text-ink outline-none transition focus:border-accent focus:ring-4 focus:ring-accent/15" required maxlength="120" @focus="input?.select()" >
          </label>
        </div>
        <div class="flex flex-col-reverse gap-2 border-t border-line bg-canvas/40 px-5 py-4 sm:flex-row sm:justify-end">
          <button type="button" class="rounded-lg px-3.5 py-2 text-sm font-medium text-muted hover:bg-panel hover:text-ink focus-ring" @click="emit('update:modelValue', false)">{{ cancelLabel }}</button>
          <button type="submit" class="rounded-lg bg-accent px-3.5 py-2 text-sm font-semibold text-white shadow-sm hover:bg-accent/90 focus-ring">{{ confirmLabel }}</button>
        </div>
      </form>
    </div>
  </Teleport>
</template>
