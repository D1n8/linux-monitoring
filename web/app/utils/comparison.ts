import type { Metric, Point } from '../composables/useMetrics'

export type Period = { from: number; to: number }
export type MetricSummary = { mean: number | null; min: number | null; max: number | null; samples: number; coverage: number }
export const comparisonMetrics: Metric[] = ['cpu', 'memory', 'disk', 'rx', 'tx']

export function validatePeriods(before: Period, after: Period, now = Date.now()): string {
  for (const period of [before, after]) {
    if (!Number.isFinite(period.from) || !Number.isFinite(period.to)) return 'Укажите начало и конец обоих периодов.'
    const seconds = (period.to - period.from) / 1000
    if (seconds < 60 || seconds > 86400) return 'Длительность каждого периода должна быть от минуты до суток.'
    if (period.from < now - 86400000 || period.to > now) return 'Выберите время в пределах последних 24 часов.'
  }
  if (before.to - before.from !== after.to - after.from) return 'Для сравнения выберите периоды одинаковой длительности.'
  if (before.to > after.from) return 'Период «После» должен начинаться после окончания периода «До».'
  return ''
}

export function summarize(points: Point[], metric: Metric, period: Period): MetricSummary {
  let weighted = 0, samples = 0, min: number | null = null, max: number | null = null
  for (const point of points) {
    const value = point[metric]
    const weight = point.count ?? 1
    if (value != null && Number.isFinite(value) && weight > 0) { weighted += value * weight; samples += weight }
    const low = point[`${metric}_min`] ?? value, high = point[`${metric}_max`] ?? value
    if (low != null && Number.isFinite(low)) min = min == null ? low : Math.min(min, low)
    if (high != null && Number.isFinite(high)) max = max == null ? high : Math.max(max, high)
  }
  return { mean: samples ? weighted / samples : null, min, max, samples, coverage: Math.min(1, samples / Math.max(1, (period.to - period.from) / 1000)) }
}

export function change(before: number | null, after: number | null): { absolute: number | null; relative: number | null } {
  if (before == null || after == null) return { absolute: null, relative: null }
  return { absolute: after - before, relative: before === 0 ? after === 0 ? 0 : null : (after - before) / before * 100 }
}

// datetime-local values represent the browser's local time, not UTC.
export function localInput(time: number): string {
  const date = new Date(time)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}
