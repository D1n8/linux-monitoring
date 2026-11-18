import type { Ref } from 'vue'
export type Metric = 'cpu' | 'memory' | 'disk' | 'rx' | 'tx'
export type Sample = {
  time: string; cpu: number | null; memory: number | null; disk: number | null
  rx: number | null; tx: number | null; memory_total: number; memory_used: number
  load1: number | null; collection_ms: number; errors: Record<string, string>
  disks: { name: string; mount: string; total: number; used: number; percent: number }[]
  networks: { name: string; rx: number | null; tx: number | null }[]
}
export type Point = { time: string; count?: number; step?: number } & Record<Metric, number | null> & Partial<Record<`${Metric}_${'min' | 'max'}`, number | null>>
export type Health = { monitoring_enabled: boolean; storage_ok: boolean; storage_error?: string; last_saved: string | null; dropped: number; missed: number; queue: number; started: string }
export const metricNames: Record<Metric, string> = { cpu: 'Процессор', memory: 'Память', disk: 'Заполнение диска', rx: 'Входящий трафик', tx: 'Исходящий трафик' }
export function bytes(value: number | null | undefined, rate = false): string {
  if (value == null || !Number.isFinite(value)) return '—'
  const units = ['Б', 'КиБ', 'МиБ', 'ГиБ', 'ТиБ']; let i = 0
  while (value >= 1024 && i < units.length - 1) { value /= 1024; i++ }
  return `${value.toLocaleString('ru-RU', { maximumFractionDigits: i ? 1 : 0 })} ${units[i]}${rate ? '/с' : ''}`
}
export function valueText(value: number | null | undefined, metric: Metric): string {
  return value == null ? '—' : metric === 'rx' || metric === 'tx' ? bytes(value, true) : `${value.toLocaleString('ru-RU', { maximumFractionDigits: 1 })}%`
}
export const clockTime = (value: string) => new Date(value).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
export function useMetrics(historyActive: Ref<boolean> = ref(true)) {
  const sample = ref<Sample | null>(null), points = ref<Point[]>([])
  const host = ref<{ name: string; os: string; kernel: string; cores: number; uptime: number } | null>(null)
  const health = ref<Health | null>(null), connected = ref(false), paused = ref(false), range = ref(900), loading = ref(false)
  const error = ref(''), historyError = ref(''), now = ref(Date.now())
  const pausedAt = ref<number | null>(null)
  const end = computed(() => pausedAt.value ?? now.value), start = computed(() => Math.max(now.value - 86400000, end.value - range.value * 1000))
  const lastReceived = ref(0)
  const monitoringEnabled = computed(() => health.value?.monitoring_enabled !== false)
  const stale = computed(() => monitoringEnabled.value && now.value - lastReceived.value > 4000)
  let stream: EventSource | undefined, interval: ReturnType<typeof setInterval> | undefined, historyController: AbortController | undefined
  let disposed = false, metaBusy = false, metadataController: AbortController | undefined, lastMeta = 0, lastHistory = 0
  async function loadHistory() {
    if (disposed || !historyActive.value) return
    historyController?.abort(); const controller = new AbortController(); historyController = controller; loading.value = true
    try {
      const data = await $fetch<{ points: Point[]; step: number }>('/api/history', { query: { from: new Date(start.value).toISOString(), to: new Date(end.value).toISOString() }, signal: controller.signal, timeout: 7000 })
      if (disposed || controller.signal.aborted) return
      // Keep unflushed live samples at the tail; persisted buckets replace earlier points.
      const persistedEnd = data.points.length ? Date.parse(data.points[data.points.length - 1]!.time) + data.step * 1000 : start.value
      const tail = points.value.filter(p => Date.parse(p.time) >= persistedEnd && Date.parse(p.time) >= start.value)
      points.value = [...data.points.map(p => ({ ...p, step: data.step })), ...tail].slice(-1600); historyError.value = ''
    } catch { if (!controller.signal.aborted && !disposed) historyError.value = 'История недоступна. Проверьте состояние хранилища.' }
    finally { if (!controller.signal.aborted) loading.value = false }
  }
  async function loadMeta() {
    if (metaBusy || disposed) return; metaBusy = true; metadataController = new AbortController()
    const opts = { signal: metadataController.signal, timeout: 6000, retry: 0 }
    try {
      const results = await Promise.allSettled([
        $fetch<typeof host.value>('/api/host', opts),
        $fetch<Health>('/api/health', { ...opts, ignoreResponseError: true })
      ])
      if (disposed) return
      const [h, s] = results
      if (h.status === 'fulfilled') host.value = h.value
      if (s.status === 'fulfilled') health.value = s.value
      error.value = results.some(v => v.status === 'rejected') ? 'Часть данных недоступна. Повторяем подключение…' : ''
    } finally { metaBusy = false }
  }
  async function setMonitoring(enabled: boolean) {
    health.value = await $fetch<Health>('/api/service', { method: 'PUT', body: { enabled }, timeout: 15000, retry: 0 })
    if (enabled) { paused.value = false; void loadHistory() }
  }
  function goLive() { paused.value = false; void loadHistory() }
  watch(historyActive, (enabled) => { if (enabled) void loadHistory(); else { historyController?.abort(); loading.value = false } })
  watch(range, () => { points.value = []; void loadHistory() })
  watch(paused, (value) => { pausedAt.value = value ? Date.now() : null; if (!value) void loadHistory() })
  onMounted(() => {
    void loadMeta(); void loadHistory(); stream = new EventSource('/api/stream')
    stream.onopen = () => { connected.value = true; void loadHistory(); void loadMeta() }
    stream.addEventListener('service', (event: MessageEvent) => {
      health.value = JSON.parse(event.data) as Health
      connected.value = true
    })
    stream.addEventListener('metrics', (event: MessageEvent) => {
      const next = JSON.parse(event.data) as Sample
      if (!next.time || Date.parse(next.time) < 1000) return
      connected.value = true; lastReceived.value = Date.parse(next.time)
      if (!paused.value) {
        sample.value = next
        if (historyActive.value) {
          const time = Date.parse(next.time)
          points.value = [...points.value.filter(p => Date.parse(p.time) >= time - range.value * 1000 && p.time !== next.time), next].slice(-1600)
        }
      }
    })
    stream.onerror = () => { connected.value = false }
    interval = setInterval(() => {
      now.value = Date.now()
      if (now.value - lastMeta > 10000) { lastMeta = now.value; void loadMeta() }
      if (historyActive.value && !paused.value && monitoringEnabled.value && now.value - lastHistory > 30000) { lastHistory = now.value; void loadHistory() }
    }, 1000)
  })
  onBeforeUnmount(() => { disposed = true; stream?.close(); clearInterval(interval); historyController?.abort(); metadataController?.abort() })
  return { sample, points, host, health, connected, paused, range, loading, error, historyError, stale, monitoringEnabled, start, end, loadHistory, loadMeta, setMonitoring, goLive }
}
