# HTTP API · v0.2

База: `http://127.0.0.1:9099/api`. JSON, даты RFC3339 UTC. Ошибка: `{"error":"сообщение"}`. Все допущенные пользователи равноправны.

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/health` | monitoring_enabled, storage_ok, last_saved, queue, dropped, missed, collected, viewers, started; 503 при последней ошибке хранения |
| GET | `/host` | Имя, Linux, ядро, число CPU, uptime в секундах. Статические сведения кешируются |
| GET | `/snapshot` | Последний снимок без отдельного чтения Linux |
| GET | `/stream` | SSE: `metrics` — снимок, `service` — состояние мониторинга/хранения; heartbeat каждые 15 секунд |
| GET | `/history?from=…&to=…` | points, step, from, to; по умолчанию последние 15 минут |
| PUT | `/service` | Включить или остановить сбор: `{"enabled":true}` / `{"enabled":false}` |

Управление требует `Content-Type: application/json`, явного boolean enabled, одного JSON-объекта без лишних полей; максимум 1 KiB. Остановка подтверждается после завершения очереди и сохранения настройки. При ошибке сохранения возвращается 503, прежнее состояние остаётся в силе. Повторная установка текущего состояния идемпотентна. Состояние передаётся всем SSE-зрителям.

```bash
curl -X PUT http://127.0.0.1:9099/api/service \
  -H 'Content-Type: application/json' -d '{"enabled":false}'
```

HTTP и PostgreSQL при таком запросе остаются запущены. Полное выключение обоих контейнеров: `./stop.sh`.

Снимок: time; cpu/memory/disk в %; rx/tx в байт/с; memory_total/memory_used в байтах; load1; disks[]; networks[]; errors{source:message}; collection_ms. `null` — подготовка или отсутствие достоверных данных. Общий disk — максимальное заполнение доступных файловых систем.

История: time; cpu/memory/disk/rx/tx (avg или null); `<metric>_min`; `<metric>_max`; count. Время — начало корзины, step — ширина в секундах. Пропущенные корзины и null нужно рисовать разрывом. CSV интерфейса содержит средние значения, а не сырой архив.

Период должен быть упорядочен и лежать в последних 24 часах. Некорректное тело/период — 400; Host/Origin/Fetch Metadata — 403; тип содержимого — 415; ошибка хранения — 503. CORS не включён. Доступ извне — через SSH.

Маршруты `/events` и `/rules` удалены, возвращают 404.
