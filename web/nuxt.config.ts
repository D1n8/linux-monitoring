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
    meta: [{ name: 'description', content: 'Состояние вашего Linux-сервера. Простые метрики и история в одном месте.' }],
    link: [{ rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }, { rel: 'preload', href: '/fonts/InterVariable.woff2', as: 'font', type: 'font/woff2', crossorigin: 'anonymous' }],
    script: [{ innerHTML: `(function(){try{var t=localStorage.getItem('goinmetrics-theme');document.documentElement.dataset.theme=t==='light'?'light':'dark';if(localStorage.getItem('goinmetrics-sidebar')==='hidden')document.documentElement.dataset.sidebar='hidden'}catch(e){}})()` }]
  } },
  vite: { server: { proxy: { '/api': { target: 'http://127.0.0.1:9099', changeOrigin: false } } } },
  typescript: { strict: true }
})
