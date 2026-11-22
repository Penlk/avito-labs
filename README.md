# Trip Service — лабораторная работа 3

HTTP API поездок и координат, PostgreSQL, наблюдаемость и сканер устаревших поездок.
Сканер выбирает кандидатов и ставит их в ограниченную очередь. Фиксированный пул воркеров
отправляет запросы через Push-гейтвей; сохранение координаты отменяет ожидающую задачу.

Полное условие: [лабораторная 3 — координаты, очередь и worker pool](docs/lab-03/README.md).
В одном README собраны ТЗ, контракты и все действующие для неё требования из общих разделов курса.

## Запуск

Нужны Go 1.27.0, Make, golangci-lint 2.14, Docker и
[tripgoctl](https://github.com/course-go-autumn-2026/course-infra).
Инструменты goose, oapi-codegen и mockgen закреплены в go.mod.

```sh
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect
make migrate
make run
```

`environment.toml` выбирает lab 3 с PostgreSQL, observability и заглушкой Push Service.
`tripgoctl` создаёт `.env`; адреса Jaeger, Grafana и Prometheus берём из
`tripgoctl connect`. Сервис экспортирует телеметрию только в OTel Collector.

`make run` загружает `.env.example`, затем `.env`. Если порт 8080 занят,
можно запустить собранный бинарник с другим адресом:

```sh
make build
set -a
. ./.env.example
. ./.env
HTTP_ADDR=127.0.0.1:18082 ./bin/trip-service
```

SIGINT/SIGTERM запускает graceful shutdown: HTTP перестаёт принимать запросы,
дожидается текущих, затем останавливается и выгружается телеметрия, после чего закрывается пул.
Общий предел — `SHUTDOWN_TIMEOUT`; превышение попадает в JSON-лог.

## Архитектура

```text
cmd/service → config → container
                         ├─ http → usecase → domain
                         ├─ repository → domain, ошибки usecase
                         └─ infra: postgres, transaction, telemetry

HTTP: Recover → Tracing → Logging → Metrics → router → handler
```

- `cmd/service` читает конфигурацию, создаёт контейнер, запускает сервер и
  обрабатывает остановку. Все конкретные зависимости связывает `internal/container`.
- `http/trip/{create,get,finish}` разбирает запросы, вызывает юзкейсы и собирает
  ответы. У каждого действия свой пакет. Общая конвертация Trip → API живёт
  в `http/trip/convert.go`, поскольку формат ответа одинаков.
- `usecase/trip/{create,get,finish}` описывает сценарии. Создание и завершение
  включают запись истории в ту же транзакцию. Бизнес-событие вызывается только
  после успешного коммита, поэтому rollback не увеличивает счётчик поездок.
- `domain/trip` содержит модель с закрытыми полями, переходы состояний и ошибки.
  `Finish(at)` устанавливает время завершения; повторный вызов возвращает
  `ErrTripCompleted` и сохраняет исходное время. Value objects проверяют цену
  и координаты, включая NaN. `ValidationError` содержит поле и причину.
- `repository/trip` содержит SQL и преобразование строк БД в доменную модель.
  Он не импортирует OTel. Собственных интерфейсов ему не требуется.
- `infra/postgres/transaction` передаёт транзакцию через context и выполняет
  commit/rollback. `infra/postgres` подключает otelpgx на уровне пула.
  `infra/telemetry` создаёт SDK, OTLP-экспортёры, JSON-логгер и обработчик
  бизнес-событий. `NewHTTPMetrics` создаёт гистограмму длительности HTTP-запросов
  и счётчик активных запросов; middleware получает готовые инструменты из контейнера.

Контейнер передаёт middleware в `http.NewRouter`, который регистрирует их через
`chi.Router.Use`. HTTP-статус и тело ответа перехватывает `bufferWriter`.

Интерфейсы объявляет потребитель: неэкспортируемые типы в `contract.go` рядом
с каждым хендлером и юзкейсом. Юзкейсы получают репозиторий, TxManager и
обработчик бизнес-событий через конструктор. Моки генерируются в соседний
`mocks_test.go`, пакет `<action>_test`. Зависимость от инфраструктуры в домене,
от транспорта в юзкейсах и от репозиториев/infra в HTTP запрещает depguard.

Отступления от примерного дерева задания: отдельного `model` нет — между
слоями передаётся существующая доменная Trip с закрытыми полями. Дублировать её
в ещё одну структуру для трёх сценариев не требуется. Сохранён State-паттерн
модели; `Finish(at)` использует существующий переход состояния.
Общий тип ошибки валидации находится в `domain/validation`: `validation.Error` с полями `Field` и `Reason`.
Имена `trip.ValidationError`, `vo.ValidationError` и `position.ValidationError` сохранены через alias.

`http/domainerror.Problem(error) (int, api.Problem)` — единое место маппинга
ошибок через `errors.Is/As`. Неизвестная ошибка превращается в
`500 internal_error` без деталей. Ошибки валидации — `400 invalid_request`,
занятый водитель и завершённая поездка — `409`, отсутствующая поездка — `404`.
Поля `instance` и `application/problem+json` выставляет `domainerror.Write`.
Ошибки роутера тоже проходят через него: неизвестный маршрут возвращает
`404 not_found`, неподдерживаемый метод — `405 method_not_allowed`.
Для ответа 405 допустимые методы определяются через `chi.Match` и перечисляются
в заголовке `Allow`.

## Транзакции

Создание пишет `trips` и историю `NULL → active`; завершение обновляет поездку
и пишет `active → completed`. Ошибка любой операции откатывает обе записи.
Вложенный `TxManager.Do` использует текущую транзакцию; коммитит только внешний.
Rollback выполняется и при панике, с отдельным ограниченным контекстом.

Изоляция — READ COMMITTED. `UPDATE ... WHERE status = 'active'` защищает от
повторного завершения. Если строка не обновилась, юзкейс перечитывает поездку,
чтобы различить конкурентное завершение и отсутствие записи. Частичный
уникальный индекс запрещает две активные поездки одного водителя.

Репозиторий `repository/position` сохраняет точки через `Add` и читает маршрут через `GetByTripId`.
ID точки назначает PostgreSQL, маршрут отсортирован по `recorded_at`, затем по `id`.
Для существующей поездки без точек возвращается пустой список, для отсутствующей — `ErrTripNotFound`.

Usecase создания позиции получает зависимости через
`create.New(txManager, positionRepository, tripRepository, queue, events)`.
В одной транзакции он вызывает `tripRepository.UpdateLastPositionAt`, затем `positionRepository.Add`.
Обновление разрешено только для активной поездки; `GREATEST` в SQL не позволяет старой точке уменьшить время.
При ошибке вставки или коммита изменения откатываются. Только после успешной транзакции
вызывается `queue.Cancel(tripID)`. Колонку и таблицу добавляет миграция `00004`.
После коммита также увеличивается `tripgo.positions.saved`; неуспешные транзакции не учитываются.

## Опрос устаревших поездок

```text
PostgreSQL → Scanner.Select → общая ограниченная Queue → фиксированный пул воркеров → Push Service
```

`repository/trip.Select(ctx, staleAfter)` одним запросом выбирает активные поездки,
у которых `COALESCE(last_position_at, started_at)` старше порога. Время проверяется в SQL,
результат отсортирован от самых старых поездок к новым. Миграция `00005` добавляет
частичный индекс; интеграционный тест проверяет его через `EXPLAIN (ANALYZE)` на 12 000 поездок.
Явной транзакции у обхода нет.

`scanner.Scan(ctx)` — блокирующий цикл. Контейнер запускает его одной горутиной,
после завершения HTTP останавливает и дожидается выхода до закрытия телеметрии и БД.
Тикер останавливается, ошибка SQL логируется и не прерывает следующие проверки.
Обходы не выполняются параллельно. В тестах тикер подменяется через `Options.NewTicker`.

`NewQueue(size)` создаёт буферизованный канал и синхронизированную структуру дедупликации.
`TryAdd(ctx, trip)` возвращает `queued`, `duplicate` или `dropped`, не ожидая освобождения канала.
При переполнении задача пропускается, увеличиваются оба счётчика с `result=dropped` и пишется `Warn`.
Повторная постановка подавляется до завершения обработки или отмены задачи.

Воркер читает `Queue.Tasks()`, проверяет `Queue.Start(task)` и вызывает `Complete(task)` после обработки,
включая ошибку или таймаут отправки.
`Cancel(tripID)` логически отменяет старую задачу; её наличие в канале не разрешает отправку.
Задачи различаются по своей identity: отменённая задача и её завершение не затрагивают новую задачу той же поездки.
После остановки сканер закрывает очередь, сохраняя возможность дочитать принятые задачи.
`Queue.Len()` считает элементы канала, включая ещё не извлечённые отменённые задачи.

На каждом тике создаётся спан `positions.scan`. Его SpanContext сохраняется в задаче для span link воркера.
`tripgo.stale_checks{result}` учитывает все исходы постановки,
`tripgo.position_requests{result=dropped}` — переполнение. Тесты проверяют метрики, спаны,
конкурентную дедупликацию, отмену, остановку и отсутствие утечек через `goleak`.

`workers.NewWorker(pusher, timeout, logger, meter, tracer)` создаёт обработчик, общий для всего пула.
Контейнер запускает `POSITION_WORKERS` горутин с вызовом `Work(ctx, queue)`.
Каждая задача получает отдельный контекст с `PUSH_REQUEST_TIMEOUT`; выполняется одна попытка отправки
`REQUEST_POSITION` водителю с `trip_id` в данных. Ошибки логируются, после них воркер продолжает работу.
Повторных попыток и ожидания `Retry-After` в лабораторной 3 нет.

Спан `positions.process` начинает новую трассу и связывается со сканом через link.
Дочерний клиентский спан `push.request` передаётся Push Service через W3C `traceparent`.
Метрики: `tripgo.position_requests{result=success|error}`, гистограмма
`tripgo.position_requests.duration` в секундах, gauges `tripgo.position_requests.queue.size`
и `tripgo.position_requests.workers.active`.

При остановке контейнер завершает HTTP, останавливает сканер и закрывает очередь.
Воркеры дочитывают очередь в пределах общего `SHUTDOWN_TIMEOUT`; по дедлайну их контекст отменяется.
Затем закрываются idle-соединения Push-клиента, телеметрия и пул БД.

## Наблюдаемость

JSON-логи идут в stdout. Logger передаётся зависимостью; его handler берёт
`trace_id` и `span_id` из context. Вне спана эти поля пустые. Access-log один
на запрос: method, route, status, duration_ms. Уровень INFO для 2xx/4xx,
ERROR для 5xx. Тела запросов/ответов, SQL-параметры и данные подключения
не логируются. Успешные бизнес-события пишутся отдельно после транзакции.
Исходная причина внутренней ошибки записывается в поле `error` той же access-log
записи. `domainerror.Record` передаёт её в `bufferWriter`, а вложенные буферы
сохраняют её при переносе ответа до `Logging`. Клиент получает безопасный Problem
без внутренних деталей. Ошибка проверки readiness тоже попадает в access-log;
её HTTP-ответ остаётся `503` с `HealthResponse` по контракту.

HTTP-спан создаёт собственная middleware `Tracing` через `tracer.Start`
с `SpanKindServer`. Имя — метод и шаблон chi-маршрута; если маршрут не найден,
используется `/unmatched`. Перед созданием спана `propagation.TraceContext`
извлекает родительский контекст из HTTP-заголовков `traceparent` и `tracestate`.
Валидный `traceparent` позволяет продолжить входящий трейс; без него или при
повреждённом заголовке обычный HTTP-запрос начинает новый трейс. При стандартном
parent-based sampler учитывается решение родителя о сэмплировании.
Контекст HTTP-спана передаётся в handler, затем через юзкейс и репозиторий до pgx.
Перед завершением HTTP-спана записывается `http.response.status_code`.
Ответы 4xx/5xx помечаются `codes.Error`, а `error.type` содержит строковый код
ответа, например `"409"` или `"500"`. При панике записываются статус 500
и `error.type="panic"`; при ошибке записи ответа — `error.type="response_write"`.
SQL-спаны создаёт otelpgx; адаптер пула добавляет
`db.operation.name` и `db.collection.name`. При ошибке он выставляет статус
`codes.Error` и атрибут `error.type`, а длительность каждого запроса записывает
в гистограмму в секундах.
Лейблы SQL ограничены известными таблицами/операциями, UUID и параметры
запросов туда не попадают.

Метрики сервиса:

| OTel | Prometheus после коллектора |
|---|---|
| `http.server.request.duration` | `http_server_request_duration_seconds_{bucket,count,sum}` |
| `http.server.active_requests` | `http_server_active_requests` |
| `db.client.operation.duration` | `db_client_operation_duration_seconds_{bucket,count,sum}` |
| `tripgo.trips.created` | `tripgo_trips_created_total` |
| `tripgo.trips.completed` | `tripgo_trips_completed_total` |
| `tripgo.trips.active` | `tripgo_trips_active` |
| `tripgo.positions.saved` | `tripgo_positions_saved_total` |

HTTP-метрики используют лейблы `http.request.method` и `http.route`,
гистограмма длительности дополнительно содержит `http.response.status_code`.
Неизвестные HTTP-методы объединяются в `_OTHER`; стандартные сохраняются,
в том числе при ответе 405. Шаблон маршрута определяется до вызова handler
через `chi.Routes.Match`, при отсутствии совпадения используется `/unmatched`.

`tripgo.trips.active` — observable gauge из общего каталога телеметрии курса.
При каждом сборе callback читает `count(*)` активных поездок из PostgreSQL
с таймаутом. Периодический сбор запускает `PeriodicReader` с интервалом
`OTEL_METRIC_EXPORT_INTERVAL`. Эти SQL-запросы также попадают в метрики БД,
поэтому счётчик SELECT растёт даже без HTTP-трафика.
Значение учитывает поездки, созданные до запуска процесса. При ошибке БД
callback сообщает о ней через `otel.Handle`, пропускает наблюдение и возвращает
`nil`, чтобы остальные метрики продолжили экспортироваться. Неизвестное значение
не подменяется нулём. SDK останавливается до закрытия пула, чтобы последний сбор
мог прочитать БД.

OTel resource содержит `service.version`: версию модуля или VCS revision
из метаданных Go-бинарника, с суффиксом `-dirty` для изменённой рабочей копии.
Без метаданных используется `devel`. Версию можно переопределить стандартным
`OTEL_RESOURCE_ATTRIBUTES`; этот же env задаёт namespace и окружение.
Ошибки до создания контейнера также идут одним JSON-сообщением ERROR в stdout.

Встроенные метрики otelpgx отключены через noop MeterProvider; длительность SQL
измеряет адаптер пула. HTTP-метрики записывает собственная middleware `Metrics`.
Инструменты используют общий SDK MeterProvider и экспортируются по OTLP.

Recover буферизует небольшие JSON-ответы: паника после начала записи всё равно
даёт один корректный Problem 500. Внутренние middleware учитывают панику в defer:
активные запросы уменьшаются, метрики/лог получают 500. `Tracing` помечает
HTTP-спан ошибкой и повторно вызывает `panic`, чтобы внешний `Recover`
сформировал ответ; спан завершается после записи атрибутов.
Recover пишет отдельную запись со стеком. Она использует исходный контекст,
поэтому ID внутреннего HTTP-спана в ней могут отсутствовать; access-log
записывается внутри спана и содержит его ID. Буферизация рассчитана на текущий
JSON API; для streaming-ручек её нужно пересмотреть.

Экспорт асинхронный: batch processor для трейсов, periodic reader для метрик.
Очередь трейсов ограничена и не блокирует запросы. Недоступный коллектор
не влияет на readiness и бизнес-операции. Контейнер задаёт глобальный обработчик
через `otel.SetErrorHandler`: ошибки сбора и экспорта телеметрии пишутся как WARN
с сообщением `telemetry error`.
При длительном сбое часть телеметрии может потеряться.

## Проверки и генерация

```sh
make generate
make lint
make test
make test-integration
```

Описание PR, отчёт о проверках, JSON дашборда Grafana и скриншоты собраны в
[`docs/pr/lab-03`](docs/pr/lab-03/README.md).

`make generate-api` отдельно генерирует модели и chi-маршруты из OpenAPI, включая ручки координат.
`make generate` запускает эту генерацию и обновляет моки.

`make test` запускает unit-тесты с `-race -count=2`, печатает покрытие пакетов
и итог, сохраняет `coverage.out`. Доменные тесты проверяют граничные значения
и переходы состояния; gomock-тесты юзкейсов — успех, конфликты, ошибки
репозитория/истории/коммита. HTTP-тесты проверяют контракт и сокрытие деталей.
Тесты middleware проверяют traceparent, имя/статус спана, `error.type`, JSON-логи,
метрики и панику до записи ответа и после частичной записи. Проверяются один
access-log с ID HTTP-спана и отдельная запись паники со стеком.
Проверяется сохранение причины внутренней ошибки в access-log без утечки в ответ.
Тесты роутера проверяют Problem JSON для 400/404/405 и заголовок `Allow`.

Отдельный `TestTracingPropagation` проверяет продолжение входящего трейса,
передачу `tracestate`, создание нового трейса без валидного заголовка
и поведение с несэмплированным родителем.

Тесты телеметрии проверяют значения gauge, пропуск измерения при ошибке БД
и восстановление после ошибки. Отдельный тест с `PeriodicReader` проверяет,
что HTTP-гистограмма доходит до экспортера даже при ошибке подсчёта поездок.

Интеграционные тесты отделены `//go:build integration`, используют PostgreSQL
из `.env`, запускаются с `-race -count=2`. Каждый создаёт отдельную схему,
применяет существующие миграции и удаляет схему через Cleanup. Проверяются
CRUD, ограничения, условное обновление, rollback обеих вставок, rollback
при ошибке истории и откат завершения. Рабочие таблицы сервиса не очищаются.
Тестов, требующих `time.Sleep`, нет.

Дополнительные команды: `make migrate-down`, `make migrate-status`,
`make docker-build`, `make docker-run`, `make docker-stop`.
Существующий Dockerfile из лабы 1 собирает текущий `cmd/service`; Docker
не является дополнительным заданием лабы 2.

## Конфигурация

Все значения задаются через env; безопасный шаблон — `.env.example`.
Адреса PostgreSQL и OTel переопределяются файлом `.env` от tripgoctl.

| Переменная | Назначение |
|---|---|
| `HTTP_ADDR` | Адрес сервера |
| `HTTP_READ_TIMEOUT`, `HTTP_READ_HEADER_TIMEOUT` | Таймауты чтения |
| `HTTP_WRITE_TIMEOUT`, `HTTP_IDLE_TIMEOUT` | Таймауты записи и простоя |
| `LOG_LEVEL` | debug, info, warn, error |
| `SHUTDOWN_TIMEOUT` | Общий бюджет остановки |
| `DATABASE_URL` | DSN PostgreSQL |
| `DATABASE_MAX_CONNS`, `DATABASE_MIN_CONNS` | Размер пула |
| `DATABASE_MAX_CONN_LIFETIME` | Срок жизни соединения |
| `DATABASE_CONNECT_TIMEOUT` | Подключение и стартовый Ping |
| `DATABASE_QUERY_TIMEOUT` | Чтение, readiness и бюджет транзакции |
| `POSITION_CHECK_INTERVAL` | Период сканера, по умолчанию `30s` |
| `POSITION_STALE_AFTER` | Порог устаревания координат, по умолчанию `60s` |
| `POSITION_QUEUE_SIZE` | Ёмкость очереди, по умолчанию `100` |
| `POSITION_WORKERS` | Число воркеров, по умолчанию `4` |
| `PUSH_HTTP_URL` | Базовый URL Push Service из `.env` от tripgoctl |
| `PUSH_REQUEST_TIMEOUT` | Таймаут одной попытки отправки, по умолчанию `1s` |
| `OTEL_RESOURCE_ATTRIBUTES` | service.namespace, deployment.environment; опционально service.version |
| `OTEL_SERVICE_NAME` | Имя сервиса в телеметрии и логах |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Адрес коллектора, включая схему |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | http/protobuf или grpc |
| `OTEL_EXPORTER_OTLP_TIMEOUT` | Таймаут экспорта, миллисекунды |
| `OTEL_TRACES_SAMPLER` | Стандартный OTel sampler, например parentbased_always_on |
| `OTEL_TRACES_SAMPLER_ARG` | Необязательный параметр sampler, например 0.1 |
| `OTEL_METRIC_EXPORT_INTERVAL` | Период экспорта метрик, миллисекунды |
| `OTEL_METRIC_EXPORT_TIMEOUT` | Таймаут экспорта метрик, миллисекунды |

SDK читает стандартные OTEL-переменные сам; дополнительно поддерживает
`OTEL_RESOURCE_ATTRIBUTES` и отдельные настройки экспортёров traces/metrics.
Обязательные поля конфигурации проверяются при запуске. Таймауты приложения
должны быть положительными.
