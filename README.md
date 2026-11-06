# Trip Service — лабораторная работа 2

HTTP API создания, получения и завершения поездок. PostgreSQL хранит поездки
и историю статусов. Обязательная часть лабораторной 2 реализована;
мутационное тестирование (дополнительное задание) не выполнялось.
HTTP-контракт и миграции остались прежними.

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

`environment.toml` выбирает lab 2 с PostgreSQL и observability.
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
                         ├─ repository → domain, контракты usecase
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
  бизнес-событий.

Интерфейсы объявляет потребитель: неэкспортируемые типы в `contract.go` рядом
с каждым хендлером и юзкейсом. Юзкейсы получают репозиторий, TxManager и
обработчик бизнес-событий через конструктор. Моки генерируются в соседний
`mocks_test.go`, пакет `<action>_test`. Зависимость от инфраструктуры в домене,
от транспорта в юзкейсах и от репозиториев/infra в HTTP запрещает depguard.

Отступления от примерного дерева задания: отдельного `model` нет — между
слоями передаётся существующая доменная Trip с закрытыми полями. Дублировать её
в ещё одну структуру для трёх сценариев не требуется. Сохранён State-паттерн
модели; `Finish(at)` использует существующий переход состояния.
Тип ошибки value objects находится в `domain/trip/vo/errors.go` и доступен как
`trip.ValidationError` через alias, чтобы избежать цикла импортов.

`http/domainerror.Problem(error) (int, api.Problem)` — единое место маппинга
ошибок через `errors.Is/As`. Неизвестная ошибка превращается в
`500 internal_error` без деталей. Ошибки валидации — `400 invalid_request`,
занятый водитель и завершённая поездка — `409`, отсутствующая поездка — `404`.
Поля `instance` и `application/problem+json` выставляет `domainerror.Write`.

## Транзакции

Создание пишет `trips` и историю `NULL → active`; завершение обновляет поездку
и пишет `active → completed`. Ошибка любой операции откатывает обе записи.
Вложенный `TxManager.Do` использует текущую транзакцию; коммитит только внешний.
Rollback выполняется и при панике, с отдельным ограниченным контекстом.

Изоляция — READ COMMITTED. `UPDATE ... WHERE status = 'active'` защищает от
повторного завершения. Если строка не обновилась, юзкейс перечитывает поездку,
чтобы различить конкурентное завершение и отсутствие записи. Частичный
уникальный индекс запрещает две активные поездки одного водителя.

## Наблюдаемость

JSON-логи идут в stdout. Logger передаётся зависимостью; его handler берёт
`trace_id` и `span_id` из context. Вне спана эти поля пустые. Access-log один
на запрос: method, route, status, duration_ms. Уровень INFO для 2xx/4xx,
ERROR для 5xx. Тела запросов/ответов, SQL-параметры и данные подключения
не логируются. Успешные бизнес-события пишутся отдельно после транзакции.

HTTP-спан создаёт otelhttp, имя — метод и шаблон chi-маршрута. Входящий
`traceparent` сохраняет связь с вызывающим сервисом. SQL-спаны создаёт otelpgx;
адаптер пула добавляет `db.operation.name`, `db.collection.name`, `error.type`
и гистограмму в секундах. Лейблы SQL ограничены известными таблицами/операциями,
UUID и параметры запросов туда не попадают.

Обязательные метрики:

| OTel | Prometheus после коллектора |
|---|---|
| `http.server.request.duration` | `http_server_request_duration_seconds_{bucket,count,sum}` |
| `http.server.active_requests` | `http_server_active_requests` |
| `db.client.operation.duration` | `db_client_operation_duration_seconds_{bucket,count,sum}` |
| `tripgo.trips.created` | `tripgo_trips_created_total` |
| `tripgo.trips.completed` | `tripgo_trips_completed_total` |
| `tripgo.trips.active` | `tripgo_trips_active` |

`tripgo.trips.active` — observable gauge из общего каталога телеметрии курса.
При каждом сборе он читает `count(*)` активных поездок из PostgreSQL с таймаутом.
Значение учитывает поездки, созданные до запуска процесса; при ошибке БД
наблюдение пропускается, а не подменяется нулём. SDK останавливается до закрытия
пула, чтобы последний сбор мог прочитать БД.

OTel resource содержит `service.version`: версию модуля или VCS revision
из метаданных Go-бинарника, с суффиксом `-dirty` для изменённой рабочей копии.
Без метаданных используется `devel`. Версию можно переопределить стандартным
`OTEL_RESOURCE_ATTRIBUTES`; этот же env задаёт namespace и окружение.
Ошибки до создания контейнера также идут одним JSON-сообщением ERROR в stdout.

Встроенные метрики otelhttp/otelpgx отключены через noop MeterProvider,
поскольку их набор/единицы не совпадают с обязательной таблицей задания.
Собственные инструменты используют общий SDK MeterProvider и экспортируются
по OTLP. Повторных HTTP/SQL-спанов при этом не создаётся.

Recover буферизует небольшие JSON-ответы: паника после начала записи всё равно
даёт один корректный Problem 500. Внутренние middleware учитывают панику в defer:
активные запросы уменьшаются, метрики/лог получают 500, спан помечается ошибкой.
Recover пишет отдельную запись со стеком. Буферизация рассчитана на текущий
JSON API; для streaming-ручек её нужно пересмотреть.

Экспорт асинхронный: batch processor для трейсов, periodic reader для метрик.
Очередь трейсов ограничена и не блокирует запросы. Недоступный коллектор
не влияет на readiness и бизнес-операции; ошибка экспорта пишется как WARN.
При длительном сбое часть телеметрии может потеряться.

Скриншоты и результаты живой проверки: [docs/lab2/verification.md](docs/lab2/verification.md).

## Проверки и генерация

```sh
make generate
make lint
make test
make test-integration
```

`make test` запускает unit-тесты с `-race -count=2`, печатает покрытие пакетов
и итог, сохраняет `coverage.out`. Доменные тесты проверяют граничные значения
и переходы состояния; gomock-тесты юзкейсов — успех, конфликты, ошибки
репозитория/истории/коммита. HTTP-тесты проверяют контракт и сокрытие деталей.
Тесты middleware проверяют traceparent, имя/статус спана, JSON-логи, метрики
и панику после частичной записи ответа.

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
