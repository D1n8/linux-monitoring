# Готовые поставки

Обычный запуск не компилирует приложение. `compose.yaml` содержит только ссылки на образы; сборка вынесена в `compose.build.yaml` и вызывается явно через `./start.sh --build`.

## Запуск на VPS

Поддерживаются Linux amd64 и arm64, Docker Engine 25+ и Compose 2.20.2+.

Два варианта поставки:

1. Из репозитория: `./start.sh` скачивает готовый `ghcr.io/d1n8/goinmetrics:latest` и PostgreSQL. Этот вариант требует опубликованного образа с публичным чтением GHCR.
2. Из архива: распаковать `goinmetrics-linux-amd64.tar.gz` или `goinmetrics-linux-arm64.tar.gz`, перейти в каталог и выполнить `./start.sh`. Образ приложения уже внутри. PostgreSQL скачивается готовым образом при первом запуске. Доступ к GHCR для приложения не нужен.

```bash
tar -xzf goinmetrics-linux-amd64.tar.gz
cd goinmetrics-linux-amd64
./start.sh
```

В обоих случаях пароль создаётся автоматически, схема готовится при запуске, адрес панели — `http://127.0.0.1:9099`. Для доступа извне используется SSH-туннель из README. Остановка — `./stop.sh`; тома и `.env` сохраняются.

Архив содержит только compose.yaml, start.sh, stop.sh, README.txt, PLATFORM, SHA256SUMS и готовый образ goinmetrics-image.tar.gz. Go, Node.js, исходники и Dockerfile на VPS не нужны. Загрузчик проверяет архитектуру и контрольные суммы перед первым импортом. Повторный запуск использует кешированный образ. Первая загрузка PostgreSQL требует сети; если оба образа уже есть, запуск работает без сборки и доступа к registry.

Если готовый образ недоступен, загрузчик завершится с понятной ошибкой. Автоматического перехода к тяжёлой сборке на VPS нет.

## Автоматический выпуск

`.github/workflows/release.yml` запускается по тегу `vMAJOR.MINOR.PATCH`. Он:

1. Выполняет общий CI: Go с PostgreSQL/race detector, TypeScript, тесты сравнения, проверки загрузчика.
2. Собирает и публикует один multiarch-образ для `linux/amd64` и `linux/arm64` в GHCR.
3. Формирует архивы с готовыми образами и SHA256 для обеих архитектур.
4. После успешной упаковки прикладывает архивы к GitHub Release и обновляет тег образа `latest`.

Сборка выполняется на CI, включая Nuxt. Go кросс-компилируется на архитектуре builder; целевой слой содержит только COPY и числовой UID 65532. Эмуляция ARM при сборке не требуется.

Manual workflow_dispatch создаёт версию `sha-<commit>` и сохраняет архивы в Actions artifacts; стабильный `latest` при этом не изменяется. Секреты GHCR вручную не настраиваются: workflow использует выданный GitHub `GITHUB_TOKEN` с ограниченными разрешениями job.

После добавления изменений в GitHub выпуск инициируется обычным тегом:

```bash
git tag v0.3.0
git push origin v0.3.0
```

Для первого публичного скачивания GHCR-пакета владелец должен выставить его visibility=Public. Архивы GitHub Release обходят эту настройку: они содержат образ приложения и доступны согласно правам самого репозитория. [Доступ к пакетам GitHub](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility).

В текущей локальной сессии CI на GitHub не запускался: подключение с правом публикации отсутствует. Локально готовые образы и архивы проверяются отдельно; наличие workflow не означает уже опубликованный релиз.

## Локальная проверка сборки и упаковки

```bash
docker buildx build --platform linux/amd64 --load -t ghcr.io/d1n8/goinmetrics:local-amd64 .
./scripts/package-release.sh ghcr.io/d1n8/goinmetrics:local-amd64 linux/amd64 dist

docker buildx build --platform linux/arm64 --load -t ghcr.io/d1n8/goinmetrics:local-arm64 .
./scripts/package-release.sh ghcr.io/d1n8/goinmetrics:local-arm64 linux/arm64 dist
```

Архивы появляются в dist/ и не хранятся в Git. ARM64-бинарник проверяется по ELF-заголовку; выполнение ARM64 на реальном ARM-хосте требует отдельной проверки, если такой хост недоступен.
