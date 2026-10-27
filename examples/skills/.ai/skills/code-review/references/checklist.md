# Pull Request Review Checklist

Consult this checklist before approving any pull request.

## Clean Architecture & Design
- [ ] Logic is placed in the appropriate architectural layer.
- [ ] Public interfaces are clean and minimal.
- [ ] Dependencies flow toward the domain, never outward.

## Robustness & Error Handling
- [ ] All returned errors are checked and wrapped with `%w`.
- [ ] Defers are used for resource cleanup (file handles, database locks).
- [ ] Context cancellation is respected in blocking loops.

## Testing & Verification
- [ ] Table-driven tests cover both success and failure branches.
- [ ] Tests pass without race conditions (`go test -race ./...`).
- [ ] No extraneous logs, temporary files, or secrets are committed.
