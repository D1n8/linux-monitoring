<script setup lang="ts">
import { bytes, metricNames, valueText, type Metric, type Point } from '~/composables/useMetrics'
import { change, comparisonMetrics, localInput, summarize, validatePeriods, type Period } from '~/utils/comparison'

type History = { points: Point[]; step: number }
type Result = { before: Period; after: Period; a: History; b: History }
const beforeFrom = ref(''), beforeTo = ref(''), afterFrom = ref(''), afterTo = ref('')
const metric = ref<Metric>('cpu'), loading = ref(false), error = ref(''), result = shallowRef<Result | null>(null)
const preset = ref(900), browserZone = ref(''), today = ref(Date.now())
let controller: AbortController | undefined
const earliest = computed(() => localInput(today.value - 86400000)), latest = computed(() => localInput(today.value))
const metrics = computed(() => result.value ? comparisonMetrics.map(metric => {
  const a = summarize(result.value!.a.points, metric, result.value!.before)
  const b = summarize(result.value!.b.points, metric, result.value!.after)
  return { metric, a, b, delta: change(a.mean, b.mean) }
}) : [])
const hasGaps = computed(() => metrics.value.some(m => m.a.coverage < .95 || m.b.coverage < .95))
const scaleMax = computed(() => {
  if (metric.value !== 'rx' && metric.value !== 'tx') return 100
  return Math.max(1024, ...metrics.value.filter(m => m.metric === metric.value).flatMap(m => [m.a.max ?? 0, m.b.max ?? 0])) * 1.15
})
const chartData = computed(() => result.value ? [
  { title: 'До', period: result.value.before, points: result.value.a.points.map(p => ({ ...p, step: result.value!.a.step })) },
  { title: 'После', period: result.value.after, points: result.value.b.points.map(p => ({ ...p, step: result.value!.b.step })) }
] : [])
const selected = computed(() => metrics.value.find(m => m.metric === metric.value))
function setPreset(seconds: number) {
  preset.value = seconds; today.value = Date.now()
  // The latest complete minute avoids comparing an unflushed live tail.
  const anchor = Math.floor((Date.now() - 5000) / 60000) * 60000
  beforeFrom.value = localInput(anchor - seconds * 2000); beforeTo.value = localInput(anchor - seconds * 1000)
  afterFrom.value = beforeTo.value; afterTo.value = localInput(anchor)
}
async function compare() {
  controller?.abort(); loading.value = false; error.value = ''
  today.value = Date.now()
  const before = { from: Date.parse(beforeFrom.value), to: Date.parse(beforeTo.value) }
  const after = { from: Date.parse(afterFrom.value), to: Date.parse(afterTo.value) }
  const validation = validatePeriods(before, after)
  if (validation) { error.value = validation; return }
  const current = new AbortController(); controller = current; loading.value = true
  try {
    const fetchPeriod = (period: Period) => $fetch<History>('/api/history', { query: { from: new Date(period.from).toISOString(), to: new Date(period.to).toISOString() }, signal: current.signal, timeout: 7000, retry: 0 })
    const [a, b] = await Promise.all([fetchPeriod(before), fetchPeriod(after)])
    if (!current.signal.aborted) result.value = { before, after, a, b }
  } catch {
    if (!current.signal.aborted) { error.value = 'Не удалось загрузить оба периода. Проверьте состояние хранилища и повторите.'; current.abort() }
  } finally { if (controller === current) loading.value = false }
}
const periodText = (period: Period) => `${new Date(period.from).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })} — ${new Date(period.to).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })}`
function deltaText(value: number | null, key: Metric) {
  if (value == null) return '—'
  const sign = value > 0 ? '+' : value < 0 ? '−' : ''
  return key === 'rx' || key === 'tx' ? `${sign}${bytes(Math.abs(value), true)}` : `${sign}${Math.abs(value).toLocaleString('ru-RU', { maximumFractionDigits: 2 })} п.п.`
}
function relativeText(value: number | null) { return value == null ? 'Нет базы для %' : `${value > 0 ? '+' : ''}${value.toLocaleString('ru-RU', { maximumFractionDigits: 1 })}%` }
onMounted(() => { browserZone.value = Intl.DateTimeFormat().resolvedOptions().timeZone; setPreset(900); void compare() })
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <div class="comparison">
    <section class="panel comparison-form-panel">
      <form @submit.prevent="compare">
        <div class="comparison-toolbar"><div class="segmented" aria-label="Длительность сравнения"><button v-for="option in [{ label: '15 минут', value: 900 }, { label: '1 час', value: 3600 }, { label: '6 часов', value: 21600 }]" :key="option.value" type="button" :class="{ active: preset === option.value }" @click="setPreset(option.value)">{{ option.label }}</button></div><span class="muted">{{ browserZone }}</span></div>
        <div class="comparison-periods"><fieldset><legend>До</legend><label>Начало<input v-model="beforeFrom" type="datetime-local" required :min="earliest" :max="latest" @input="preset = 0" /></label><label>Конец<input v-model="beforeTo" type="datetime-local" required :min="earliest" :max="latest" @input="preset = 0" /></label></fieldset><fieldset><legend>После</legend><label>Начало<input v-model="afterFrom" type="datetime-local" required :min="earliest" :max="latest" @input="preset = 0" /></label><label>Конец<input v-model="afterTo" type="datetime-local" required :min="earliest" :max="latest" @input="preset = 0" /></label></fieldset></div>
        <div class="comparison-submit"><span class="muted">Одинаковая длительность, в пределах последних суток</span><button class="button primary" :disabled="loading">{{ loading ? 'Загрузка…' : 'Сравнить' }}</button></div>
      </form>
    </section>
    <p v-if="error" class="alert danger" role="alert">{{ error }}</p>
    <template v-if="result">
      <div class="comparison-result-heading"><h2>Результаты</h2><span class="muted">{{ periodText(result.before) }} / {{ periodText(result.after) }}</span></div>
      <p v-if="hasGaps" class="comparison-gap" role="status">В периодах есть пропуски. Средние рассчитаны по доступным измерениям; покрытие указано в таблице.</p>
      <div class="panel comparison-table-wrap"><table class="comparison-table"><thead><tr><th>Показатель</th><th>До, среднее</th><th>После, среднее</th><th>Изменение</th><th>Максимум до / после</th></tr></thead><tbody><tr v-for="row in metrics" :key="row.metric" :class="{ selected: metric === row.metric }"><th><button :aria-pressed="metric === row.metric" @click="metric = row.metric">{{ metricNames[row.metric] }}</button></th><td>{{ valueText(row.a.mean, row.metric) }}<small>Покрытие {{ Math.round(row.a.coverage * 100) }}%</small></td><td>{{ valueText(row.b.mean, row.metric) }}<small>Покрытие {{ Math.round(row.b.coverage * 100) }}%</small></td><td>{{ deltaText(row.delta.absolute, row.metric) }}<small>{{ relativeText(row.delta.relative) }}</small></td><td>{{ valueText(row.a.max, row.metric) }} / {{ valueText(row.b.max, row.metric) }}</td></tr></tbody></table></div>
      <div class="comparison-charts"><section v-for="chart in chartData" :key="chart.title" class="panel"><div class="panel-heading"><div><h2>{{ chart.title }}: {{ metricNames[metric] }}</h2><p>{{ periodText(chart.period) }}</p></div></div><MetricChart :points="chart.points" :metric="metric" :from="chart.period.from" :to="chart.period.to" :scale-max="scaleMax" color="var(--chart-cpu)" /><div class="comparison-chart-note">Среднее {{ valueText(chart.title === 'До' ? selected?.a.mean : selected?.b.mean, metric) }}</div></section></div>
    </template>
  </div>
</template>
