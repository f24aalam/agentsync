# Code Review Checklist

Use this checklist during every code review.

## Architecture and Design
- [ ] Code follows existing project layering and package structure.
- [ ] Interfaces are defined where needed, not prematurely abstracted.
- [ ] Concurrency: goroutines have clear shutdown signals and bounded lifetimes.

## Correctness and Robustness
- [ ] Input arguments are validated at public boundaries.
- [ ] Boundary and edge cases (empty strings, nil pointers, zero values) are handled.
- [ ] Errors are wrapped with actionable context (`%w`).

## Testing and Quality
- [ ] New logic is covered by unit or integration tests.
- [ ] Tests run cleanly and deterministically (`go test ./...`).
- [ ] No temporary debugging code or unneeded dependencies are committed.
