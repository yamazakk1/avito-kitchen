# AI-assisted development plan

ИИ использовался как инженерный помощник, а не как источник требований. До реализации основных endpoint'ов были отдельно зафиксированы scope, бизнес-сценарии, API-контракт, data model и C4.

Работа выполнялась небольшими этапами:

1. зафиксировать границы MVP и ownership данных;
2. описать пользовательские и partner-сценарии;
3. спроектировать OpenAPI и PostgreSQL model;
4. подготовить bootstrap двух Go-сервисов и Docker Compose;
5. реализовать Restaurant vertical slice;
6. реализовать Menu snapshot и availability;
7. реализовать атомарное создание `PROCESSING` order и idempotency;
8. реализовать чтение заказа;
9. реализовать Demo Cafe и его идемпотентную обработку;
10. реализовать background order processor и Cafe HTTP client;
11. проверить happy-path и failure scenarios через unit/integration tests и Postman;
12. провести финальную сверку кода с OpenAPI, design docs и README.

При работе с ИИ использовались следующие ограничения:

- не менять архитектуру без явного изменения design docs;
- не добавлять технологии без необходимости для MVP;
- не выполнять внешний HTTP внутри DB transaction;
- соблюдать `expected_price`, menu snapshot version и idempotency invariants;
- делать изменения небольшими и проверяемыми;
- после каждого этапа отдельно проверять diff, тесты, линтер и ручной flow;
- автор проекта должен уметь объяснить каждое принятое решение и сгенерированный участок кода.

Основные source-of-truth документы проекта:

- `docs/design/scopes-and-responsibilities.md`;
- `docs/design/business-scenarios.md`;
- `docs/design/api-design.md`;
- `docs/design/data-model.md`;
- `api/avito-kitchen.openapi.yaml`;
- `api/demo-cafe.openapi.yaml`;
- `services/avito-kitchen/migrations/`.
