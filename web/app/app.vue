<script setup lang="ts">
import { bytes, clockTime, metricNames, valueText, type Metric } from '~/composables/useMetrics'
const page = ref('overview'), metric = ref<Metric>('cpu'), sidebarHidden = ref(false), mobileOpen = ref(false)
const isResources = computed(() => ['overview', 'cpu', 'memory', 'disk', 'network'].includes(page.value))
const { sample, points, host, health, connected, paused, range, loading, error, historyError, stale, monitoringEnabled, start, end, loadHistory, loadMeta, setMonitoring, goLive } = useMetrics(isResources)
const isMobile = ref(false)
const sidebarTab = ref<HTMLButtonElement | null>(null)
const sidebarOpen = computed(() => isMobile.value ? mobileOpen.value : !sidebarHidden.value)
let viewport: MediaQueryList | undefined
function updateViewport() { isMobile.value = viewport?.matches ?? false; if (!isMobile.value) mobileOpen.value = false }
const theme = ref<'dark' | 'light'>('dark'), serviceBusy = ref(false), serviceError = ref('')
const nav = [
  { id: 'overview', name: 'Обзор', icon: 'grid' }, { id: 'cpu', name: 'Процессор', icon: 'cpu' },
  { id: 'memory', name: 'Память', icon: 'memory' }, { id: 'disk', name: 'Диски', icon: 'disk' }, { id: 'network', name: 'Сеть', icon: 'network' }, { id: 'compare', name: 'Сравнение периодов', icon: 'compare' }
]
const cards: { metric: Metric; title: string; icon: string; color: string }[] = [
  { metric: 'cpu', title: 'Процессор', icon: 'cpu', color: 'var(--chart-cpu)' },
  { metric: 'memory', title: 'Память', icon: 'memory', color: 'var(--chart-memory)' },
  { metric: 'disk', title: 'Диски', icon: 'disk', color: 'var(--chart-disk)' },
  { metric: 'rx', title: 'Входящий трафик', icon: 'network', color: 'var(--chart-network)' }
]
const titles: Record<string, string> = { overview: 'Обзор сервера', cpu: 'Процессор', memory: 'Оперативная память', disk: 'Файловые системы', network: 'Сетевая активность', compare: 'Сравнение периодов', docs: 'Документация', about: 'Состояние сервиса' }
const connectionText = computed(() => !monitoringEnabled.value ? 'Мониторинг выключен' : paused.value ? 'Просмотр на паузе' : !connected.value || stale.value ? 'Нет свежих данных' : 'В реальном времени')
const currentColor = computed(() => cards.find(c => c.metric === metric.value)?.color ?? 'var(--chart-network)')
const currentValues = computed(() => points.value.map(p => p[metric.value]).filter((v): v is number => v != null))
const average = computed(() => currentValues.value.length ? currentValues.value.reduce((a, b) => a + b, 0) / currentValues.value.length : null)
const peak = computed(() => {
  const values = points.value.map(p => p[`${metric.value}_max`] ?? p[metric.value]).filter((v): v is number => v != null)
  return values.length ? Math.max(...values) : null
})
const uptime = computed(() => host.value ? `${Math.floor(host.value.uptime / 86400)} д ${Math.floor(host.value.uptime % 86400 / 3600)} ч` : '—')
function navigate(id: string) {
  page.value = id; mobileOpen.value = false
  if (id === 'cpu' || id === 'memory' || id === 'disk') metric.value = id
  if (id === 'network') metric.value = 'rx'
}
function persist(key: string, value: string) { try { localStorage.setItem(key, value) } catch { /* Private browsers can deny local storage. */ } }
function toggleTheme() { theme.value = theme.value === 'dark' ? 'light' : 'dark'; document.documentElement.dataset.theme = theme.value; persist('goinmetrics-theme', theme.value) }
function hideSidebar() { sidebarHidden.value = true; mobileOpen.value = false; document.documentElement.dataset.sidebar = 'hidden'; persist('goinmetrics-sidebar', 'hidden'); void nextTick(() => sidebarTab.value?.focus()) }
function openSidebar() {
  if (window.matchMedia('(max-width: 720px)').matches) mobileOpen.value = true
  else { sidebarHidden.value = false; delete document.documentElement.dataset.sidebar; persist('goinmetrics-sidebar', 'visible') }
}
function toggleSidebar() { if (sidebarOpen.value) hideSidebar(); else openSidebar() }
async function toggleService() {
  if (serviceBusy.value) return; serviceBusy.value = true; serviceError.value = ''
  try { await setMonitoring(!monitoringEnabled.value) }
  catch (e: unknown) { const err = e as { data?: { error?: string } }; serviceError.value = err.data?.error ?? 'Не удалось изменить состояние. Повторите после восстановления связи.' }
  finally { serviceBusy.value = false }
}
function exportCSV() {
  const columns: Metric[] = ['cpu', 'memory', 'disk', 'rx', 'tx']
  const csv = ['time,cpu_percent,memory_percent,disk_percent,rx_bytes_per_second,tx_bytes_per_second', ...points.value.map(p => [p.time, ...columns.map(c => p[c] ?? '')].join(','))].join('\n')
  const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' })); const a = document.createElement('a'); a.href = url; a.download = `goinmetrics-${new Date().toISOString().slice(0, 10)}.csv`; a.click(); URL.revokeObjectURL(url)
}
onMounted(() => { theme.value = document.documentElement.dataset.theme === 'light' ? 'light' : 'dark'; sidebarHidden.value = document.documentElement.dataset.sidebar === 'hidden'; viewport = window.matchMedia('(max-width: 720px)'); updateViewport(); viewport.addEventListener('change', updateViewport) })
onBeforeUnmount(() => viewport?.removeEventListener('change', updateViewport))
</script>

<template>
  <div class="app-shell" :class="{ 'sidebar-hidden': sidebarHidden }" @keydown.esc="mobileOpen = false">
    <button v-if="mobileOpen" class="mobile-shade" aria-label="Закрыть меню" @click="mobileOpen = false" />
    <div class="sidebar-shell" :class="{ open: mobileOpen }">
    <aside :inert="(isMobile || sidebarHidden) && !mobileOpen" id="navigation" class="sidebar">
      <a class="brand" href="#" @click.prevent="navigate('overview')"><AppIcon name="server" :size="25" /><span>GoInMetrics</span></a>
      <div class="server-switch"><AppIcon name="server" :size="19" /><div><strong>{{ host?.name || 'Linux-сервер' }}</strong><small><i class="dot" :class="{ stopped: !monitoringEnabled }" /> {{ monitoringEnabled ? 'Мониторинг включён' : 'Мониторинг выключен' }}</small></div></div>
      <nav aria-label="Ресурсы сервера"><button v-for="item in nav" :key="item.id" :class="['nav-item', { active: page === item.id }]" :aria-current="page === item.id ? 'page' : undefined" @click="navigate(item.id)"><AppIcon :name="item.icon" :size="19" />{{ item.name }}</button></nav>
      <nav aria-label="Сервис"><button :class="['nav-item', { active: page === 'docs' }]" @click="navigate('docs')"><AppIcon name="book" :size="19" />Документация</button><button :class="['nav-item', { active: page === 'about' }]" @click="navigate('about')"><AppIcon name="sliders" :size="19" />Состояние сервиса</button></nav>
      <div class="sidebar-bottom"><button class="theme-switch" :aria-label="theme === 'dark' ? 'Включить светлую тему' : 'Включить тёмную тему'" @click="toggleTheme"><AppIcon :name="theme === 'dark' ? 'sun' : 'moon'" :size="18" /><span>{{ theme === 'dark' ? 'Светлая тема' : 'Тёмная тема' }}</span></button><button class="service-switch" :disabled="serviceBusy || !health" @click="toggleService"><AppIcon name="power" :size="18" /><span>{{ serviceBusy ? 'Меняем состояние…' : monitoringEnabled ? 'Остановить мониторинг' : 'Включить мониторинг' }}</span></button></div>
    </aside>
    <button ref="sidebarTab" class="sidebar-tab" :aria-label="sidebarOpen ? 'Скрыть боковую панель' : 'Открыть боковую панель'" :title="sidebarOpen ? 'Скрыть боковую панель' : 'Открыть боковую панель'" aria-controls="navigation" :aria-expanded="sidebarOpen" @click="toggleSidebar"><AppIcon :name="sidebarOpen ? 'chevron-left' : 'chevron'" :size="18" /></button>
    </div>
    <div class="main-shell" :inert="isMobile && mobileOpen">
      <main>
        <div class="page-heading"><h1>{{ titles[page] }}</h1><div class="heading-actions"><button class="icon-button theme-shortcut" :aria-label="theme === 'dark' ? 'Включить светлую тему' : 'Включить тёмную тему'" @click="toggleTheme"><AppIcon :name="theme === 'dark' ? 'sun' : 'moon'" :size="21" /></button><button v-if="isResources" class="button secondary export-button" :disabled="!points.length" @click="exportCSV"><AppIcon name="download" :size="17" />Экспорт CSV</button></div></div>
        <div v-if="serviceError" class="alert danger" role="alert"><AppIcon name="info" :size="18" /><span>{{ serviceError }}</span><button class="icon-button" aria-label="Закрыть ошибку" @click="serviceError = ''"><AppIcon name="close" :size="16" /></button></div>
        <div v-if="!monitoringEnabled" class="alert service-stopped" role="status"><AppIcon name="power" :size="20" /><div><strong>Мониторинг выключен</strong><span>Сбор и запись остановлены. Сохранённая история доступна; состояние сохранится после перезапуска.</span></div><button class="button secondary" :disabled="serviceBusy" @click="toggleService">Включить</button></div>
        <div v-else-if="!connected || (!paused && stale)" class="alert warning" role="status"><AppIcon name="refresh" :size="18" /><div><strong>Ожидаем свежие данные</strong><span>Соединение восстановится автоматически. Последние значения могут быть неактуальны.</span></div></div>
        <div v-if="health && !health.storage_ok" class="alert danger" role="alert"><AppIcon name="disk" :size="18" /><div><strong>История не сохраняется</strong><span>{{ health.storage_error }}</span></div></div>
        <div v-if="monitoringEnabled && sample && Object.keys(sample.errors).length" class="alert warning" role="status"><AppIcon name="info" :size="18" /><div><strong>Часть источников недоступна</strong><span v-for="(message, source) in sample.errors" :key="source">{{ source }}: {{ message }}</span></div></div>
        <div v-if="error" class="inline-error" role="status">{{ error }}</div>

        <template v-if="isResources">
          <section class="host-strip"><div class="host-main"><span class="host-glyph"><AppIcon name="server" :size="24" /></span><div><strong>{{ host?.name || 'Подключение к серверу…' }}</strong><span>{{ host?.os || 'Linux' }} <span class="strip-separator">/</span> {{ host?.cores || '—' }} CPU <span class="strip-separator">/</span> {{ bytes(sample?.memory_total) }} RAM</span></div></div><div class="host-meta"><span class="muted">Время работы<strong>{{ uptime }}</strong></span><span class="host-status" :class="{ inactive: !monitoringEnabled || stale || paused }"><i class="dot" />{{ !monitoringEnabled ? 'Выключен' : stale || paused ? 'Нет оценки' : Object.keys(sample?.errors || {}).length ? 'Частичные данные' : 'Сбор работает' }}</span></div></section>
          <div class="section-toolbar"><h2>{{ monitoringEnabled ? 'Показатели' : 'Последние показатели' }}</h2><span class="live-indicator" :class="{ offline: !monitoringEnabled || !connected || stale || paused }"><i class="dot" />{{ connectionText }}</span></div>
          <section class="metrics-grid" aria-label="Текущие показатели"><button v-for="card in cards" :key="card.metric" :class="['metric-card', { selected: metric === card.metric }]" :style="{ '--accent': card.color }" @click="metric = card.metric"><div class="metric-card-top"><span>{{ card.title }}</span><AppIcon :name="card.icon" :size="20" /></div><div class="metric-value">{{ valueText(sample?.[card.metric], card.metric) }}</div><div class="metric-detail">{{ !monitoringEnabled ? 'Сбор остановлен' : sample?.[card.metric] == null ? sample?.errors[card.metric === 'rx' ? 'network' : card.metric] ? 'Источник недоступен' : 'Подготовка данных…' : card.metric === 'cpu' || card.metric === 'disk' ? '' : card.metric === 'memory' ? `${bytes(sample?.memory_used)} из ${bytes(sample?.memory_total)}` : `↑ ${bytes(sample?.tx, true)}` }}</div><MetricChart :points="points" :metric="card.metric" :from="start" :to="end" :color="card.color" small /></button></section>
          <section class="panel main-chart-panel"><div class="panel-heading chart-heading"><div><h2>История показателей</h2></div><div class="chart-controls"><div class="segmented" aria-label="Период"><button v-for="period in [{ label: '15 мин', value: 900 }, { label: '1 час', value: 3600 }, { label: '6 часов', value: 21600 }, { label: '24 часа', value: 86400 }]" :key="period.value" :class="{ active: range === period.value }" @click="range = period.value">{{ period.label }}</button></div><button class="icon-button pause-button" :disabled="!monitoringEnabled" :aria-label="paused ? 'Возобновить просмотр' : 'Приостановить просмотр'" :title="paused ? 'Возобновить просмотр' : 'Приостановить просмотр'" @click="paused = !paused"><AppIcon :name="paused ? 'play' : 'pause'" :size="17" /></button></div></div>
            <div class="chart-tabs"><button v-for="m in (['cpu', 'memory', 'disk', 'rx', 'tx'] as Metric[])" :key="m" :class="{ active: metric === m }" @click="metric = m">{{ metricNames[m] }}</button></div>
            <div v-if="historyError" class="history-error" role="status">{{ historyError }} <button @click="loadHistory">Повторить</button></div>
            <div v-if="paused" class="chart-notice">Просмотр приостановлен. Сбор и запись продолжаются.<button @click="goLive">К текущим данным →</button></div>
            <MetricChart :points="points" :metric="metric" :from="start" :to="end" :color="currentColor" />
            <div class="chart-footer"><div class="chart-stat"><span>{{ monitoringEnabled ? 'Сейчас' : 'Последнее' }}</span><strong :style="{ color: currentColor }">{{ valueText(sample?.[metric], metric) }}</strong></div><div class="chart-stat"><span>Среднее</span><strong>{{ valueText(average, metric) }}</strong></div><div class="chart-stat"><span>Максимум</span><strong>{{ valueText(peak, metric) }}</strong></div><span v-if="loading" class="chart-range">Загрузка…</span></div>
          </section>
          <div class="detail-grid"><section class="panel disk-panel"><div class="panel-heading"><h2><AppIcon name="disk" :size="18" />Файловые системы</h2><span class="muted small-text">{{ sample?.disks.length || 0 }} подключено</span></div><div v-if="!sample?.disks.length" class="empty compact">Нет доступных файловых систем</div><div v-for="disk in sample?.disks" :key="disk.mount" class="disk-row"><div class="disk-title"><span><strong>{{ disk.mount }}</strong><small>{{ disk.name }}</small></span><b>{{ disk.percent.toFixed(1) }}<span>%</span></b></div><div class="progress"><span :style="{ width: `${disk.percent}%`, background: disk.percent > 90 ? 'var(--warning)' : 'var(--accent)' }" /></div><div class="disk-foot"><span>{{ bytes(disk.used) }} занято</span><span>из {{ bytes(disk.total) }}</span></div></div></section>
            <section class="panel"><div class="panel-heading"><h2><AppIcon name="network" :size="18" />Сетевые интерфейсы</h2><span class="muted small-text">байт/с</span></div><div v-if="!sample?.networks.length" class="empty compact">Нет доступных интерфейсов</div><div v-for="network in sample?.networks" :key="network.name" class="network-row"><strong>{{ network.name }}</strong><span>↓ {{ bytes(network.rx, true) }}</span><span>↑ {{ bytes(network.tx, true) }}</span></div></section></div>
        </template>

        <template v-else-if="page === 'about'"><section class="panel service-control"><div><h2>{{ monitoringEnabled ? 'Мониторинг включён' : 'Мониторинг выключен' }}</h2><p>Остановка прекращает чтение счётчиков, запись и очистку истории. Панель и PostgreSQL остаются запущены для просмотра и повторного включения.</p></div><button class="button" :class="monitoringEnabled ? 'secondary' : 'primary'" :disabled="serviceBusy || !health" @click="toggleService"><AppIcon name="power" :size="17" />{{ serviceBusy ? 'Меняем состояние…' : monitoringEnabled ? 'Остановить сбор' : 'Включить сбор' }}</button></section><div class="diagnostics-grid"><section class="panel"><div class="panel-heading"><h2>Состояние наблюдателя</h2><button class="icon-button" aria-label="Обновить состояние" @click="loadMeta"><AppIcon name="refresh" :size="18" /></button></div><dl class="diagnostics"><div><dt>Мониторинг</dt><dd :class="{ 'accent-text': monitoringEnabled }">{{ monitoringEnabled ? 'Включён' : 'Выключен' }}</dd></div><div><dt>Соединение с панелью</dt><dd>{{ connected ? 'Подключено' : 'Переподключение…' }}</dd></div><div><dt>Хранилище</dt><dd>{{ health ? health.storage_ok ? monitoringEnabled ? 'Запись работает' : 'Сбор остановлен' : 'Ошибка записи' : 'Проверяем…' }}</dd></div><div><dt>Последняя запись</dt><dd>{{ health?.last_saved ? clockTime(health.last_saved) : 'Ожидается' }}</dd></div><div><dt>Время одного сбора</dt><dd>{{ sample?.collection_ms.toFixed(2) ?? '—' }} мс</dd></div><div><dt>Очередь записи</dt><dd>{{ health?.queue ?? '—' }} / 60</dd></div><div><dt>Несохранённые измерения</dt><dd>{{ health?.dropped ?? '—' }}</dd></div><div><dt>Пропущенные циклы</dt><dd>{{ health?.missed ?? '—' }}</dd></div><div><dt>Ядро Linux</dt><dd>{{ host?.kernel || '—' }}</dd></div></dl></section><section class="panel access-panel"><div class="panel-heading"><h2>Полное выключение сервиса</h2><AppIcon name="power" /></div><p>Чтобы полностью остановить приложение и PostgreSQL и освободить их ресурсы, выполните в каталоге проекта:</p><pre><code>./stop.sh</code></pre><p>Данные и настройки сохранятся. Повторный запуск:</p><pre><code>./start.sh</code></pre><div class="access-note"><AppIcon name="info" :size="18" /><span>Выключенное в панели состояние мониторинга сохраняется между запусками. После запуска нажмите «Включить мониторинг».</span></div></section></div></template>

        <PeriodComparison v-else-if="page === 'compare'" />
        <ServiceDocs v-else-if="page === 'docs'" />
        <footer class="page-footer"><span>{{ sample ? `Последнее измерение: ${clockTime(sample.time)}` : 'Нет текущих измерений' }}</span></footer>
      </main>
    </div>
  </div>
</template>
