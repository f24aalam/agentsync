# Order Management Service — Coding Style Guidelines

Follow standard Go idioms and project-wide conventions for all service components.

## Error Handling
- Use sentinel errors defined in `domain/errors.go` (e.g., `ErrOrderNotFound`, `ErrInvalidOrderState`).
- Wrap unexpected errors with context using `fmt.Errorf("...: %w", err)`.
- Use `errors.Is(err, target)` and `errors.As(err, &target)` for inspection; never compare error strings directly.

## Context Propagation
- Accept `context.Context` as the first argument in all network, storage, or blocking operations (`ctx context.Context`).
- Respect context cancellation and deadlines in database queries and external RPCs.
- Never store contexts inside struct fields.

## Concurrency
- Manage goroutine lifetimes using `sync.WaitGroup` or `errgroup.Group`.
- Ensure cancellation signals (`ctx.Done()`) terminate background worker routines cleanly.
- Never spawn unbounded goroutines per incoming request.

## Testing Standards
- Write table-driven unit tests for all usecase methods.
- Use interface fakes or mocks for repository boundaries.
- Run `go test -race ./...` before submitting changes.
