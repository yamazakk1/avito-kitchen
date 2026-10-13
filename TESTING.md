# Тестирование

Проект реализует полный MVP flow:

```text
Restaurant registration
→ Menu synchronization
→ Create PROCESSING order
→ Background delivery to Demo Cafe
→ ACCEPTED / REJECTED
→ GET order
```

Ниже приведены команды автоматических проверок и минимальный ручной E2E flow.

## 1. Автоматические проверки

### Avito Kitchen

Из каталога сервиса:

```bash
cd services/avito-kitchen

go test ./...
go vet ./...
golangci-lint run ./...
```

### PostgreSQL integration tests

Из корня репозитория сначала поднять PostgreSQL и применить миграции:

```bash
docker compose up -d postgres migrate
```

Затем:

```bash
cd services/avito-kitchen
go test -tags=integration -p 1 ./...
```

`-p 1` нужен, потому что integration test packages используют одну общую локальную PostgreSQL database.

### Demo Cafe

```bash
cd services/demo-cafe

go test ./...
go vet ./...
golangci-lint run ./...
```

## 2. Docker Compose

Из корня репозитория:

```bash
docker compose config
docker compose up --build -d
```

Проверить состояние контейнеров:

```bash
docker compose ps
```

Остановка:

```bash
docker compose down
```

Полный сброс PostgreSQL:

```bash
docker compose down -v
```

## 3. Health endpoints

Avito Kitchen:

```text
GET http://localhost:8080/health/live
GET http://localhost:8080/health/ready
```

Ожидается `200 OK`.

Если PostgreSQL недоступен, `/health/ready` должен вернуть `503 Service Unavailable`.

Demo Cafe:

```text
GET http://localhost:8081/health/live
```

Ожидается `200 OK`.

# 4. Полный E2E flow через Postman

Для чистого сценария:

```bash
docker compose down -v
docker compose up --build -d
```

Создать Collection Variables:

```text
base_url = http://localhost:8080
restaurant_id =
menu_item_id =
order_id =
```

## 4.1. Регистрация Demo Cafe

```text
POST {{base_url}}/api/v1/partner/restaurants
Content-Type: application/json
```

```json
{
  "external_id": "demo-cafe-1",
  "name": "Demo Cafe",
  "order_endpoint": "http://demo-cafe:8081/api/v1/orders",
  "timezone": "Europe/Moscow",
  "is_accepting_orders": true,
  "working_hours": [
    { "day_of_week": 1, "mode": "OPEN_24H" },
    { "day_of_week": 2, "mode": "OPEN_24H" },
    { "day_of_week": 3, "mode": "OPEN_24H" },
    { "day_of_week": 4, "mode": "OPEN_24H" },
    { "day_of_week": 5, "mode": "OPEN_24H" },
    { "day_of_week": 6, "mode": "OPEN_24H" },
    { "day_of_week": 7, "mode": "OPEN_24H" }
  ]
}
```

Ожидается `201 Created`.

Сохранить поле `id` ответа как `restaurant_id`.

Postman Tests:

```javascript
const body = pm.response.json();
pm.collectionVariables.set("restaurant_id", body.id);
```

## 4.2. Синхронизация меню

Demo Cafe по умолчанию знает позиции `pizza-1` и `cola-1` и snapshot version `1`, поэтому happy-path использует именно их.

```text
PUT {{base_url}}/api/v1/partner/restaurants/{{restaurant_id}}/menu
Content-Type: application/json
```

```json
{
  "snapshot_version": 1,
  "items": [
    {
      "external_id": "pizza-1",
      "name": "Маргарита",
      "description": "Томаты и моцарелла",
      "price": 75000,
      "available_quantity": 10
    },
    {
      "external_id": "cola-1",
      "name": "Cola",
      "description": null,
      "price": 15000,
      "available_quantity": 20
    }
  ]
}
```

Ожидается `200 OK`.

## 4.3. Получение внутреннего menu item ID

```text
GET {{base_url}}/api/v1/restaurants/{{restaurant_id}}/menu
```

Ожидается `200 OK`, `snapshot_version = 1`.

Сохранить внутренний `id` Маргариты как `menu_item_id`:

```javascript
const body = pm.response.json();
const pizza = body.items.find(item => item.name === "Маргарита");
pm.collectionVariables.set("menu_item_id", pizza.id);
```

Важно: при создании Customer order используется внутренний `menu_item_id` Avito Kitchen, а при интеграции с Cafe Service Avito Kitchen сам использует сохранённый `external_menu_item_id` (`pizza-1`).

## 4.4. Создание заказа

```text
POST {{base_url}}/api/v1/orders
Content-Type: application/json
Idempotency-Key: e2e-order-1
```

```json
{
  "user_id": "postman-user-1",
  "restaurant_id": "{{restaurant_id}}",
  "items": [
    {
      "menu_item_id": "{{menu_item_id}}",
      "quantity": 2,
      "expected_price": 75000
    }
  ]
}
```

Ожидается `201 Created`.

Сразу после create ответ может содержать:

```text
status = PROCESSING
external_order_id = null
rejection_reason = null
total_price = 150000
```

Сохранить `id` заказа:

```javascript
const body = pm.response.json();
pm.collectionVariables.set("order_id", body.id);
```

## 4.5. Проверка фоновой обработки

Подождать примерно 1–2 секунды и выполнить:

```text
GET {{base_url}}/api/v1/orders/{{order_id}}
```

Ожидаемый happy-path результат:

```text
status = ACCEPTED
external_order_id != null
rejection_reason = null
total_price = 150000
```

Если заказ остаётся `PROCESSING` несколько секунд, проверить логи:

```bash
docker compose logs avito-kitchen
docker compose logs demo-cafe
```

# 5. Критичные edge cases

## 5.1. Idempotency create-order

Повторить тот же `POST /api/v1/orders` с тем же:

```text
Idempotency-Key: e2e-order-1
```

и тем же body.

Должен вернуться тот же `order.id`, новый заказ не создаётся.

Затем оставить ключ тем же, но изменить `quantity`. Ожидается:

```text
409 Conflict
IDEMPOTENCY_KEY_REUSED
```

## 5.2. PRICE_CHANGED

Использовать новый `Idempotency-Key` и отправить неверную цену, например:

```json
{
  "user_id": "postman-user-1",
  "restaurant_id": "{{restaurant_id}}",
  "items": [
    {
      "menu_item_id": "{{menu_item_id}}",
      "quantity": 1,
      "expected_price": 1
    }
  ]
}
```

Ожидается:

```text
409 Conflict
PRICE_CHANGED
```

Заказ не создаётся.

## 5.3. Недостаточный локальный stock

```text
PATCH {{base_url}}/api/v1/partner/restaurants/{{restaurant_id}}/menu-items/pizza-1/availability
Content-Type: application/json
```

```json
{
  "available_quantity": 1
}
```

После этого попытка заказать `quantity = 2` должна вернуть:

```text
409 Conflict
INSUFFICIENT_STOCK
```

## 5.4. Недоступный ресторан

```text
PATCH {{base_url}}/api/v1/partner/restaurants/{{restaurant_id}}
Content-Type: application/json
```

```json
{
  "is_accepting_orders": false
}
```

Новый заказ должен вернуть:

```text
409 Conflict
RESTAURANT_UNAVAILABLE
```

## 5.5. Stale menu snapshot

После успешной загрузки `snapshot_version = 2` попытаться загрузить `snapshot_version = 1`.

Ожидается:

```text
409 Conflict
STALE_SNAPSHOT_UPDATE
```

Старый snapshot не должен изменить текущее меню.

## 5.6. Technical failure Cafe Service

После создания корректного `PROCESSING` заказа временно остановить Demo Cafe:

```bash
docker compose stop demo-cafe
```

Order Processor не должен переводить заказ в `REJECTED`; заказ остаётся `PROCESSING`.

После запуска Cafe обратно:

```bash
docker compose start demo-cafe
```

следующий retry должен снова попытаться доставить заказ.

# 6. Прямой тест Demo Cafe

Demo Cafe endpoint можно проверить отдельно:

```text
POST http://localhost:8081/api/v1/orders
Content-Type: application/json
```

```json
{
  "avito_order_id": "00000000-0000-4000-8000-000000000001",
  "restaurant_external_id": "demo-cafe-1",
  "menu_snapshot_version": 1,
  "items": [
    {
      "external_item_id": "pizza-1",
      "quantity": 1
    }
  ]
}
```

Ожидается `200 OK` и `ACCEPTED`.

Повтор точно того же запроса должен вернуть тот же результат и не списать stock второй раз.

Demo Cafe имеет in-memory состояние, поэтому его restart сбрасывает stock и историю обработанных заказов.
