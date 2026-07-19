<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

type SelectValue = string | number

interface SelectOption {
  value: SelectValue
  label: string
  disabled?: boolean
}

const props = withDefaults(defineProps<{
  modelValue: SelectValue | undefined
  options: SelectOption[]
  disabled?: boolean
  placeholder?: string
}>(), { disabled: false, placeholder: 'Select an option' })

const emit = defineEmits<{
  'update:modelValue': [value: SelectValue]
  change: [value: SelectValue]
}>()

const root = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
const open = ref(false)
const activeIndex = ref(-1)

const selectedOption = computed(() => props.options.find((option) => option.value === props.modelValue))
const enabledIndexes = computed(() => props.options.reduce<number[]>((indexes, option, index) => {
  if (!option.disabled) indexes.push(index)
  return indexes
}, []))

function setOpen(value: boolean) {
  if (props.disabled) return
  open.value = value
  if (value) {
    activeIndex.value = props.options.findIndex((option) => option.value === props.modelValue && !option.disabled)
    if (activeIndex.value < 0) activeIndex.value = enabledIndexes.value[0] ?? -1
  }
}

function select(option: SelectOption) {
  if (option.disabled) return
  emit('update:modelValue', option.value)
  emit('change', option.value)
  open.value = false
  nextTick(() => trigger.value?.focus())
}

function moveActive(direction: 1 | -1) {
  const indexes = enabledIndexes.value
  if (!indexes.length) return
  const position = indexes.indexOf(activeIndex.value)
  activeIndex.value = indexes[(position + direction + indexes.length) % indexes.length]!
}

function handleKeydown(event: KeyboardEvent) {
  if (props.disabled) return
  if (event.key === 'Escape') { open.value = false; return }
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    if (!open.value) setOpen(true)
    else moveActive(event.key === 'ArrowDown' ? 1 : -1)
    return
  }
  if (event.key === 'Home' || event.key === 'End') {
    event.preventDefault()
    if (!open.value) setOpen(true)
    activeIndex.value = enabledIndexes.value[event.key === 'Home' ? 0 : enabledIndexes.value.length - 1] ?? -1
    return
  }
  if ((event.key === 'Enter' || event.key === ' ') && open.value) {
    event.preventDefault()
    const option = props.options[activeIndex.value]
    if (option) select(option)
  }
}

function closeOnOutsideClick(event: MouseEvent) {
  if (root.value && !root.value.contains(event.target as Node)) open.value = false
}

onMounted(() => document.addEventListener('mousedown', closeOnOutsideClick))
onBeforeUnmount(() => document.removeEventListener('mousedown', closeOnOutsideClick))
</script>

<template>
  <div ref="root" class="relative">
    <button ref="trigger" type="button" class="app-select" :disabled="disabled" :aria-expanded="open" aria-haspopup="listbox" @click="setOpen(!open)" @keydown="handleKeydown">
      <span class="truncate">{{ selectedOption?.label || placeholder }}</span>
      <svg class="h-4 w-4 shrink-0 text-muted transition-transform" :class="open ? 'rotate-180' : ''" viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" /></svg>
    </button>
    <Transition name="select-menu">
      <div v-if="open" class="app-select-menu" role="listbox" :aria-activedescendant="activeIndex >= 0 ? `app-select-option-${activeIndex}` : undefined">
        <button v-for="(option, index) in options" :id="`app-select-option-${index}`" :key="String(option.value)" type="button" class="app-select-option" :class="[option.value === modelValue ? 'app-select-option-selected' : '', index === activeIndex ? 'app-select-option-active' : '']" :disabled="option.disabled" role="option" :aria-selected="option.value === modelValue" @mouseenter="activeIndex = index" @click="select(option)">
          <span class="truncate">{{ option.label }}</span><span v-if="option.value === modelValue" aria-hidden="true">✓</span>
        </button>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.app-select { @apply flex h-10 w-full items-center justify-between gap-2 rounded-md border border-line bg-canvas px-3 text-left text-sm font-normal text-ink shadow-sm transition-colors hover:border-muted/60 focus:outline-none focus:ring-2 focus:ring-accent/50 disabled:cursor-not-allowed disabled:opacity-50; }
.app-select-menu { @apply absolute z-30 mt-1 max-h-56 w-full overflow-auto rounded-md border border-line bg-panel p-1 shadow-lg; }
.app-select-option { @apply flex w-full items-center justify-between gap-3 rounded px-2.5 py-2 text-left text-sm text-ink hover:bg-accent/10 focus:outline-none disabled:cursor-not-allowed disabled:opacity-50; }
.app-select-option-active { @apply bg-accent/10; }
.app-select-option-selected { @apply font-medium text-accent; }
.select-menu-enter-active, .select-menu-leave-active { transition: opacity 100ms ease, transform 100ms ease; }
.select-menu-enter-from, .select-menu-leave-to { opacity: 0; transform: translateY(-3px); }
</style>
