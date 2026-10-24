# Order Management Service — Architecture Guidelines

## Architecture Overview
The Order Management Service is structured using Clean Architecture principles, enforcing unidirectional dependency flow from outer infrastructure layers toward the inner domain layer.

```text
cmd/orders-api/         # Application entrypoints
internal/
├── domain/            # Entities, value objects, domain errors
│   ├── order.go
│   └── errors.go
├── ports/             # Inbound and outbound interface contracts
│   ├── services.go    # Inbound use case interfaces
│   └── repositories.go# Outbound storage/integration interfaces
├── usecases/          # Application business logic implementations
│   ├── create_order.go
│   └── cancel_order.go
├── adapters/          # Interface adapters
│   ├── http/          # REST controllers and request/response DTOs
│   └── postgres/      # Database repository implementations
└── platform/          # Logging, metrics, and configuration
```

## Architectural Rules
1. **Domain Independence:** Packages under `domain/` must have zero external dependencies and must never import framework, HTTP, or database packages.
2. **Ports and Adapters:** Use cases depend only on interface contracts defined in `ports/`. Concrete adapters (e.g., PostgreSQL repositories) implement these interfaces and are injected at application startup in `cmd/`.
3. **No Cross-Layer Leakage:** Database rows, SQL queries, or HTTP status codes must never leak into use cases or domain logic.
4. **Dependency Injection:** Constructors in each layer must accept their interface dependencies explicitly; avoid global singletons or service locators.
