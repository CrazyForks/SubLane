# Agent instructions

## Context

Read `PRODUCT.md` for product scope, `DESIGN.md` for interface rules, and [the architecture guide](https://sublane.dev/docs/architecture) for implementation boundaries before making significant changes. Local account access and Codex OAuth/import, encrypted credentials, and gateway forwarding are active. Claude and Antigravity integrations are retained but temporarily disabled. Live subscription and desktop compatibility require separate evidence; see [the Codex guide](https://sublane.dev/docs/codex).

Use English for code comments, `PRODUCT.md`, and primary developer documentation. Keep the English and Simplified Chinese UI dictionaries complete. English is the default interface language.

## Work style

Usage, operations, and architecture guides are maintained in [the documentation repository](https://github.com/murongg/sublane-website). Update guides there instead of duplicating them here; see [the documentation index](docs/README.md) for references retained locally.

- Make the smallest complete change that meets the request. Prefer existing mechanisms to new dependencies or abstractions.
- Keep unrelated edits out of a task. Do not introduce speculative provider frameworks, shared packages, or runtime services.
- Explain assumptions and verification results concisely. Never report an untested build, endpoint, or client integration as working.
- Product plans and specifications in `docs/plans/` and `docs/specs/` are local working files; do not commit them unless explicitly requested.
- Do not create commits, publish a repository, or deploy without a user request.

## Releases

- Publishing a version is not complete until `CHANGELOG.md` includes that tag. The tag workflow generates GitHub Release notes but does not update the repository file.
- After the release workflow succeeds, use `git-cliff 2.14.2` to run `make changelog-check` and `make changelog`. Review the new version section and comparison link, then commit the changelog and merge it into `main`.

## Boundaries

- `cmd/sublane` owns startup, process lifecycle, and dependency wiring.
- `internal/config` owns environment parsing and validation.
- `internal/backup` owns bounded backup archives, integrity/key validation and atomic publication into new paths. `internal/storage` owns read-only SQLite snapshots. Never overwrite a live data directory or export backups to unauthenticated/member routes.
- `internal/versions` owns persisted Codex version policy, bounded official release checks and their lifecycle. Publish versions only after persistence; manual pins take priority. It must not depend on SDK types or install executables.
- `internal/auth` owns global login identities, credentials, first-run initialization, and persisted sessions. `internal/tenants` owns workspace lifecycle, membership roles, and member status within a workspace.
- `internal/groups` owns tenant-scoped account pools, direct member grants, available-pool discovery, and pool-scoped readiness. New pools grant no member access until explicitly configured.
- `internal/allocations` owns exclusive-pool schemes, configuration revisions, and per-scheme accounting; it does not own provider network IO, browser authorization, or billing.
- `internal/apikey` owns personal gateway key generation, immutable group binding, hashed authentication, encrypted recoverable values, owner-only audited disclosure, ownership, expiry, enablement, revocation, and bearer authentication. API keys must never authenticate browser management sessions. Never return full keys in metadata or cache disclosed values in the frontend. Revocation removes encrypted values; legacy hash-only keys remain usable without disclosure.
- `internal/audit` owns bounded management metadata and actor context. Successful mutation events must share the domain transaction. Never audit credentials, request bodies, raw URLs or error contents; automatic refresh is not a manual authorization event.
- `internal/storage` owns SQLite initialization, query SQL, and sqlc-generated database access under `internal/storage/db`. Keep one-to-one metadata on its owner: encrypted key values on `api_keys`, member limits on `memberships`, and instance-level collection timestamps in `settings`. Keep runtime counters, relations and historical records separate when their lifecycle or cardinality calls for it.
- `internal/server` owns chi routing and HTTP handling; it must not silently serve HTML for API errors.
- `web` is a client-rendered React app and an embedded Go asset package. It must not require a Node.js server in production.
- `internal/upstream` owns provider protocols and the pinned public CLIProxyAPI SDK executors. Its SDK service is a private executor registry with an empty credential store, no-op watcher, blocked loopback HTTP routes, and automatic refresh disabled. Never register live credentials in the SDK manager or pass refresh tokens to execution auth. Antigravity may receive a refresh token only during the explicit SDK refresh call owned by `accounts.Prepare`; persist the returned snapshot before model execution. Do not import upstream `internal` packages.
- `internal/accounts` owns subscription metadata, normalized persisted model snapshots, lifecycle revisions and serialized credential changes, `internal/vault` owns encryption, `internal/oauth` owns session-bound OAuth attempts (PKCE where supported), and `internal/gateway` owns account affinity, account leases, durable cooldowns, bounded request metadata, request admission, and quota snapshot caching. Membership, policy, and storage code must not depend on SDK types.
- Avoid adding packages solely for hypothetical reuse. Keep related code together and move it only when ownership or reuse justifies a boundary.

## Backend

- Use standard Go conventions and explicit dependency injection at test seams.
- Register workspace management routes on the workspace-administrator-protected chi router. Platform settings and instance backups require the platform owner. Keep handlers compatible with `net/http`; domain services must not depend on chi.
- Use chi `Route` and method-specific registrations for HTTP resources; do not dispatch methods or gateway paths manually inside handlers. Apply session, role, and gateway authentication with router-level `Use` so 404/405 responses remain protected. Reserve `With` for endpoint-specific checks such as login origin validation and throttling.
- Write application queries in `internal/storage/queries/` and run `make generate`. Never hand-edit `internal/storage/db/`; keep generated code with its SQL changes. Migration bootstrap SQL remains in storage.
- Keep transaction ownership in domain services and use `queries.WithTx(tx)` for every query inside a transaction. Keep database row types separate from public API responses.
- Before the first stable release, SQLite uses one initialization schema. After release, preserve deployed schemas with ordered additive migrations.
- Do not hold a database transaction open during network IO or model generation. Persist rotated credentials before returning them to callers.
- Reauthorization must preserve upstream identity. Recheck gateway keys on every WebSocket turn, and never move an existing conversation to another account after disablement, deletion, or removal from its pool. Gateway candidates and models must remain inside the key’s group, and every request/WS turn must recheck current group access and model allowlists, including local WebSocket prewarm. Key expiry and enablement are checked again on every turn.
- Persist model snapshots before publishing them. Discovery must respect account lifecycle revisions, remain bounded and join shutdown. Unknown or over-age catalogs never mean unrestricted model support. Derived group catalogs and inference must use the same account capabilities and current group policy; recheck membership and policy after discovery IO. Never replace administrator allowlists during synchronization. Public catalogs use native IDs; resolve accounts by capability and recheck legacy provider-scoped rules against the selected account. Native conversations use one cross-provider affinity binding and must never move silently.
- Persist a successful quota snapshot before publishing it. Cached reads must check account enablement; stale values retain their original observation time. Shared background refreshes use the process context and must be joined before closing SQLite.
- Keep model-request leases until body closure and release exactly once on cancellation. Preserve sticky accounts under saturation/cooldown; sessionless requests must not create affinity. Never store prompt/response/error bodies or credentials in request history. Personal history and usage summaries must derive identity from the enabled session and filter in SQL by workspace. Hide subscription account identities from personal history; full history and workspace summaries remain workspace-administrator-only. Member rate/concurrency admission is shared across keys and transports within one workspace; release member leases with the model observation. Aggregate and history writes commit together, with bounded model labels and explicit token coverage. Password changes must atomically revoke browser sessions, and session creation must recheck the verified password hash.
- Keep OAuth states, request bodies, stream events, WebSocket history, and concurrent operations bounded. Never read local Codex credentials automatically or use real credentials in tests.
- Preserve cancellation and graceful shutdown. Future model streaming routes need explicit timeout and resource policies rather than blanket response buffering.
- Do not add authentication bypasses or expose management endpoints as member APIs.
- Default to loopback listening. Browser management derives workspace role from active membership; gateway requests derive workspace identity from the authenticated key, never the browser selection header. Members must never inherit administrator API access; enforce roles on both direct routes and backend requests. First-run setup uses a username and password and atomically creates the initial workspace owner; preserve its single-administrator guard. See [the authentication guide](https://sublane.dev/docs/authentication) for the current authentication contract.

## Frontend

- Do not use the JavaScript `void` operator, including to discard promises. Return or await promises where appropriate and handle failures explicitly. TypeScript `void` return types are allowed.
- Use PascalCase component filenames and lowercase domain folders. Follow TanStack Router conventions for routes.
- Reuse the adapted Shadcn Admin components under `web/src/components/ui/`.
- Use TanStack Query for remote state and validate API responses at the boundary.
- Keep user-facing text in `web/src/locales/en.ts` and `zh.ts`, including accessibility labels and error messages.
- Keep colors in semantic CSS tokens. Primary actions stay black/white; use status colors only for status or relevant feedback.
- Preserve keyboard access, visible focus, reduced-motion behavior, responsive navigation, and both themes.
- Render honest loading, failure, and empty states. Never add fake account or usage data to product screens.

## Tests and verification

- Behavior-changing code requires a failing test before implementation, then a passing test. Text-only, documentation, and nonfunctional configuration edits do not require unit tests.
- Use only synthetic fixtures, fake upstreams, and temporary SQLite databases. Never read actual credentials or use personal, customer, production, or identifiable data in tests.
- Keep meaningful tests close to the code they cover. Do not add tests that merely duplicate implementation details or assert deleted text is gone.
- Run `make check` before completing a broad implementation. For a small change, start with affected tests and run the relevant build or checks.
- For UI changes, verify the affected flows in the browser at desktop and narrow widths, in both themes, and with English and Chinese where relevant.
- For deployment changes, validate Compose and build the image when Docker is available. State clearly when the Docker daemon is unavailable.
- Inspect critical logic after verification. Add concise comments where ordering, invariants, compatibility assumptions, or edge cases would otherwise be easy to break.

## Credentials and licensing

- Never commit databases, credentials, `.env` files, auth caches, or upstream request bodies.
- Do not log tokens or model prompt/response content by default.
- Preserve AGPL-3.0-only licensing and all third-party notices. Adapted Shadcn Admin components retain their MIT attribution.
- Do not invent repository URLs, maintainer contact addresses, support promises, or performance benchmarks.
