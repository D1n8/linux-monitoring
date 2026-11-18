<script setup lang="ts">
import { type Metric, type Point, valueText, clockTime, metricNames } from '~/composables/useMetrics'
const props = withDefaults(defineProps<{ points: Point[]; metric: Metric; from: number; to: number; color?: string; small?: boolean }>(), { color: 'var(--chart-cpu)', small: false })
const width = 900, height = 240, hover = ref<number | null>(null)
const max = computed(() => props.metric === 'rx' || props.metric === 'tx' ? Math.max(1024, ...props.points.flatMap(p => [p[props.metric] ?? 0, p[`${props.metric}_max`] ?? 0])) * 1.15 : 100)
const x = (time: string) => Math.max(0, Math.min(width, (Date.parse(time) - props.from) / Math.max(1, props.to - props.from) * width))
const y = (value: number) => height - Math.min(1, Math.max(0, value / max.value)) * height
const segments = computed(() => {
  const result: Point[][] = []; let current: Point[] = []
  for (const p of props.points) {
    const previous = current[current.length - 1]
    if (p[props.metric] == null || (previous && Date.parse(p.time) - Date.parse(previous.time) > Math.max(2500, (previous.step ?? 1) * 1500))) { if (current.length) result.push(current); current = [] }
    if (p[props.metric] != null) current.push(p)
  }
  if (current.length) result.push(current); return result
})
const lines = computed(() => segments.value.map(segment => ({
  line: segment.map((p, i) => `${i ? 'L' : 'M'}${x(p.time)},${y(p[props.metric]!)}`).join(' '),
  band: segment.map((p, i) => `${i ? 'L' : 'M'}${x(p.time)},${y(p[`${props.metric}_max`] ?? p[props.metric]!)}`).join(' ') + ' ' + [...segment].reverse().map(p => `L${x(p.time)},${y(p[`${props.metric}_min`] ?? p[props.metric]!)}`).join(' ') + 'Z'
})))
const selected = computed(() => hover.value == null ? null : props.points.reduce<Point | null>((best, p) => !best || Math.abs(x(p.time) - hover.value!) < Math.abs(x(best.time) - hover.value!) ? p : best, null))
function move(event: PointerEvent) { const rect = (event.currentTarget as SVGElement).getBoundingClientRect(); hover.value = (event.clientX - rect.left) / rect.width * width }
const ticks = computed(() => [0, 1, 2, 3, 4].map(i => ({ x: i * 25, label: new Date(props.from + (props.to - props.from) * i / 4).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }) })))
</script>
<template>
  <div class="metric-chart" :class="{ small }">
    <div v-if="!small" class="chart-axis" :class="{ rate: metric === 'rx' || metric === 'tx' }"><span v-for="i in [4,3,2,1,0]" :key="i">{{ metric === 'rx' || metric === 'tx' ? valueText(max * i / 4, metric) : `${i * 25}%` }}</span></div>
    <div class="chart-plot">
      <svg :viewBox="`0 0 ${width} ${height}`" preserveAspectRatio="none" role="img" :aria-label="`График: ${metricNames[metric]}`" @pointermove="move" @pointerleave="hover = null">
        <template v-if="!small"><line v-for="i in [0,1,2,3,4]" :key="i" x1="0" :y1="i * 60" :x2="width" :y2="i * 60" stroke="var(--grid)" stroke-dasharray="3 6" vector-effect="non-scaling-stroke" /></template>
        <g v-for="(line, index) in lines" :key="index"><path :d="line.band" :fill="color" opacity=".13" /><path :d="line.line" fill="none" :stroke="color" :stroke-width="small ? 1.6 : 2" vector-effect="non-scaling-stroke" stroke-linecap="round" stroke-linejoin="round" /></g>
        <template v-if="selected && !small"><line :x1="x(selected.time)" y1="0" :x2="x(selected.time)" :y2="height" stroke="var(--muted)" stroke-dasharray="3 4" /><circle v-if="selected[metric] != null" :cx="x(selected.time)" :cy="y(selected[metric]!)" r="4" :fill="color" /></template>
      </svg>
      <div v-if="!points.some(p => p[metric] != null) && !small" class="chart-empty">Нет измерений за выбранный период</div>
      <div v-if="selected && !small" class="chart-tooltip" :style="{ left: `${Math.min(75, Math.max(5, x(selected.time) / width * 100))}%` }"><span>{{ clockTime(selected.time) }}</span><strong>{{ valueText(selected[metric], metric) }}</strong><small v-if="selected[`${metric}_max`] != null">мин. {{ valueText(selected[`${metric}_min`], metric) }} · макс. {{ valueText(selected[`${metric}_max`], metric) }}</small></div>
      <div v-if="!small" class="chart-times"><span v-for="tick in ticks" :key="tick.x">{{ tick.label }}</span></div>
    </div>
  </div>
</template>
