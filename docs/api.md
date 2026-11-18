# HTTP API

База: `http://127.0.0.1:9099/api`. Ответы JSON, даты RFC3339 UTC. Значение `null` означает подготовку или отсутствие достоверного показателя. Ошибки: `{"error":"понятное сообщение"}`. Все пользователи локального сервиса равноправны.

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/health` | Запись БД, last_saved, queue, dropped, missed, started. 503 при последнем сбое хранения |
| GET | `/host` | Имя, Linux, ядро, число CPU, uptime в секундах |
| GET | `/snapshot` | Последнее состояние без отдельного чтения метрик |
| GET | `/stream` | SSE, event `metrics`, один JSON-снимок в `data`; heartbeat каждые 15 секунд |
| GET | `/history?from=…&to=…` | История: points, step, from, to; по умолчанию 15 минут |
| GET | `/rules` | Массив общих правил |
| PUT | `/rules` | Создать/заменить одно правило, `Content-Type: application/json` |
| GET | `/events` | Последние 100 событий по убыванию начала |

Снимок: time; cpu/memory/disk в %; rx/tx в байт/с; memory_total/memory_used в байтах; load1; disks[]; networks[]; errors{source:message}; collection_ms. Набор интерфейсов и дисков может меняться. Общее заполнение дисков — максимум, не сумма.

Точка истории: time; cpu/memory/disk/rx/tx (avg или null); `<metric>_min`; `<metric>_max`; count. Время — начало корзины; step — её ширина в секундах. Отсутствующие корзины нужно рисовать разрывом. Внутренние пропуски делают среднюю точку недостоверной; min/max могут сохранять известные значения.

```bash
curl http://127.0.0.1:9099/api/snapshot
curl -N http://127.0.0.1:9099/api/stream
curl -X PUT http://127.0.0.1:9099/api/rules \
  -H 'Content-Type: application/json' \
  -d '{"metric":"cpu","threshold":85,"duration":30,"enabled":true}'
```

Допустимые metric: cpu, memory, disk, rx, tx. Для процентов threshold 0–100; для сети 0–10^15 байт/с. Duration — целые секунды 1–3600. Для выключения сохраните то же правило с enabled=false. Изменение прерывает старое событие при обработке следующего пакета.

Диапазон истории должен быть упорядочен и лежать в последних 24 часах. Неверный диапазон/правило — 400, недопустимый Host/Origin — 403, неверный Content-Type — 415, БД недоступна — 503. API не предоставляет публичный сетевой доступ или токены: используйте SSH.
