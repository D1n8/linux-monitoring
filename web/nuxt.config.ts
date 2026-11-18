export default defineNuxtConfig({
  compatibilityDate: '2025-11-01',
  ssr: false,
  debug: false,
  telemetry: false,
  devtools: { enabled: false },
  css: ['~/assets/main.css'],
  app: { head: {
    title: 'GoInMetrics · Мониторинг сервера',
    htmlAttrs: { lang: 'ru' },
    meta: [{ name: 'description', content: 'Состояние вашего Linux-сервера. Простые метрики, история и события в одном месте.' }],
    link: [{ rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }]
  } },
  vite: { server: { proxy: { '/api': { target: 'http://127.0.0.1:9099', changeOrigin: false } } } },
  typescript: { strict: true }
})
