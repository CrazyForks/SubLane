# SubLane

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

A modular Go monolith with chi, sqlc-generated database queries, SQLite, and an embedded static frontend. The frontend uses the user-selected Shadcn Admin, React, TypeScript, Vite, TanStack Router, and TanStack Query. A provider adapter reuses CLIProxyAPI v7.3.7’s built-in executors while SubLane owns OAuth, encrypted storage, refresh, account selection, and the public HTTP boundary.

## Users

Platform operators, workspace owners, workspace administrators, and members. One login identity may belong to several workspaces with different roles. Workspace administrators manage their own subscription accounts, pools, members, and allowances; members use personal gateway keys.

## Product Purpose

Build a minimal subscription gateway that can host several isolated workspaces in one Go process and SQLite database. Keep a single-workspace self-hosted installation simple while supporting a hosted multi-tenant deployment. Codex is the only active subscription provider. Claude and Antigravity integrations are retained but temporarily disabled. Official API key upstreams remain later work.

## Capabilities and Constraints

The foundation and local account access are implemented: one-time administrator setup, member creation and enable/disable controls, shared login/logout, per-user sessions, personal API key management and gateway authentication, workspace-scoped management APIs, role-aware navigation and homepages, startup configuration, SQLite migrations, health checks, themes, localization, and build tooling. Codex browser OAuth and credential JSON import, encrypted subscription credentials, and HTTP/SSE/WebSocket forwarding are active. Claude and Antigravity integrations have synthetic upstream coverage but are disabled in the application runtime; stored accounts remain visible and cannot serve requests. Native Claude Messages and Gemini generation JSON/SSE endpoints share existing gateway permissions, admission and history; actual subscription authorization and desktop acceptance remain unverified; Codex subscription quota snapshots and freshness-aware exhausted-account admission are available; Claude and Antigravity quota readers are not implemented; account pools, member grants, immutable key/pool bindings, and pool-scoped routing are implemented; new imports remain unassigned until explicitly added to a pool, and former default pools retain their IDs as ordinary editable pools; per-account concurrency, durable cooldown/recovery, independent sessionless selection, and bounded request metadata with personal history and an workspace-administrator view, scoped diagnostic filters, request IDs and first-output latency are implemented; password changes, administrator member resets and local administrator recovery, configurable member RPM/concurrency limits (unlimited by default), and durable personal/workspace usage summaries with hourly activity heatmaps are implemented; key expiry/rename/pause controls and owner-only copying of newly created encrypted keys, exact pool model allowlists and a bounded administrator management audit are implemented; persistent account model discovery, automatic pool catalogs and model-aware account selection are implemented, with searchable models in CC Switch import; platform-owner system settings support a persistent instance-wide IANA time zone, manual Codex client versions and persistent, automatic official stable-release checks; CLI and platform-owner web backup/verification and isolated-directory restore are implemented; version-tag release automation, Linux amd64/arm64 packaging and container runtime checks are configured; direct member grants control account-pool access, with optional advanced pool-bound allocation schemes supporting mutually exclusive token, internal USD amount, or share allowances derived from a configured token or internal USD budget, with per-scheme daily/monthly reset dates and times and configurable multi-condition time-window USD limits with shared defaults and per-member overrides, plus pool-model-bound automatic price snapshots, bounded provisional exposure for unsettled requests, next-cycle configuration changes, and scheme-bound keys; monetary billing is not implemented. See [allocation schemes](https://sublane.dev/docs/allocations). Do not populate the product with fabricated accounts, usage, or performance claims.

An allowance revision uses current pool-model prices when available. If a supported model has no current catalog price, the revision reuses its own previously saved price when one exists; otherwise that model cannot pass allowance admission. Price-coverage warnings are derived from the current catalog and each revision's saved rates when read, rather than persisted with the revision. A restored catalog price clears its warning; a model absent from the saved rates remains blocked until a new priced revision takes effect. Saving fails only when no usable model price remains. For an active time-window allowance, configuration edits take effect at the next boundary of its longest current window, potentially up to 365 days away; the edit form shows that effective date before saving. Pausing or enabling access remains immediate.

English is the default interface language. Support English and Simplified Chinese and persist the user's language preference. PRODUCT.md and the primary project documentation are written in English.

The project uses the GNU Affero General Public License v3.0 (AGPL-3.0-only). Retain third-party license and attribution notices separately.

## Brand Commitments

The name is SubLane. Black, white, and neutral grays form the primary palette. Success, warning, error, and information states may use semantic colors. Preserve the previously agreed palette and Shadcn Admin components; keep the interface minimal and restrained.

## Product Principles

- Keep deployment simple and avoid unnecessary runtime dependencies.
- Complete real connection workflows before broadening the feature set.
- Separate upstream credentials from member gateway keys.
- Validate resource budgets with measurements rather than presenting targets as guarantees.
