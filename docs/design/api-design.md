# API Design

## 1. Назначение документа

Документ фиксирует API-контракт MVP **Avito Kitchen** и основные правила взаимодействия между:

* User и Avito Kitchen;
* Cafe Service и Avito Kitchen;
* Avito Kitchen и Cafe Service.

API проектируется на основании ранее определённых:

* границ MVP;
* зон ответственности систем;
* источников истины;
* бизнес-сценариев.

На данном этапе фиксируются:

* необходимые операции;
* назначение endpoint'ов;
* структура основных запросов и ответов;
* бизнес-правила API;
* модель обработки ошибок;
* правила идемпотентности;
* правила синхронизации меню;
* lifecycle заказа.

Точный формальный контракт будет дополнительно описан в OpenAPI specification.

---

# 2. Общая структура API

API Avito Kitchen логически разделяется на две части.

## Customer API

Используется клиентской частью Avito Kitchen.

```text
/api/v1/...
```

Основные возможности:

* просмотр заведений;
* просмотр меню;
* создание заказа;
* получение состояния заказа.

---

## Partner API

Используется информационными системами заведений.

```text
/api/v1/partner/...
```

Основные возможности:

* регистрация заведения;
* обновление информации о заведении;
* полная синхронизация меню;
* оперативное обновление доступности товаров.

Несмотря на отсутствие авторизации в MVP, Customer API и Partner API разделяются на уровне URL для явного разграничения двух различных контрактов.

---

# 3. Общие правила API

## 3.1. Формат данных

Основной формат обмена:

```text
application/json
```

---

## 3.2. Идентификаторы

Avito Kitchen использует собственные идентификаторы ресурсов.

Например:

```text
restaurant_id
order_id
menu_item_id
```

Cafe Service при этом может иметь собственные идентификаторы.

Для связи систем используется `external_id`.

Пример:

```text
Avito Kitchen:

menu_item_id = 018f...

Cafe Service:

external_id = pizza-42
```

Таким образом, Avito Kitchen не зависит от внутренней схемы идентификаторов конкретного Cafe Service.

---

## 3.3. Представление денежных значений

Денежные значения передаются целым числом в минимальных денежных единицах.

Например:

```json
{
  "price": 75000
}
```

означает:

```text
750.00 RUB
```

Использование floating-point типов для хранения и передачи денежных значений не предполагается.

---

## 3.4. Время

Временные значения передаются в формате RFC 3339.

Пример:

```json
{
  "updated_at": "2026-08-27T02:10:15Z"
}
```

---

# 4. Customer API

## 4.1. Получение доступных заведений

```http
GET /api/v1/restaurants
```

### Назначение

Позволяет пользователю получить список заведений, доступных для оформления заказа в текущий момент.

### Основной сценарий

`Просмотр доступных заведений и меню`.

### Пример ответа

```json
{
  "restaurants": [
    {
      "id": "restaurant-1",
      "name": "Coffee Place",
      "is_available": true
    }
  ]
}
```

### Бизнес-правила

Avito Kitchen самостоятельно определяет доступность заведения.

При определении доступности могут учитываться:

* рабочий статус;
* график работы;
* возможность принимать новые заказы.

Полученный пользователем результат не резервирует состояние заведения.

Заведение должно повторно проверяться при создании заказа.

---

# 5. Получение меню заведения

```http
GET /api/v1/restaurants/{restaurant_id}/menu
```

### Назначение

Возвращает локальное представление актуального известного Avito Kitchen меню заведения.

### Основной сценарий

`Просмотр доступных заведений и меню`.

### Пример ответа

```json
{
  "restaurant_id": "restaurant-1",
  "items": [
    {
      "id": "menu-item-1",
      "name": "Маргарита",
      "description": "Томаты, моцарелла",
      "price": 75000,
      "available_quantity": 5
    }
  ]
}
```

### Бизнес-правила

Customer API не обращается к Cafe Service для каждого запроса меню.

Avito Kitchen использует локально синхронизированное представление данных Cafe Service.

Полученная пользователем:

* цена;
* доступность;
* количество

не резервируются до момента создания заказа.

---

# 6. Создание заказа

```http
POST /api/v1/orders
```

### Назначение

Создаёт заказ из выбранных пользователем позиций.

Это основной write-operation Customer API.

---

## Request

```json
{
  "restaurant_id": "restaurant-1",
  "items": [
    {
      "menu_item_id": "menu-item-1",
      "quantity": 2,
      "expected_price": 75000
    }
  ]
}
```

---

## `expected_price`

`expected_price` — цена позиции, которую пользователь видел и согласился подтвердить.

Поле **не является источником цены для Avito Kitchen**.

Avito Kitchen самостоятельно получает актуальную стоимость позиции.

Пример:

```text
expected_price = 75000
current_price  = 75000

→ OK
```

Если цена изменилась:

```text
expected_price = 75000
current_price  = 80000

→ PRICE_CHANGED
```

В таком случае заказ автоматически по новой стоимости не подтверждается.

Пользователь должен получить актуальную цену и повторно подтвердить своё намерение.

---

## Проверки

Перед созданием заказа Avito Kitchen должен проверить:

1. существование заведения;
2. возможность заведения принимать заказы;
3. наличие хотя бы одной позиции;
4. принадлежность всех позиций выбранному заведению;
5. существование позиций;
6. доступность позиций;
7. корректность количества;
8. соответствие `expected_price` актуальной стоимости.

После этого Avito Kitchen:

1. рассчитывает итоговую стоимость;
2. сохраняет заказ;
3. устанавливает статус `PROCESSING`;
4. возвращает созданный заказ пользователю.

---

## Response

```http
201 Created
```

```json
{
  "id": "order-123",
  "restaurant_id": "restaurant-1",
  "status": "PROCESSING",
  "total_price": 150000,
  "created_at": "2026-08-27T02:15:00Z"
}
```

Передача заказа Cafe Service не должна блокировать пользовательский запрос до момента завершения обработки заведением.

---

# 7. Идемпотентность создания заказа

Повтор пользовательского запроса не должен приводить к созданию нескольких одинаковых заказов.

Для этого `POST /orders` использует:

```http
Idempotency-Key: <uuid>
```

Пример:

```http
POST /api/v1/orders
Idempotency-Key: 44d61e7d-1ff2-48f5-ae30-b82d0fd23a43
```

При повторной отправке запроса с тем же ключом Avito Kitchen должен вернуть результат ранее созданной операции вместо создания нового заказа.

Это защищает от ситуации:

```text
User
  │
  │ POST /orders
  ▼
Avito

заказ создан

  X response потерян

User делает retry

  │
  │ POST /orders
  ▼
Avito
```

Без идемпотентности здесь могло бы появиться два заказа.

---

# 8. Получение заказа

```http
GET /api/v1/orders/{order_id}
```

### Назначение

Позволяет клиенту получить текущее состояние ранее созданного заказа.

### Основной сценарий

Используется после асинхронного создания заказа.

### Пример ответа

```json
{
  "id": "order-123",
  "restaurant_id": "restaurant-1",
  "status": "ACCEPTED",
  "total_price": 150000,
  "items": [
    {
      "menu_item_id": "menu-item-1",
      "name": "Маргарита",
      "unit_price": 75000,
      "quantity": 2
    }
  ],
  "created_at": "2026-08-27T02:15:00Z",
  "updated_at": "2026-08-27T02:15:02Z"
}
```

---

# 9. Lifecycle заказа

Для MVP достаточно трёх публичных состояний:

```text
              ┌──────────→ ACCEPTED
              │
PROCESSING ───┤
              │
              └──────────→ REJECTED
```

## PROCESSING

Заказ успешно зарегистрирован в Avito Kitchen и ожидает результата обработки Cafe Service.

## ACCEPTED

Cafe Service подтвердил возможность выполнения заказа.

## REJECTED

Cafe Service корректно обработал запрос, но не может выполнить заказ.

---

## Возможное развитие

Текущая модель позволяет позднее расширить lifecycle:

```text
CREATED
   ↓
WAITING_FOR_PAYMENT
   ↓
PAID
   ↓
WAITING_FOR_CAFE
   ↓
ACCEPTED
   ↓
PREPARING
   ↓
READY
   ↓
COMPLETED
```

Эти состояния не входят в текущий MVP.

---

# 10. Partner API

# 10.1. Регистрация заведения

```http
POST /api/v1/partner/restaurants
```

### Назначение

Позволяет Cafe Service зарегистрировать заведение в Avito Kitchen.

### Request

```json
{
  "external_id": "cafe-42",
  "name": "Coffee Place",
  "order_endpoint": "http://demo-cafe:8081/api/v1/orders",
  "working_hours": [
    {
      "day": "MONDAY",
      "opens_at": "09:00",
      "closes_at": "22:00"
    }
  ]
}
```

### Response

```http
201 Created
```

```json
{
  "id": "restaurant-1",
  "external_id": "cafe-42",
  "created_at": "2026-08-27T02:00:00Z"
}
```

### Бизнес-правила

`external_id` должен однозначно идентифицировать заведение внутри Cafe Service.

Avito Kitchen создаёт собственный `restaurant_id`.

---

# 11. Обновление данных заведения

```http
PATCH /api/v1/partner/restaurants/{restaurant_id}
```

### Назначение

Обновляет изменяемые свойства зарегистрированного заведения.

Пример:

```json
{
  "name": "Coffee Place",
  "is_accepting_orders": false
}
```

Операция может использоваться для изменения:

* названия;
* рабочего состояния;
* графика;
* возможности принимать новые заказы;
* других изменяемых свойств заведения.

---

# 12. Полная синхронизация меню

```http
PUT /api/v1/partner/restaurants/{restaurant_id}/menu
```

### Назначение

Cafe Service передаёт полное актуальное представление своего меню.

Операция имеет семантику:

> Сделать меню Avito Kitchen соответствующим переданному snapshot.

Это не частичный upsert.

---

## Request

```json
{
  "snapshot_version": 42,
  "items": [
    {
      "external_id": "pizza-1",
      "name": "Маргарита",
      "description": "Томаты, моцарелла",
      "price": 75000,
      "available_quantity": 5
    },
    {
      "external_id": "cola-1",
      "name": "Cola",
      "description": null,
      "price": 15000,
      "available_quantity": 10
    }
  ]
}
```

---

## Семантика полного snapshot

Если ранее Avito Kitchen содержал:

```text
Pizza
Cola
Burger
```

а новый snapshot содержит:

```text
Pizza
Cola
```

то после успешной синхронизации:

```text
Burger
```

не должен больше считаться частью актуального меню.

Конкретная реализация может физически удалять позицию или переводить её в неактивное состояние.

Это решение определяется на этапе проектирования БД.

---

# 13. Атомарность обновления меню

Полная синхронизация должна применяться атомарно.

Упрощённо:

```text
BEGIN

apply complete snapshot

update snapshot metadata

COMMIT
```

При ошибке:

```text
ROLLBACK
```

Не должно существовать наблюдаемого состояния:

```text
часть старого меню
+
часть нового меню
```

---

# 14. Версионирование snapshot

Каждый логически новый snapshot имеет:

```text
snapshot_version
```

Версия монотонно увеличивается Cafe Service.

Пример:

```text
41
 ↓
42
 ↓
43
```

При повторной отправке того же snapshot после сетевой ошибки Cafe Service может повторно отправить:

```text
snapshot_version = 42
```

Полная операция `PUT` должна быть безопасна для retry.

Cafe Service не должен использовать одну и ту же версию для разных логических состояний меню.

---

# 15. Ответ на синхронизацию меню

После успешного применения Avito Kitchen возвращает:

```json
{
  "snapshot_version": 42,
  "updated_at": "2026-08-27T02:10:15Z",
  "items_count": 2
}
```

Используются два разных значения:

### `snapshot_version`

Используется системами для определения логической версии меню.

### `updated_at`

Показывает время последней успешно применённой синхронизации.

`updated_at` используется для:

* observability;
* диагностики;
* отображения состояния синхронизации.

Версия snapshot используется для проверки консистентности между системами.

---

# 16. Обновление доступности товара

Количество товара может изменяться значительно чаще, чем основная структура меню.

Поэтому для оперативного обновления состояния предусмотрена отдельная операция:

```http
PATCH /api/v1/partner/restaurants/{restaurant_id}/menu-items/{external_item_id}/availability
```

### Request

```json
{
  "available_quantity": 3
}
```

или:

```json
{
  "available_quantity": 0
}
```

### Назначение

Операция позволяет Cafe Service быстро обновлять локальное представление наличия товара без передачи полного snapshot меню.

---

# 17. Особенности availability

Информация о количестве в Avito Kitchen является предварительным представлением состояния Cafe Service.

Она используется:

* при отображении меню;
* при предварительной проверке заказа.

Она не является абсолютной гарантией наличия.

Cafe Service выполняет окончательную проверку при получении заказа.

---

# 18. Demo Cafe API

Demo Cafe является отдельным сервисом.

Для выполнения основного сценария достаточно одного бизнес-endpoint.

```http
POST /api/v1/orders
```

Avito Kitchen использует его для передачи сформированного заказа заведению.

---

# 19. Передача заказа Cafe Service

### Request

```json
{
  "avito_order_id": "order-123",
  "menu_snapshot_version": 42,
  "items": [
    {
      "external_item_id": "pizza-1",
      "quantity": 2
    }
  ]
}
```

### `avito_order_id`

Используется:

* для связывания заказов двух систем;
* как идентификатор идемпотентности при повторной доставке.

### `menu_snapshot_version`

Позволяет Cafe Service определить, на основании какой версии меню Avito Kitchen сформировал заказ.

---

# 20. Успешное принятие Cafe Service

```json
{
  "external_order_id": "cafe-order-991",
  "status": "ACCEPTED"
}
```

Cafe Service создаёт собственный идентификатор:

```text
external_order_id
```

Avito Kitchen сохраняет связь:

```text
Avito order_id
        ↕
Cafe external_order_id
```

---

# 21. Отклонение Cafe Service

Cafe Service может корректно обработать запрос и определить, что заказ выполнить невозможно.

Пример:

```json
{
  "status": "REJECTED",
  "reason": "OUT_OF_STOCK"
}
```

Возможные причины MVP:

```text
OUT_OF_STOCK
ITEM_UNAVAILABLE
NOT_ACCEPTING_ORDERS
STALE_MENU_SNAPSHOT
```

---

# 22. STALE_MENU_SNAPSHOT

Особый случай возникает, когда Cafe Service имеет более новую версию меню, чем Avito Kitchen.

Например:

```text
Cafe Service:

menu_snapshot_version = 43


Avito Kitchen:

menu_snapshot_version = 42
```

Avito отправляет заказ:

```text
menu_snapshot_version = 42
```

Cafe Service обнаруживает рассинхронизацию и отвечает:

```json
{
  "status": "REJECTED",
  "reason": "STALE_MENU_SNAPSHOT",
  "current_snapshot_version": 43
}
```

Это является бизнес-конфликтом состояния, а не внутренней технической ошибкой.

После обнаружения рассинхронизации Cafe Service должен иметь возможность повторить полную синхронизацию меню.

---

# 23. Идемпотентность Avito Kitchen → Cafe Service

Передача заказа должна быть безопасна для retry.

Возможный сценарий:

```text
Avito Kitchen
      │
      │ order-123
      ▼
Cafe Service

заказ создан

      X

ответ потерян
```

Avito Kitchen не знает результата и повторяет запрос:

```text
Avito Kitchen
      │
      │ order-123
      ▼
Cafe Service
```

Cafe Service не должен создавать второй заказ.

Для этого:

```text
avito_order_id
```

должен однозначно идентифицировать заказ Avito Kitchen.

При повторном запросе Cafe Service возвращает результат уже обработанного заказа.

---

# 24. Асинхронная обработка заказа

Customer API не должен быть напрямую связан со временем ответа Cafe Service.

Основной flow:

```text
User
  │
  │ POST /orders
  ▼
Avito Kitchen
  │
  │ persist
  ▼
Order = PROCESSING

  │
  └──────────────→ response to User


Background processing
        │
        ▼
   Cafe Service
        │
   ACCEPTED / REJECTED
        │
        ▼
   Avito Kitchen
```

После этого пользователь получает результат через:

```http
GET /api/v1/orders/{order_id}
```

---

# 25. Надёжность фоновой обработки

Заказ должен быть сохранён до начала взаимодействия с Cafe Service.

Не предполагается использование только in-memory механизма вида:

```go
go sendOrder()
```

поскольку при завершении процесса такой заказ может быть потерян.

Минимальная реализация MVP может использовать PostgreSQL как persistent storage очереди обработки:

```text
order.status = PROCESSING
```

Фоновый worker получает необработанные заказы и пытается передать их Cafe Service.

---

# 26. Retry Cafe Service

Технические временные ошибки могут быть повторены.

Например:

```text
connection refused
timeout
HTTP 5xx
```

При этом бизнес-ответ:

```text
REJECTED
```

не должен автоматически retry'иться как техническая ошибка.

Таким образом:

```text
technical failure
        ↓
retry may be performed


business rejection
        ↓
final business result
```

---

# 27. Ошибки Customer API

API должен использовать единый формат ошибок.

Пример:

```json
{
  "error": {
    "code": "PRICE_CHANGED",
    "message": "One or more item prices have changed",
    "details": {
      "items": [
        {
          "menu_item_id": "menu-item-1",
          "expected_price": 75000,
          "current_price": 80000
        }
      ]
    }
  }
}
```

---

# 28. Основные бизнес-ошибки

## `PRICE_CHANGED`

Цена одной или нескольких позиций изменилась после того, как пользователь увидел меню.

Рекомендуемый HTTP status:

```http
409 Conflict
```

---

## `RESTAURANT_UNAVAILABLE`

Заведение больше не принимает заказы.

```http
409 Conflict
```

---

## `ITEM_UNAVAILABLE`

Выбранная позиция больше недоступна.

```http
409 Conflict
```

---

## `INSUFFICIENT_STOCK`

Известного системе количества недостаточно для формирования заказа.

```http
409 Conflict
```

---

## `RESTAURANT_NOT_FOUND`

Запрошенное заведение не существует.

```http
404 Not Found
```

---

## `ORDER_NOT_FOUND`

Заказ не существует.

```http
404 Not Found
```

---

## `INVALID_REQUEST`

Запрос нарушает контракт API.

Например:

* пустой список товаров;
* отрицательное количество;
* отсутствует обязательное поле.

```http
400 Bad Request
```

---

# 29. Business errors и technical errors

Важно различать два класса результата.

## Business error

Система работает корректно, но выполнить бизнес-операцию невозможно.

Например:

```text
PRICE_CHANGED
OUT_OF_STOCK
RESTAURANT_UNAVAILABLE
STALE_MENU_SNAPSHOT
```

---

## Technical error

Операция не была корректно обработана из-за технической проблемы.

Например:

```text
database unavailable
internal server error
Cafe Service timeout
network error
```

Это различие особенно важно при определении retry policy.

---

# 30. Сводка Avito Kitchen API

## Customer API

| Method | Endpoint                                   | Назначение                   |
| ------ | ------------------------------------------ | ---------------------------- |
| `GET`  | `/api/v1/restaurants`                      | Получить доступные заведения |
| `GET`  | `/api/v1/restaurants/{restaurant_id}/menu` | Получить меню                |
| `POST` | `/api/v1/orders`                           | Создать заказ                |
| `GET`  | `/api/v1/orders/{order_id}`                | Получить состояние заказа    |

## Partner API

| Method  | Endpoint                                                                                 | Назначение                      |
| ------- | ---------------------------------------------------------------------------------------- | ------------------------------- |
| `POST`  | `/api/v1/partner/restaurants`                                                            | Зарегистрировать заведение      |
| `PATCH` | `/api/v1/partner/restaurants/{restaurant_id}`                                            | Обновить данные заведения       |
| `PUT`   | `/api/v1/partner/restaurants/{restaurant_id}/menu`                                       | Полностью синхронизировать меню |
| `PATCH` | `/api/v1/partner/restaurants/{restaurant_id}/menu-items/{external_item_id}/availability` | Обновить наличие                |

Итого:

```text
8 business endpoints
```

---

# 31. Demo Cafe API

| Method | Endpoint         | Назначение                      |
| ------ | ---------------- | ------------------------------- |
| `POST` | `/api/v1/orders` | Получить заказ от Avito Kitchen |

Итого вся демонстрационная система использует:

```text
Avito Kitchen: 8 endpoints
Demo Cafe:     1 endpoint

Total:         9 endpoints
```

---

# 32. Infrastructure endpoints

Infrastructure endpoints не относятся к бизнес API.

Планируется добавить:

```http
GET /health/live
GET /health/ready
```

`live` показывает, что процесс приложения работает.

`ready` показывает, что приложение готово обслуживать запросы и имеет доступ к обязательным зависимостям.

---

# 33. Соответствие API бизнес-сценариям

| Сценарий                    | Основные операции                                  |
| --------------------------- | -------------------------------------------------- |
| Просмотр заведений          | `GET /restaurants`                                 |
| Просмотр меню               | `GET /restaurants/{id}/menu`                       |
| Успешное создание заказа    | `POST /orders`                                     |
| Получение результата заказа | `GET /orders/{id}`                                 |
| Изменение цены              | `PUT /partner/.../menu` + `POST /orders`           |
| Товар закончился            | `PATCH .../availability` + `POST /orders`          |
| Заведение стало недоступно  | `PATCH /partner/restaurants/{id}` + `POST /orders` |
| Cafe Service отклонил заказ | Cafe `POST /orders`                                |
| Обновление меню             | `PUT /partner/.../menu`                            |

Таким образом, каждый бизнес-endpoint связан с конкретным ранее определённым сценарием.

---

# 34. Принципы проектирования API

Для MVP фиксируются следующие принципы.

### API следует бизнес-сценариям

Endpoint не добавляется только потому, что он выглядит типичным CRUD endpoint.

Для каждой операции должна существовать бизнес-причина.

### Server-side данные являются источником истины

Клиент не определяет:

* цену;
* итоговую стоимость;
* доступность;
* состояние заказа.

### Изменяемые данные перепроверяются перед write-operation

Ранее прочитанное состояние не считается гарантированным.

### Retry должен быть безопасным

Операции, которые потенциально повторяются после сетевых ошибок, должны быть идемпотентными или иметь idempotency mechanism.

### Бизнес-конфликты не маскируются под технические ошибки

`REJECTED`, `PRICE_CHANGED` или `OUT_OF_STOCK` являются ожидаемыми состояниями бизнес-процесса.

### Сложность вводится только при наличии причины

В MVP не требуется добавлять брокер сообщений, отдельный cache или микросервисную архитектуру только ради демонстрации технологий.


