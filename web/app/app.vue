<script setup lang="ts">
import { bytes, clockTime, metricNames, valueText, type Metric, type Rule, type MetricEvent } from '~/composables/useMetrics'
const { sample, points, rules, events, host, health, connected, paused, range, loading, error, historyError, stale, start, end, historicalEnd, loadHistory, loadMeta, saveRule, goLive, showEvent } = useMetrics()
const modalElement = ref<HTMLElement | null>(null)
let returnFocus: HTMLElement | null = null
const page = ref('overview'), metric = ref<Metric>('cpu'), mobileOpen = ref(false), editing = ref<Rule | null>(null), saving = ref(false), formError = ref(''), notice = ref('')
const nav = [
  { id: 'overview', name: 'Обзор', icon: 'grid' }, { id: 'cpu', name: 'Процессор', icon: 'cpu' },
  { id: 'memory', name: 'Память', icon: 'memory' }, { id: 'disk', name: 'Диски', icon: 'disk' }, { id: 'network', name: 'Сеть', icon: 'network' }
]
const cards: { metric: Metric; title: string; icon: string; color: string }[] = [
  { metric: 'cpu', title: 'Процессор', icon: 'cpu', color: '#c5f277' },
  { metric: 'memory', title: 'Память', icon: 'memory', color: '#a29cf6' },
  { metric: 'disk', title: 'Диски', icon: 'disk', color: '#efb67c' },
  { metric: 'rx', title: 'Сеть · приём', icon: 'network', color: '#7cbfda' }
]
const titles: Record<string, string> = { overview: 'Обзор сервера', cpu: 'Процессор', memory: 'Оперативная память', disk: 'Файловые системы', network: 'Сетевая активность', events: 'События', rules: 'Пороговые правила', about: 'Состояние GoInMetrics' }
const activeEvents = computed(() => events.value.filter(e => e.state === 'active'))
const isResources = computed(() => ['overview', 'cpu', 'memory', 'disk', 'network'].includes(page.value))
const connectionText = computed(() => paused.value ? 'Просмотр на паузе' : !connected.value || stale.value ? 'Нет свежих данных' : 'В реальном времени')
const currentColor = computed(() => cards.find(c => c.metric === metric.value)?.color ?? '#7cbfda')
const currentValues = computed(() => points.value.map(p => p[metric.value]).filter((v): v is number => v != null))
const average = computed(() => currentValues.value.length ? currentValues.value.reduce((a, b) => a + b, 0) / currentValues.value.length : null)
const peak = computed(() => {
  const values = points.value.map(p => p[`${metric.value}_max`] ?? p[metric.value]).filter((v): v is number => v != null)
  return values.length ? Math.max(...values) : null
})
const uptime = computed(() => host.value ? `${Math.floor(host.value.uptime / 86400)} д ${Math.floor(host.value.uptime % 86400 / 3600)} ч` : '—')
const explanations: Record<Metric, string> = {
  cpu: 'Доля занятого времени всех ядер. Короткие пики нормальны; длительная высокая нагрузка — повод проверить приложение.',
  memory: 'Занятая память без доступного кеша Linux. Доступный кеш система может освободить для приложений.',
  disk: 'Наибольшее заполнение среди доступных файловых систем. Учитывается место, доступное обычному пользователю.',
  rx: 'Скорость входящего трафика. Loopback, Docker-мосты и veth исключены, чтобы уменьшить двойной подсчёт.',
  tx: 'Скорость исходящего трафика, рассчитанная по разнице двух последовательных счётчиков Linux.'
}
function navigate(id: string) {
  page.value = id; mobileOpen.value = false
  if (id === 'cpu' || id === 'memory' || id === 'disk') metric.value = id
  if (id === 'network') metric.value = 'rx'
}
function openEditor(rule?: Rule) { formError.value = ''; editing.value = rule ? { ...rule } : { metric: 'cpu', threshold: 85, duration: 30, enabled: true } }
async function submitRule() {
  if (!editing.value || saving.value) return; saving.value = true; formError.value = ''
  try { await saveRule(editing.value); editing.value = null; notice.value = 'Правило сохранено. Отсчёт превышения начнётся заново.' }
  catch (e: unknown) { const err = e as { data?: { error?: string } }; formError.value = err.data?.error ?? 'Не удалось сохранить. Проверьте подключение к БД.' }
  finally { saving.value = false }
}
function eventHistory(event: MetricEvent) { metric.value = event.metric; page.value = 'overview'; showEvent(event) }
function exportCSV() {
  const columns: Metric[] = ['cpu','memory','disk','rx','tx']
  const csv = ['time,cpu_percent,memory_percent,disk_percent,rx_bytes_per_second,tx_bytes_per_second', ...points.value.map(p => [p.time, ...columns.map(c => p[c] ?? '')].join(','))].join('\n')
  const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' })); const a = document.createElement('a'); a.href = url; a.download = `goinmetrics-${new Date().toISOString().slice(0,10)}.csv`; a.click(); URL.revokeObjectURL(url)
}
watch(editing, async (value) => { if (value) { returnFocus = document.activeElement as HTMLElement; await nextTick(); modalElement.value?.querySelector<HTMLSelectElement>('select')?.focus() } else returnFocus?.focus() })
function trapFocus(event: KeyboardEvent) {
  const items = modalElement.value?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled)')
  if (!items?.length) return
  const first = items[0]!, last = items[items.length - 1]!
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
  if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
}
const stateText = (state: string) => ({ active: 'Активно', resolved: 'Завершено', interrupted: 'Прервано' }[state] ?? state)
</script>

<template>
  <div class="app-shell">
    <button v-if="mobileOpen" class="mobile-shade" aria-label="Закрыть меню" @click="mobileOpen = false" />
    <aside :inert="editing !== null" class="sidebar" :class="{ open: mobileOpen }">
      <a class="brand" href="#" @click.prevent="navigate('overview')"><span class="brand-mark"><svg width="27" height="27" viewBox="0 0 30 30" fill="none"><path d="M3 17h6l4-11 5 19 4-12h5" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" /></svg></span><span>GoIn<span class="brand-light">Metrics</span><small>YOUR SERVER, AT A GLANCE</small></span></a>
      <div class="workspace-label">РАБОЧЕЕ ПРОСТРАНСТВО</div>
      <div class="server-switch"><span class="server-icon"><AppIcon name="server" :size="19" /></span><div><strong>{{ host?.name || 'Linux-сервер' }}</strong><small><i class="dot" /> Локальный сервер</small></div><span class="server-label">01</span></div>
      <div class="nav-label">МОНИТОРИНГ</div>
      <nav aria-label="Ресурсы сервера"><button v-for="item in nav" :key="item.id" :class="['nav-item', { active: page === item.id }]" @click="navigate(item.id)"><AppIcon :name="item.icon" :size="19" />{{ item.name }}<span v-if="page === item.id" class="nav-active-dot" /></button></nav>
      <div class="nav-label second-label">УПРАВЛЕНИЕ</div>
      <nav aria-label="Управление мониторингом"><button :class="['nav-item', { active: page === 'events' }]" @click="navigate('events')"><AppIcon name="bell" :size="19" />События<span v-if="activeEvents.length" class="count-badge">{{ activeEvents.length }}</span></button><button :class="['nav-item', { active: page === 'rules' }]" @click="navigate('rules')"><AppIcon name="sliders" :size="19" />Пороговые правила</button><button :class="['nav-item', { active: page === 'about' }]" @click="navigate('about')"><AppIcon name="info" :size="19" />О сервисе</button></nav>
      <div class="sidebar-bottom"><div class="private-box"><AppIcon name="terminal" :size="20" /><strong>Данные остаются у вас</strong><p>Локальный сбор. Доступ через SSH.<br>Никакого облака.</p><button @click="navigate('about')">Как это работает <AppIcon name="arrow" :size="14" /></button></div><div class="version"><span><i class="dot" /> GoInMetrics</span><span>v0.1.0</span></div></div>
    </aside>

    <div :inert="editing !== null" class="main-shell">
      <header class="topbar"><div class="breadcrumbs"><button class="mobile-menu icon-button" aria-label="Открыть меню" @click="mobileOpen = !mobileOpen"><AppIcon name="grid" /></button><AppIcon name="server" :size="17" /><span>Мой сервер</span><AppIcon name="chevron" :size="13" /><strong>{{ titles[page] }}</strong></div><div class="topbar-right"><span class="local-badge"><i class="dot" /> Локальный экземпляр</span><span class="avatar">{{ (host?.name || 'GM').slice(0, 2).toUpperCase() }}</span></div></header>
      <main>
        <div class="page-heading"><div><div class="eyebrow">ВАШ СЕРВЕР. ВСЁ ПОД КОНТРОЛЕМ.</div><h1>{{ titles[page] }}<span class="heading-dot">.</span></h1><p>{{ isResources ? 'Всё, что нужно знать о состоянии сервера, на одной странице.' : page === 'events' ? 'Превышения порогов помогают выбрать момент для проверки.' : page === 'rules' ? 'Укажите, на какие изменения стоит обратить внимание.' : 'Прозрачный мониторинг начинается с самого наблюдателя.' }}</p></div><button v-if="isResources" class="button secondary export-button" :disabled="!points.length" @click="exportCSV"><AppIcon name="download" :size="17" />Экспорт CSV</button><button v-if="page === 'rules'" class="button primary" @click="openEditor()">+ Добавить правило</button></div>

        <div v-if="!connected || (!paused && stale)" class="alert warning" role="status"><AppIcon name="refresh" :size="18" /><div><strong>Ожидаем свежие данные</strong><span>Соединение восстановится автоматически. Последние значения могут быть неактуальны.</span></div></div>
        <div v-if="health && !health.storage_ok" class="alert danger" role="alert"><AppIcon name="disk" :size="18" /><div><strong>История не сохраняется</strong><span>{{ health.storage_error }}</span></div></div>
        <div v-if="sample && Object.keys(sample.errors).length" class="alert warning" role="status"><AppIcon name="info" :size="18" /><div><strong>Часть источников недоступна</strong><span v-for="(message, source) in sample.errors" :key="source">{{ source }}: {{ message }}</span></div></div>
        <div v-if="error" class="inline-error" role="status">{{ error }}</div>
        <div v-if="notice" class="alert success" role="status"><AppIcon name="check" :size="18" />{{ notice }}<button class="icon-button" aria-label="Закрыть уведомление" @click="notice = ''"><AppIcon name="close" :size="16" /></button></div>

        <template v-if="isResources">
          <section class="host-strip"><div class="host-main"><span class="host-glyph"><AppIcon name="server" :size="24" /></span><div><strong>{{ host?.name || 'Подключение к серверу…' }}</strong><span>{{ host?.os || 'Linux' }} <span class="strip-separator">/</span> {{ host?.cores || '—' }} CPU <span class="strip-separator">/</span> {{ bytes(sample?.memory_total) }} RAM</span></div></div><div class="host-meta"><span class="muted">Время работы<strong>{{ uptime }}</strong></span><span class="host-status" :class="{ muted: stale || paused }"><i class="dot" />{{ stale || paused ? 'Нет оценки' : activeEvents.length ? 'Есть события' : Object.keys(sample?.errors || {}).length ? 'Частичные данные' : 'Сбор работает' }}</span></div></section>
          <div class="section-toolbar"><h2>Ключевые показатели <span>СЕЙЧАС</span></h2><span class="live-indicator" :class="{ offline: !connected || stale || paused }"><i class="dot" />{{ connectionText }}<span class="live-divider" />1 сек</span></div>
          <section class="metrics-grid" aria-label="Текущие показатели"><button v-for="card in cards" :key="card.metric" :class="['metric-card', { selected: metric === card.metric }]" :style="{ '--accent': card.color }" @click="metric = card.metric"><div class="metric-card-top"><span>{{ card.title }}</span><AppIcon :name="card.icon" :size="20" /></div><div class="metric-value">{{ valueText(sample?.[card.metric], card.metric) }}<AppIcon name="arrow" :size="18" /></div><div class="metric-detail">{{ sample?.[card.metric] == null ? sample?.errors[card.metric === 'rx' ? 'network' : card.metric] ? 'Источник недоступен' : 'Подготовка данных…' : card.metric === 'cpu' ? `${host?.cores || '—'} ядер · общая загрузка` : card.metric === 'memory' ? `${bytes(sample?.memory_used)} из ${bytes(sample?.memory_total)}` : card.metric === 'disk' ? `${sample?.disks.length || 0} файловых систем · макс. заполнение` : `Отправка: ${bytes(sample?.tx, true)}` }}</div><MetricChart :points="points" :metric="card.metric" :from="start" :to="end" :color="card.color" small /></button></section>

          <section class="panel main-chart-panel"><div class="panel-heading chart-heading"><div><h2>Динамика нагрузки</h2><p>{{ historicalEnd ? 'Выбранный участок истории' : 'История показателей вашего сервера' }}</p></div><div class="chart-controls"><div class="segmented" aria-label="Период"><button v-for="period in [{ label: '15 мин', value: 900 }, { label: '1 час', value: 3600 }, { label: '6 часов', value: 21600 }, { label: '24 часа', value: 86400 }]" :key="period.value" :class="{ active: range === period.value }" @click="range = period.value">{{ period.label }}</button></div><button class="icon-button pause-button" :aria-label="paused ? 'Возобновить просмотр' : 'Приостановить просмотр'" :title="paused ? 'Возобновить просмотр' : 'Приостановить просмотр'" @click="paused = !paused"><AppIcon :name="paused ? 'play' : 'pause'" :size="17" /></button></div></div>
            <div class="chart-tabs"><button v-for="m in (['cpu', 'memory', 'disk', 'rx', 'tx'] as Metric[])" :key="m" :class="{ active: metric === m }" @click="metric = m"><span class="legend-dot" :style="{ background: metric === m ? currentColor : '#636c79' }" />{{ metricNames[m] }}</button></div>
            <div v-if="historyError" class="history-error" role="status">{{ historyError }} <button @click="loadHistory">Повторить</button></div>
            <div v-if="historicalEnd || paused" class="chart-notice">{{ paused ? 'Просмотр приостановлен. Сбор и запись продолжаются.' : 'Просмотр события в истории.' }}<button @click="goLive">К текущим данным →</button></div>
            <MetricChart :points="points" :metric="metric" :from="start" :to="end" :color="currentColor" />
            <div class="chart-footer"><div class="chart-stat"><span>Сейчас</span><strong :style="{ color: currentColor }">{{ valueText(sample?.[metric], metric) }}</strong></div><div class="chart-stat"><span>Среднее по точкам</span><strong>{{ valueText(average, metric) }}</strong></div><div class="chart-stat"><span>Максимум</span><strong>{{ valueText(peak, metric) }}</strong></div><span class="chart-range"><AppIcon name="clock" :size="14" />{{ loading ? 'Загрузка истории…' : 'Время вашего браузера' }}</span></div>
          </section>
          <div class="chart-explanation"><AppIcon name="info" :size="16" /><span>{{ explanations[metric] }} Полоса на графике показывает минимум и максимум.</span></div>

          <div class="detail-grid"><section v-if="page !== 'network'" class="panel disk-panel"><div class="panel-heading"><h2><AppIcon name="disk" :size="18" />Файловые системы</h2><span class="muted small-text">{{ sample?.disks.length || 0 }} подключено</span></div><div v-if="!sample?.disks.length" class="empty compact">Нет доступных файловых систем</div><div v-for="disk in sample?.disks" :key="disk.mount" class="disk-row"><div class="disk-title"><span><strong>{{ disk.mount }}</strong><small>{{ disk.name }}</small></span><b>{{ disk.percent.toFixed(1) }}<span>%</span></b></div><div class="progress"><span :style="{ width: `${disk.percent}%`, background: disk.percent > 90 ? '#ef9a7c' : '#c5f277' }" /></div><div class="disk-foot"><span>{{ bytes(disk.used) }} занято</span><span>из {{ bytes(disk.total) }}</span></div></div></section>
            <section v-else class="panel"><div class="panel-heading"><h2><AppIcon name="network" :size="18" />Сетевые интерфейсы</h2></div><div v-if="!sample?.networks.length" class="empty compact">Нет доступных интерфейсов</div><div v-for="network in sample?.networks" :key="network.name" class="network-row"><strong>{{ network.name }}</strong><span>↓ {{ bytes(network.rx, true) }}</span><span>↑ {{ bytes(network.tx, true) }}</span></div></section>
            <section class="panel events-panel"><div class="panel-heading"><h2><AppIcon name="bell" :size="18" />Последние события</h2><button class="text-button" @click="navigate('events')">Все события <AppIcon name="chevron" :size="13" /></button></div><div v-if="!events.length" class="empty event-empty"><span class="empty-icon"><AppIcon name="check" :size="22" /></span><strong>Пока без событий</strong><p>{{ rules.some(r => r.enabled) ? 'Превышений включённых порогов не зафиксировано.' : 'Включите пороговое правило, чтобы отмечать изменения нагрузки.' }}</p><button class="text-button" @click="navigate('rules')">Настроить правила <AppIcon name="arrow" :size="14" /></button></div><button v-for="event in events.slice(0, 3)" :key="event.id" class="event-preview" @click="eventHistory(event)"><span class="event-dot" :class="event.state" /><span><strong>{{ metricNames[event.metric] }} &gt; {{ valueText(event.threshold, event.metric) }}</strong><small>{{ clockTime(event.started) }} · {{ stateText(event.state) }}</small></span><AppIcon name="chevron" :size="15" /></button></section></div>
        </template>

        <section v-else-if="page === 'events'" class="panel"><div class="panel-heading"><h2>Журнал событий <span class="pill">{{ events.length }}</span></h2><button class="text-button" @click="loadMeta"><AppIcon name="refresh" :size="16" />Обновить</button></div><div v-if="!events.length" class="empty"><AppIcon name="bell" :size="32" /><h3>Событий пока нет</h3><p>Включите правила. Событие появится, когда показатель будет выше порога заданное время.</p><button class="button secondary" @click="navigate('rules')">Перейти к правилам</button></div><div v-else class="table-scroll"><table><thead><tr><th>Показатель</th><th>Порог</th><th>Начало</th><th>Завершение</th><th>Статус</th><th><span class="sr-only">История</span></th></tr></thead><tbody><tr v-for="event in events" :key="event.id"><td>{{ metricNames[event.metric] }}</td><td>{{ valueText(event.threshold, event.metric) }}</td><td>{{ new Date(event.started).toLocaleString('ru-RU') }}</td><td>{{ event.ended ? clockTime(event.ended) : '—' }}</td><td><span class="state-badge" :class="event.state">{{ stateText(event.state) }}</span></td><td><button class="text-button" @click="eventHistory(event)">На графике <AppIcon name="arrow" :size="14" /></button></td></tr></tbody></table></div><div class="panel-note">Последние 100 событий за сутки. Событие — подсказка для проверки, а не доказательство неполадки.</div></section>

        <template v-else-if="page === 'rules'"><div class="info-banner"><AppIcon name="info" :size="20" /><p>Правила общие для всех пользователей этого экземпляра. По умолчанию они выключены. Изменение правила или перезапуск сбрасывает отсчёт непрерывного превышения.</p></div><section class="panel"><div class="table-scroll"><table><thead><tr><th>Показатель</th><th>Порог</th><th>Длительность</th><th>Состояние</th><th /></tr></thead><tbody><tr v-for="rule in rules" :key="rule.metric"><td><span class="table-metric"><AppIcon :name="rule.metric === 'rx' || rule.metric === 'tx' ? 'network' : rule.metric" :size="18" />{{ metricNames[rule.metric] }}</span></td><td>&gt; {{ valueText(rule.threshold, rule.metric) }}</td><td>{{ rule.duration }} сек</td><td><span class="state-badge" :class="rule.enabled ? 'resolved' : 'interrupted'">{{ rule.enabled ? 'Включено' : 'Выключено' }}</span></td><td><button class="button small-button secondary" @click="openEditor(rule)">Изменить</button></td></tr></tbody></table></div></section></template>

        <template v-else-if="page === 'about'"><div class="diagnostics-grid"><section class="panel"><div class="panel-heading"><h2>Состояние наблюдателя</h2><AppIcon name="server" /></div><dl class="diagnostics"><div><dt>Поток измерений</dt><dd :class="{ 'green-text': connected && !stale }">{{ connected && !stale ? 'Подключён' : 'Нет свежих данных' }}</dd></div><div><dt>PostgreSQL</dt><dd>{{ health ? health.storage_ok ? 'Запись работает' : 'Ошибка записи' : 'Проверяем…' }}</dd></div><div><dt>Последняя запись</dt><dd>{{ health?.last_saved ? clockTime(health.last_saved) : 'Ожидается' }}</dd></div><div><dt>Время сбора</dt><dd>{{ sample?.collection_ms.toFixed(2) ?? '—' }} мс</dd></div><div><dt>Очередь записи</dt><dd>{{ health?.queue ?? '—' }} / 60</dd></div><div><dt>Несохранённые измерения</dt><dd>{{ health?.dropped ?? '—' }}</dd></div><div><dt>Пропущенные циклы</dt><dd>{{ health?.missed ?? '—' }}</dd></div><div><dt>История</dt><dd>24 часа · очистка раз в минуту</dd></div><div><dt>Ядро Linux</dt><dd>{{ host?.kernel || '—' }}</dd></div></dl></section><section class="panel access-panel"><div class="panel-heading"><h2>Простой и локальный</h2><AppIcon name="terminal" /></div><p>Go читает счётчики Linux раз в секунду. Один сборщик обслуживает все открытые страницы. Nuxt-интерфейс встроен в приложение.</p><h3>Подключение с вашего компьютера</h3><pre><code>ssh -N -L 127.0.0.1:19099:127.0.0.1:9099 user@server</code></pre><p>Откройте <code>http://127.0.0.1:19099</code>. Доступ разрешает SSH сервера. Все подключённые пользователи могут менять правила.</p><div class="access-note"><AppIcon name="info" :size="18" /><span>Нагрузка самого наблюдателя зависит от VPS и числа ресурсов. Время сбора не включает расходы PostgreSQL. Полный отказ сервера требует внешнего наблюдения.</span></div></section></div></template>
        <footer class="page-footer"><span><i class="dot" />Данные на вашем сервере. Только ваши.</span><span>{{ sample ? `Последнее измерение: ${clockTime(sample.time)}` : 'Подготовка счётчиков…' }}<span class="footer-divider">/</span> GoInMetrics v0.1</span></footer>
      </main>
    </div>

    <div v-if="editing" class="modal-backdrop" @click.self="!saving && (editing = null)" @keydown.esc="!saving && (editing = null)"><section ref="modalElement" class="modal" @keydown.tab="trapFocus" role="dialog" aria-modal="true" aria-labelledby="rule-title"><div class="panel-heading"><h2 id="rule-title">Пороговое правило</h2><button class="icon-button" aria-label="Закрыть" :disabled="saving" @click="editing = null"><AppIcon name="close" /></button></div><form @submit.prevent="submitRule"><label>Показатель<select v-model="editing.metric"><option v-for="(name, key) in metricNames" :key="key" :value="key">{{ name }}</option></select></label><label>Порог {{ editing.metric === 'rx' || editing.metric === 'tx' ? '(байт/с)' : '(%)' }}<input v-model.number="editing.threshold" type="number" required min="0" :max="editing.metric === 'rx' || editing.metric === 'tx' ? 1e15 : 100" step="0.1" /></label><label>Превышение дольше (секунд)<input v-model.number="editing.duration" type="number" required min="1" max="3600" step="1" /></label><label class="checkbox-label"><input v-model="editing.enabled" type="checkbox" />Включить правило</label><p class="muted small-text">Для каждого показателя действует одно правило. Сохранение заменит существующее.</p><p v-if="formError" class="form-error" role="alert">{{ formError }}</p><div class="modal-actions"><button type="button" class="button secondary" :disabled="saving" @click="editing = null">Отмена</button><button class="button primary" :disabled="saving">{{ saving ? 'Сохраняем…' : 'Сохранить правило' }}</button></div></form></section></div>
  </div>
</template>
