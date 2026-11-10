# QuickFTP

QuickFTP is a desktop deployment application designed for safely publishing Angular and .NET build artifacts to remote FTP and explicit FTPS (FTP over TLS) hosts. Built with **Go**, **Wails v2**, and **Svelte + TypeScript**, it emphasizes safe, journaled, rollback-capable deployments.

---

## Key Features

- **Safe, Multi-Stage Deployments**:
  1. **Planning**: Discovers build outputs and precomputes local SHA-256 digests.
  2. **Backup**: Downloads and verifies an isolated remote backup snapshot before touching any live files.
  3. **IIS Lock Handling (Opt-in)**: Activates `app_offline.htm` and waits for ASP.NET Core app pools to release DLL locks.
  4. **Staged Upload & Promotion**: Uploads files to temporary names (`.quickftp-<id>.tmp`), checks remote checksums, and atomically replaces targets.
  5. **Stale Cleanup**: Removes deleted files only after new files are confirmed live and backups exist.
- **Explicit 1-Click Rollback**: Restore the last verified remote snapshot immediately if a deployment fails or is interrupted.
- **Startup Crash Recovery**: Detects interrupted deployments on launch and prompts for safe rollback.
- **Secure Secret Storage**: Uses Windows Credential Manager (`wincred`) on Windows to store FTP passwords securely outside SQLite.
- **Live Deployment Cockpit**: Real-time progress bar, multi-phase tracker, and scrollable build stdout/stderr terminal.
- **Project Profiles**: Supports Angular (`npm run build`, output globbing) and .NET (`dotnet publish`, output globbing).

---

## Security Architecture

QuickFTP is engineered with a defense-in-depth approach to secret handling, network transport, file integrity, and path security:

### 1. Zero Plaintext Secrets in Storage
- **Isolated Credential Store**: Passwords are **never stored in SQLite**. The database holds only profile metadata and a cryptographic target key reference (`QuickFTP/<ConnectionName>`).
- **Windows Credential Manager**: On Windows, secrets are stored in the encrypted OS-level Credential Vault via `github.com/danieljoos/wincred` with machine-level security.
- **Redacted Frontend Bridge**: When connection profiles are fetched by the UI, passwords are systematically stripped from DTOs. Passwords flow only from user input to the credential store when explicitly saving or testing.
- **Log Sanitization**: Passwords, credential blobs, and authentication tokens are strictly forbidden from appearing in logs, journals, console output, or Wails event streams.

### 2. Transport Security (FTPS Explicit / TLS 1.2+)
- **Explicit FTPS (FTP over TLS)**: Supports RFC 4217 explicit TLS upgrades.
- **Minimum TLS 1.2**: Hardened TLS configuration requiring a minimum of TLS 1.2 (`tls.VersionTLS12`).
- **Data Channel Encryption**: Protected data channels for directory listings and file payloads with TLS session caching (`tls.NewLRUClientSessionCache`).
- **Strict Certificate Verification**: TLS validation is enforced by default. Insecure skip verification is an explicit, per-connection opt-in toggle intended solely for private testing with self-signed certificates.

### 3. Path Traversal & Escape Prevention
- **Sanitized Relative Paths**: Every artifact path and remote path is passed through `safeRelative()` which normalizes slashes, cleans relative tokens, and rejects path traversal attacks (`..`, leading slashes, backslashes, drive letters, and volume paths).
- **Directory Jails**: Local build discovery strictly verifies that output files cannot escape the configured project root before collecting artifacts.
- **Isolated Per-Deployment Backups**: Remote backups are saved under isolated subdirectories (`deployment-<id>`) with restricted permissions (`0700`), preventing cross-deployment pollution or overwriting.

### 4. Cryptographic Integrity Verification (SHA-256)
- **Pre-Upload Fingerprinting**: Local artifacts are hashed with SHA-256 before any network operations take place.
- **Pre-Promotion Staging Verification**: Remote files are uploaded to uniquely tagged staging names (`.quickftp-<id>.tmp`) and verified by comparing downloaded bytes and hashes against the local SHA-256 digest before promoting to production.
- **Backup Verification**: Remote files are verified after download by matching size and hash against the remote listing before any replacement or deletion is executed.

---

## Prerequisites

Before running or building QuickFTP, ensure you have the following installed:

| Tool | Minimum Version | Note |
|------|-----------------|------|
| **Go** | 1.22+ (tested on Go 1.26) | [golang.org/dl](https://go.dev/dl/) |
| **Node.js & npm** | Node 18+ (tested on Node 24) | [nodejs.org](https://nodejs.org/) |
| **Wails CLI v2** | `v2.16.0` | Install with `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| **Microsoft Edge WebView2** | Installed by default on Windows 10/11 | [developer.microsoft.com/microsoft-edge/webview2](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) |

Check your environment health at any time:
```powershell
wails doctor
```

---

## Quick Start & Development

### 1. Clone & Install Dependencies

```powershell
# Clone the repository
git clone https://github.com/your-username/quickftp.git
cd quickftp

# Install Go dependencies
go mod download

# Install frontend dependencies
cd frontend
npm install
cd ..
```

### 2. Run in Development Mode (Live Reloading)

Launch both the Go backend and Svelte frontend with hot-reload enabled:

```powershell
wails dev
```

Any changes to Go files will trigger a background rebuild, and frontend changes update instantly via Vite HMR.

---

## Building for Production

Compile a standalone, optimized Windows desktop executable:

```powershell
wails build
```

The resulting executable will be generated at:
```
build/bin/QuickFTP.exe
```

---

## User Guide

### 1. Managing Connections
Navigate to the **Connections** tab to register your FTP / FTPS servers:
- **Protocol**: Choose between **Explicit FTPS (Recommended)** or **Standard FTP**.
- **Host & Port**: Server hostname/IP and control port (default: 21).
- **Credentials**: Username and password. Passwords are saved directly to the OS Credential Manager and are never displayed or stored in plain text.
- **Remote Root**: Directory where artifacts should be placed (e.g. `/` or `/site/wwwroot`).
- **TLS Verification**: Enabled by default; uncheck only if connecting to an internal server using self-signed certificates.
- **Test Connection**: Click **Test** to verify connection and login credentials prior to saving.

### 2. Configuring Projects
Navigate to the **Projects** tab:
- **Type**: Select **Angular** or **.NET**.
- **Local Path**: Absolute path to your local project repository.
- **Build Command**: The build script executed before artifact collection (e.g. `npm run build` or `dotnet publish -c Release`). Leave blank if you build manually.
- **Output Rel Path**: Path relative to the project root containing the compiled distribution files (e.g. `dist/my-app`). For .NET, use `auto` with `dotnet publish -c Release`: QuickFTP detects the declared target framework and can select a single web application one folder below a solution root. Conventional framework paths are adjusted for that command. Custom commands, inherited/conditional target frameworks, and ambiguous or multi-target projects need explicit configuration. Detection is logged and does not rewrite saved profiles.
- **Artifact Rules (Optional JSON)**: Define include and exclude glob patterns:
  ```json
  {
    "include": [],
    "exclude": ["*.map", "appsettings.Development.json"]
  }
  ```

Use **Projects → project → Deployments → run** for saved build/transfer logs and that run’s file journal. Deployment records are created before building, so build failures also retain a run ID and diagnostics. Historical text logs are paginated and can be cleared per run or all at once; clearing text logs does not remove deployment records, rollback journals, or backups.

Manual file operations follow **Connections → connection → Transfers → batch → files**. Activity is a shortcut to this connection-scoped history and the application log. **Clear finished transfer history** removes only terminal entries for the selected connection (or all connections from the overview), preserving queued, running, and conflict-waiting work. Older records without batch IDs remain in recovered groups. File details and journals load on demand in pages of 50.

For a first .NET deployment, leave include patterns empty to publish the complete output directory, including dependencies, `.deps.json`, `.runtimeconfig.json`, native runtime assets, and generated `web.config`. Include/exclude rules are optional restrictions, not a required list of DLLs.

Directory uploads validate files first, persist queue admission once, emit one queue-update event, and use a bounded number of workers. Manual upload verification uses FTP SIZE when supported, with listing fallback; deployment SHA-256 verification remains unchanged.

### 3. Running a Deployment
1. Open **Projects**, choose a project, and click **Deploy** (or **New Deployment** in that project’s history).
2. Select your configured **Project** and **Target Connection**.
3. *(Optional)* Check **Enable IIS app_offline.htm handling** if deploying an ASP.NET Core application to IIS to release file locks.
4. Click **Start Safe Deployment**.
5. Watch live progress in the **Execution Pipeline** and the streaming **Output Console**.
6. Eligible failed deployments offer **Rollback** when a verified snapshot is available. Build-only failures have no remote changes to restore.

Transient staging upload failures (including FTP 426) are checked against the remote SHA-256 before recovery. A complete staging file is accepted; a confirmed missing or mismatched staging file permits up to three upload attempts. If remote verification cannot establish the outcome, deployment stops and reports both errors. Live targets are promoted only after staging verification.

### 4. Viewing History & Rollback
Each project’s **Deployments** view contains its execution history:
- Click any row to expand the journal showing individual files backed up and uploaded.
- Trigger explicit rollbacks for any failed or interrupted deployments.

---

## Command Reference

### Backend & Unit Tests
```powershell
# Run all Go tests
go test ./...

# Run tests for specific packages
go test ./internal/storage/...
go test ./internal/transfer/...
go test ./internal/projecthandler/...

# Run Go static analysis
go vet ./...

# Run the live FTP protocol probe (requires environment variables set)
go run ./cmd/ftp-spike
```

### Frontend Standalone
```powershell
cd frontend

# Run frontend in browser dev server (uses mock data)
npm run dev

# Typecheck TypeScript definitions
npx tsc --noEmit

# Compile production frontend assets
npm run build
```

### Desktop Shell (Wails)
```powershell
# Check toolchain prerequisites
wails doctor

# Run desktop app with live reload
wails dev

# Build standalone Windows .exe
wails build

# Build with debug devtools enabled
wails build -debug
```

---

## Architecture Overview

```
quickftp/
├── app.go                      # Wails Go application bridge & DTO models
├── main.go                     # Desktop window bootstrap & embedded assets
├── wails.json                  # Wails configuration
├── cmd/
│   ├── ftp-spike/              # Protocol verification probe
│   └── spike/                  # Storage & credential smoke test
├── internal/
│   ├── creds/                  # Windows Credential Manager integration
│   ├── deploy/                 # Deployment service coordinator
│   ├── projecthandler/         # Angular and .NET build runners & artifact discovery
│   ├── storage/                # SQLite engine, CRUD, and deployment journaling
│   └── transfer/               # Safe FTP/FTPS engine, staging, checksums, and rollback
└── frontend/                   # Svelte + TypeScript desktop interface
    ├── src/
    │   ├── App.svelte          # Main shell & tab router
    │   ├── lib/
    │   │   ├── components/     # UI screens (Connections, Projects, Deploy, History)
    │   │   ├── stores.ts       # Svelte reactive stores
    │   │   ├── types.ts        # Frontend DTO contracts
    │   │   └── wailsjs.ts      # Wails bridge & event listener wrappers
    │   └── styles/             # Desktop theme CSS
    └── wailsjs/                # Auto-generated Wails Go bindings
```

---

## License

MIT


### Export and import data

In **Settings → Export & Import Data**, choose **Export Data** to save a versioned
QuickFTP JSON backup containing all saved connections, projects (including build
commands and artifact rules), and configuration. Save settings before exporting
if you have edited preferences.

Choose **Import Data**, confirm, and select a backup to restore it. Profiles are
merged by name: existing connections and projects keep their IDs, history and
credentials; matching names are skipped. Saved configuration is replaced and
applied immediately. Invalid or unsupported backups are rejected before changes;
profile inserts are transactional. Import is blocked during an active deployment.
Files larger than 10 MiB are rejected.

Passwords and credential-store keys are never exported or imported. Re-enter
passwords for new connections, especially on another machine. Deployment history,
remote/local files and deployment backup contents are excluded. Review imported
local paths, collision policies and build commands before deploying; importing
does not execute commands or contact FTP servers. The JSON includes server names,
usernames and paths, so keep it private.
