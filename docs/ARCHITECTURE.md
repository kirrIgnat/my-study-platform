# Architecture

The repository follows a small Go application layout:

- `cmd/server` — executable entry point.
- `internal/app` — application code that is not intended to be imported by other repositories.
- `web/templates` — server-rendered HTML templates.
- `web/static` — browser assets.
- `deploy` — production deployment examples and PostgreSQL schema.

## Backend boundaries

HTTP page handlers are split from JSON API handlers. Database operations are grouped by domain (`courses`, `schedules`, `reschedules`, `lessons`, admin/notifications) instead of being kept in a single store file. Authentication is split into password primitives, user/session operations, and administrative operations. Database connection/query helpers are separate from migrations.

The application intentionally remains one internal Go package for now: the domains share several models and transactions, and splitting them into many packages would add interfaces and dependency plumbing without improving the current codebase. This keeps the project easy to navigate while avoiding artificial package boundaries.
