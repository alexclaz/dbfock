<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

type SelectValue = string | number
type SelectOption = { value: SelectValue; label: string; disabled?: boolean }

const props = withDefaults(defineProps<{ modelValue: SelectValue[]; options: SelectOption[]; placeholder?: string }>(), { placeholder: 'Select options' })
const emit = defineEmits<{ 'update:modelValue': [value: SelectValue[]] }>()
const root = ref<HTMLElement>()
const open = ref(false)

function isSelected(value: SelectValue) { return props.modelValue.includes(value) }
function toggle(option: SelectOption) {
  if (option.disabled) return
  emit('update:modelValue', isSelected(option.value) ? props.modelValue.filter((value) => value !== option.value) : [...props.modelValue, option.value])
}
function label() {
  const selected = props.options.filter((option) => isSelected(option.value))
  if (!selected.length) return props.placeholder
  if (selected.length === 1) return selected[0]!.label
  return selected.map((option) => option.label).join(', ')
}
function closeOnOutsideClick(event: MouseEvent) { if (root.value && !root.value.contains(event.target as Node)) open.value = false }
onMounted(() => document.addEventListener('mousedown', closeOnOutsideClick))
onBeforeUnmount(() => document.removeEventListener('mousedown', closeOnOutsideClick))
</script>

<template>
  <div ref="root" class="relative">
    <button type="button" class="app-select" :aria-expanded="open" aria-haspopup="listbox" @click="open = !open"><span class="truncate">{{ label() }}</span><Icon name="lucide:chevron-down" class="h-4 w-4 shrink-0 text-muted transition-transform" :class="open ? 'rotate-180' : ''" aria-hidden="true" /></button>
    <Transition name="select-menu"><div v-if="open" class="app-select-menu" role="listbox" aria-multiselectable="true"><label v-for="option in options" :key="String(option.value)" class="app-select-option cursor-pointer" :class="option.disabled ? 'cursor-not-allowed opacity-50' : ''"><span class="flex min-w-0 items-center gap-2"><input type="checkbox" :checked="isSelected(option.value)" :disabled="option.disabled" class="accent-[var(--accent)]" @change="toggle(option)"><span class="truncate">{{ option.label }}</span></span></label></div></Transition>
  </div>
</template>

<style scoped>
.app-select { @apply flex h-10 w-full items-center justify-between gap-2 rounded-md border border-line bg-canvas px-3 text-left text-sm font-normal text-ink shadow-sm transition-colors hover:border-muted/60 focus:outline-none focus:ring-2 focus:ring-accent/50; }
.app-select-menu { @apply absolute z-30 mt-1 max-h-56 w-full overflow-auto rounded-md border border-line bg-panel p-1 shadow-lg; }
.app-select-option { @apply flex w-full items-center gap-3 rounded px-2.5 py-2 text-left text-sm text-ink hover:bg-accent/10; }
.select-menu-enter-active, .select-menu-leave-active { transition: opacity 100ms ease, transform 100ms ease; }
.select-menu-enter-from, .select-menu-leave-to { opacity: 0; transform: translateY(-3px); }
</style>
