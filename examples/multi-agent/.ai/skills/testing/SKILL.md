---
name: testing
description: Automated testing procedures, fixtures, and execution for the Order Management Service
---
# Testing Skill

Guidelines for running and authoring automated tests in the Order Management Service.

## Test Commands
- **Unit Tests:** `go test -v ./internal/...`
- **Race Detection:** `go test -race -v ./internal/...`
- **Integration Tests:** `go test -tags=integration -v ./internal/adapters/postgres/...`

## Test Structure
- Use table-driven test cases with subtests (`t.Run(tc.name, func(t *testing.T) { ... })`).
- Mock outbound ports with test doubles in `internal/ports/mocks/`.
- Validate error classification using `errors.Is` against domain sentinel errors.
