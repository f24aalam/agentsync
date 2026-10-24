---
name: code-review
description: Code review guidelines and standards for the Order Management Service
---
# Code Review Skill

Use this skill when reviewing pull requests for the Order Management Service.

## Checklist
1. **Architecture Integrity:**
   - Are domain entities kept free of database and HTTP dependencies?
   - Are repositories implementing contracts defined in `ports/`?
2. **Error Handling & Observability:**
   - Are errors properly wrapped with `%w`?
   - Are structured logs emitted with relevant tracing fields (`order_id`, `customer_id`)?
3. **Concurrency & Resources:**
   - Are database transactions rolled back or committed with defer guards?
   - Are goroutine lifecycles bounded by `context.Context`?
4. **Testing:**
   - Do tests cover both happy paths and error conditions?
   - Are tests deterministic and free of race conditions?
