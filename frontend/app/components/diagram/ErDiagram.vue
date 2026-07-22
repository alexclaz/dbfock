<script setup lang="ts">
import type { DiagramTable } from '~/types/database'

const props = defineProps<{ tables: DiagramTable[]; focusTable?: string; loading?: boolean }>()
const emit = defineEmits<{ 'open-table': [name: string] }>()
const { t } = useI18n()

const HEADER_HEIGHT = 30
const ROW_HEIGHT = 22
const BOX_WIDTH = 220
const GAP_X = 90
const GAP_Y = 36
const MARGIN = 40

interface Box { name: string; x: number; y: number; width: number; height: number; table: DiagramTable; pkColumns: Set<string>; fkColumns: Set<string> }

// Tables are arranged left-to-right by FK dependency depth (referenced tables first), like a simplified Sugiyama layered graph layout.
const layout = computed(() => {
  const byName = new Map(props.tables.map((table) => [table.name, table]))
  const memo = new Map<string, number>()
  function layerOf(name: string, visiting: Set<string>): number {
    if (memo.has(name)) return memo.get(name)!
    if (visiting.has(name)) return 0
    visiting.add(name)
    const table = byName.get(name)
    let layer = 0
    if (table) {
      for (const fk of table.foreignKeys) {
        if (fk.referencedTable === name || !byName.has(fk.referencedTable)) continue
        layer = Math.max(layer, layerOf(fk.referencedTable, visiting) + 1)
      }
    }
    visiting.delete(name)
    memo.set(name, layer)
    return layer
  }

  const layers = new Map<number, DiagramTable[]>()
  for (const table of props.tables) {
    const layer = layerOf(table.name, new Set())
    if (!layers.has(layer)) layers.set(layer, [])
    layers.get(layer)!.push(table)
  }

  const boxes = new Map<string, Box>()
  for (const [layer, list] of [...layers.entries()].sort((a, b) => a[0] - b[0])) {
    list.sort((a, b) => a.name.localeCompare(b.name))
    let y = MARGIN
    for (const table of list) {
      const height = HEADER_HEIGHT + Math.max(1, table.columns.length) * ROW_HEIGHT + 8
      boxes.set(table.name, {
        name: table.name,
        x: MARGIN + layer * (BOX_WIDTH + GAP_X),
        y,
        width: BOX_WIDTH,
        height,
        table,
        pkColumns: new Set(table.columns.filter((c) => c.key === 'PRI').map((c) => c.name)),
        fkColumns: new Set(table.foreignKeys.map((fk) => fk.column)),
      })
      y += height + GAP_Y
    }
  }

  const maxLayer = layers.size ? Math.max(...layers.keys()) : 0
  const width = MARGIN * 2 + (maxLayer + 1) * BOX_WIDTH + maxLayer * GAP_X
  const height = MARGIN * 2 + Math.max(0, ...[...boxes.values()].map((box) => box.y + box.height - MARGIN))
  return { boxes, width, height }
})

const edges = computed(() => {
  const { boxes } = layout.value
  const list: { id: string; path: string; dimmed: boolean }[] = []
  for (const table of props.tables) {
    const source = boxes.get(table.name)
    if (!source) continue
    table.foreignKeys.forEach((fk, index) => {
      const target = boxes.get(fk.referencedTable)
      if (!target) return
      const referencedTable = props.tables.find((t) => t.name === fk.referencedTable)
      const sourceRow = Math.max(0, table.columns.findIndex((c) => c.name === fk.column))
      const targetRow = Math.max(0, referencedTable?.columns.findIndex((c) => c.name === fk.referencedColumn) ?? 0)
      const sourceY = source.y + HEADER_HEIGHT + sourceRow * ROW_HEIGHT + ROW_HEIGHT / 2
      const targetY = target.y + HEADER_HEIGHT + targetRow * ROW_HEIGHT + ROW_HEIGHT / 2
      const forward = target.x >= source.x
      const sourceX = forward ? source.x + source.width : source.x
      const targetX = forward ? target.x : target.x + target.width
      const curve = Math.max(40, Math.abs(targetX - sourceX) / 2)
      const c1x = sourceX + (forward ? curve : -curve)
      const c2x = targetX + (forward ? -curve : curve)
      const path = `M${sourceX},${sourceY} C${c1x},${sourceY} ${c2x},${targetY} ${targetX},${targetY}`
      const related = !props.focusTable || table.name === props.focusTable || fk.referencedTable === props.focusTable
      list.push({ id: `${table.name}:${fk.name}:${index}`, path, dimmed: !related })
    })
  }
  return list
})

const scale = ref(1)
const translateX = ref(0)
const translateY = ref(0)
const viewport = ref<HTMLElement>()
let panning = false
let lastX = 0
let lastY = 0

function onPointerDown(event: PointerEvent) {
  if (event.button !== 0) return
  if ((event.target as HTMLElement).closest('[data-er-box]')) return
  panning = true
  lastX = event.clientX
  lastY = event.clientY
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}
function onPointerMove(event: PointerEvent) {
  if (!panning) return
  translateX.value += event.clientX - lastX
  translateY.value += event.clientY - lastY
  lastX = event.clientX
  lastY = event.clientY
}
function onPointerUp(event: PointerEvent) {
  panning = false
  ;(event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId)
}
function onWheel(event: WheelEvent) {
  event.preventDefault()
  if (!viewport.value) return
  const rect = viewport.value.getBoundingClientRect()
  const px = event.clientX - rect.left
  const py = event.clientY - rect.top
  const factor = event.deltaY > 0 ? 0.9 : 1.1
  const next = Math.min(2.5, Math.max(0.2, scale.value * factor))
  const worldX = (px - translateX.value) / scale.value
  const worldY = (py - translateY.value) / scale.value
  translateX.value = px - worldX * next
  translateY.value = py - worldY * next
  scale.value = next
}
function zoomBy(factor: number) {
  if (!viewport.value) { scale.value = Math.min(2.5, Math.max(0.2, scale.value * factor)); return }
  const rect = viewport.value.getBoundingClientRect()
  const px = rect.width / 2
  const py = rect.height / 2
  const next = Math.min(2.5, Math.max(0.2, scale.value * factor))
  const worldX = (px - translateX.value) / scale.value
  const worldY = (py - translateY.value) / scale.value
  translateX.value = px - worldX * next
  translateY.value = py - worldY * next
  scale.value = next
}
function fitToView() {
  if (!viewport.value) return
  const rect = viewport.value.getBoundingClientRect()
  const { width, height } = layout.value
  if (!width || !height || !rect.width || !rect.height) return
  const next = Math.min(1, (rect.width - 40) / width, (rect.height - 40) / height)
  scale.value = Math.max(0.15, next)
  translateX.value = (rect.width - width * scale.value) / 2
  translateY.value = (rect.height - height * scale.value) / 2
}

watch(() => props.tables, () => nextTick(fitToView))
onMounted(() => nextTick(fitToView))
</script>

<template>
  <div
    ref="viewport"
    class="relative h-full w-full overflow-hidden bg-canvas"
    :class="panning ? 'cursor-grabbing' : 'cursor-grab'"
    @pointerdown="onPointerDown"
    @pointermove="onPointerMove"
    @pointerup="onPointerUp"
    @pointercancel="onPointerUp"
    @wheel="onWheel"
  >
    <div v-if="loading" class="absolute inset-0 grid place-items-center text-sm text-muted">{{ t('diagram.loading') }}</div>
    <div v-else-if="!tables.length" class="absolute inset-0 grid place-items-center text-sm text-muted">{{ t('diagram.empty') }}</div>
    <div
      class="absolute left-0 top-0 origin-top-left"
      :style="{ width: `${layout.width}px`, height: `${layout.height}px`, transform: `translate(${translateX}px, ${translateY}px) scale(${scale})` }"
    >
      <svg :width="layout.width" :height="layout.height" class="absolute left-0 top-0 overflow-visible">
        <defs>
          <marker id="er-arrow-dim" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M0,0 L8,4 L0,8 Z" class="fill-line" />
          </marker>
          <marker id="er-arrow-active" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M0,0 L8,4 L0,8 Z" class="fill-accent" />
          </marker>
        </defs>
        <path
          v-for="edge in edges"
          :key="edge.id"
          :d="edge.path"
          fill="none"
          :class="edge.dimmed ? 'stroke-line' : 'stroke-accent'"
          :stroke-width="edge.dimmed ? 1.25 : 1.75"
          :opacity="edge.dimmed ? 0.5 : 1"
          :marker-end="edge.dimmed ? 'url(#er-arrow-dim)' : 'url(#er-arrow-active)'"
        />
      </svg>
      <div
        v-for="box in layout.boxes.values()"
        :key="box.name"
        data-er-box
        class="absolute overflow-hidden rounded-md border bg-panel shadow-panel"
        :class="box.name === focusTable ? 'border-accent ring-1 ring-accent' : 'border-line'"
        :style="{ left: `${box.x}px`, top: `${box.y}px`, width: `${box.width}px` }"
      >
        <div
          class="flex cursor-pointer items-center gap-1.5 truncate border-b px-2 py-1.5 text-xs font-semibold"
          :class="box.name === focusTable ? 'border-accent/40 bg-accent/10 text-accent' : 'border-line text-ink'"
          :title="box.name"
          @dblclick="emit('open-table', box.name)"
        >
          <Icon name="lucide:table-2" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
          <span class="truncate">{{ box.name }}</span>
        </div>
        <div>
          <div
            v-for="column in box.table.columns"
            :key="column.name"
            class="flex items-center gap-1.5 truncate border-b border-line/60 px-2 text-[11px] last:border-b-0"
            :style="{ height: `${ROW_HEIGHT}px` }"
          >
            <Icon v-if="box.pkColumns.has(column.name)" name="lucide:key-round" class="h-3 w-3 shrink-0 text-amber-500" aria-hidden="true" />
            <Icon v-else-if="box.fkColumns.has(column.name)" name="lucide:link-2" class="h-3 w-3 shrink-0 text-accent" aria-hidden="true" />
            <span v-else class="w-3 shrink-0" />
            <span class="truncate" :class="box.pkColumns.has(column.name) ? 'font-semibold text-ink' : 'text-ink'">{{ column.name }}</span>
            <span class="ml-auto shrink-0 truncate text-muted">{{ column.columnType }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="absolute bottom-3 right-3 flex items-center gap-1 rounded-md border border-line bg-panel p-1 shadow-panel">
      <button type="button" class="grid h-7 w-7 place-items-center rounded hover:bg-canvas" @click="zoomBy(1.2)"><Icon name="lucide:zoom-in" class="h-4 w-4" aria-hidden="true" /></button>
      <button type="button" class="grid h-7 w-7 place-items-center rounded hover:bg-canvas" @click="zoomBy(1 / 1.2)"><Icon name="lucide:zoom-out" class="h-4 w-4" aria-hidden="true" /></button>
      <button type="button" class="grid h-7 w-7 place-items-center rounded hover:bg-canvas" @click="fitToView"><Icon name="lucide:maximize" class="h-4 w-4" aria-hidden="true" /></button>
    </div>
  </div>
</template>
