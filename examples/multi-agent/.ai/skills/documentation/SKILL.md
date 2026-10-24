---
name: documentation
description: Documentation conventions, Godoc standards, and API specification guides
---
# Documentation Skill

Standards for documenting packages, types, methods, and HTTP endpoints in the service.

## Godoc Guidelines
- Every exported identifier must have a comment beginning with its name.
- Package comments in `doc.go` must explain the role and responsibility of the package.
- Document any preconditions, concurrency guarantees, and returned error conditions.

## API Documentation
- HTTP handlers must include Swagger/OpenAPI annotations describing request bodies, path parameters, and responses.
- Keep `docs/api/openapi.yaml` in sync with route modifications.
