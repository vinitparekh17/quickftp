# QuickFTP Agent Guide

## What this project is

QuickFTP is a Go deployment tool for publishing Angular and .NET build artifacts to FTP or explicit FTPS hosts. The desktop shell is built on Wails v2 with a Svelte + TypeScript frontend.

The module is `quickftp` and targets Go 1.26.5. SQLite uses the pure-Go `modernc.org/sqlite` driver. On Windows, saved connection passwords use Windows Credential Manager; non-Windows builds use an in-memory credential store for development and tests.

## What is implemented

- `internal/storage`: SQLite open/migration setup, AppData paths, connection and project persistence, deployment journal records, pending-deployment queries, list/delete CRUD operations.
- `internal/creds`: credential-store interface with Windows Credential Manager and non-Windows in-memory implementations.
- `internal/projecthandler`: Angular and .NET build runners, output discovery, artifact include/exclude glob rules, and streamed stdout/stderr callbacks.
- `internal/transfer`: FTP and explicit FTPS client, recursive listing, binary file operations, journaled deployment, local per-deployment backups, checksum verification, uncertain-operation reconciliation, stale-file deletion, and explicit rollback.
- `internal/deploy`: `Service` connects saved profiles, credentials, handlers, and the transfer engine. Its `DeployProject` and `Rollback` methods are the main backend entry points.
- `app.go` & `main.go`: Wails v2 runtime shell binding Go backend methods to the desktop UI, providing DTO conversions, background deployment lifecycle, and real-time event streaming (`deploy:started`, `deploy:phase`, `deploy:log`, `deploy:complete`, `deploy:error`).
- `frontend/`: Complete Svelte + TypeScript UI featuring:
  - Connection management (FTP & explicit FTPS with credential store integration and live connection test).
  - Project configuration (Angular and .NET with build commands and artifact include/exclude glob rules).
  - Deployment cockpit (live pipeline progress bar, scrollable real-time build terminal, IIS `app_offline.htm` options, and failure rollback).
  - Startup recovery banner (detects interrupted deployments and provides 1-click rollback).
  - History & journal viewer (audit log of deployments and per-file transfer records).
- `cmd/ftp-spike`: opt-in live protocol probe for login, listing, upload, download, rename, and delete. It requires `QUICKFTP_FTP_HOST`, `QUICKFTP_FTP_USERNAME`, and `QUICKFTP_FTP_PASSWORD`; optional variables are `QUICKFTP_FTP_PORT`, `QUICKFTP_FTP_PROTOCOL` (`ftp` or `ftps_explicit`), `QUICKFTP_FTP_TLS_VERIFY`, and `QUICKFTP_FTP_REMOTE_ROOT`.

Deployment order is deliberate: snapshot and verify the current remote files, upload to temporary names, verify their SHA-256, replace targets, then remove stale files. Backups are written below the configured backup root in a `deployment-<id>` directory. IIS `app_offline.htm` handling is opt-in through `deploy.Options`; it applies to ASP.NET Core hosted on IIS.

## Roadmap & Execution Status

The application architecture spans four core milestones. The desktop shell and complete UI (Phases 3 & 4) are implemented and functional. Hardening of live FTP protocols, cancellation, and versioned schema migrations (Phases 1 & 2) remain active focus areas for backend maturation.

| Phase | Milestone | Status | Primary Goal |
|-------|-----------|--------|--------------|
| **Phase 1** | Live Protocol & Cancellation Hardening | *Pending* | Prove real FTP/FTPS behavior, enforce mid-transfer cancellation, and verify IIS lock release. |
| **Phase 2** | Storage & Recovery Maturation | *Pending* | Replace static `CREATE TABLE` setup with versioned SQLite migrations and finalize the crash-recovery contract. |
| **Phase 3** | Wails Shell & Event Bridge | **Done** | Initialized Wails v2 runtime, exposed Go backend bindings, and streamed build/transfer progress events. |
| **Phase 4** | Desktop Frontend Implementation | **Done** | Built TypeScript screens for Connections, Projects, Deployment execution, Startup Recovery, and History. |

### Phase 1 — Live Protocol & Cancellation Hardening *(In Progress / Next)*

- Stand up integration tests against a controlled FTP and explicit FTPS server, covering TLS verification, protected data transfers, passive mode, server-specific rename behavior, connection loss, and permissions.
- Review cancellation and timeout behavior during active FTP data transfers. The FTP library's operations are synchronous; passing a context to dial does not make an in-progress transfer fully cancellable. Investigate wrapping data-channel reads so cancellation closes the underlying connection.
- Test `IISOfflinePage` against the intended IIS hosting mode and confirm whether a configured ASP.NET Core deployment needs it.
- Validate `cmd/ftp-spike` against real servers and document any server-specific quirks discovered.

### Phase 2 — Storage & Recovery Maturation *(Pending)*

- Replace the static `CREATE TABLE IF NOT EXISTS` schema setup with a versioned SQLite migration framework applied at database open.
- Wire startup recovery UX around `DB.PendingDeployments()` and `Service.Rollback()`. Define the contract: rollback is explicit — there is no automatic resume or rollback policy — but the application must surface incomplete deployments on launch.
- Ensure migration ordering is safe for schema changes that existing data must survive.

### Phase 3 — Wails Shell & Event Bridge *(Completed)*

- Initialized Wails v2 runtime (`app.go`, `main.go`, `wails.json`).
- Exposed connection and project CRUD through Wails bindings, returning typed DTOs.
- Progress-event bridge streaming build output (`deploy:log`) and transfer lifecycle events (`deploy:started`, `deploy:phase`, `deploy:complete`, `deploy:error`) in real time.

### Phase 4 — Desktop Frontend Implementation *(Completed)*

- Svelte + TypeScript screens built and verified:
  - Connection management with live testing and Windows Credential Manager integration.
  - Project configuration for Angular and .NET with build commands and artifact glob rules.
  - Deployment cockpit with pipeline progress bar, scrollable real-time build terminal, and failure rollback.
  - Startup recovery banner alerting on launch if interrupted deployments are detected.
  - History viewer with audit trail and per-file transfer journals.

## Working conventions

- Keep project-specific build/artifact logic in `internal/projecthandler`; keep remote operations and deployment ordering in `internal/transfer`.
- Do not retry a failed upload, rename, or delete blindly. Inspect remote state first because the server may have completed an operation before its reply was lost.
- Do not delete or replace a remote file until its backup is present and verified. Keep each deployment's backup path isolated.
- Never log passwords or credential blobs. The FTP spike performs real writes and deletes a uniquely named probe; a failed run may leave a probe file on the server.
- `cmd/spike` is an older storage/credential smoke program. It writes a sample connection and credential, so do not run it casually against a user's normal profile.
- Prefer focused tests with temporary SQLite databases and mock `transfer.Client` implementations. Do not require a live server for unit tests.

### Security conventions

- **Zero plaintext secrets**: Never store connection passwords in SQLite or JSON files. On Windows, delegate to Windows Credential Manager (`creds.Store`); for tests, use `store_other.go` in-memory.
- **Redacted DTOs**: Never return passwords from backend DTOs to the frontend or over Wails events.
- **Path traversal prevention**: All local and remote paths must be normalized and checked with `safeRelative()` to prevent `..` and drive escapes.
- **Transport security**: Default to explicit FTPS with TLS 1.2+ and certificate verification. Only bypass certificate validation if explicitly configured by the user.

## Useful commands

### Backend & Tests
```powershell
go test ./...
go vet ./...
go build ./...
go run ./cmd/ftp-spike
```

### Desktop & Frontend
```powershell
# Run desktop app in live-reloading dev mode
wails dev

# Build production Windows executable (build/bin/QuickFTP.exe)
wails build

# Typecheck and build frontend standalone
cd frontend
npx tsc --noEmit
npm run build
```

If the default Go build cache is inaccessible in the environment, set `GOCACHE` to a writable temporary or workspace directory for the command, then remove that generated cache afterward.
