# Project Development Guidelines

Welcome to the project. Follow these general guidelines when developing, testing, and reviewing code.

## Code Standards
- Keep functions small, focused, and single-purpose.
- Prefer clear, descriptive identifiers over terse abbreviations.
- Document exported packages, types, and functions with standard Godoc comments.

## Error Handling
- Never ignore errors; always handle or wrap them with context.
- Use `fmt.Errorf("...: %w", err)` to preserve error chains.
- Return errors to callers rather than logging and continuing unless recovery is intentional.

## Testing
- Write table-driven unit tests for business logic.
- Ensure all test suites pass with `go test -v ./...`.
- Keep tests isolated and deterministic; avoid dependence on external networks.
