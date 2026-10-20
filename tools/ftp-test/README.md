# Local FTP/FTPS test server

This Compose setup runs plain FTP, explicit FTPS, and implicit FTPS against a shared test directory. It is intended for local development only; the credentials are fixed test credentials and the TLS certificate is self-signed.

Start the servers from this directory:

```powershell
docker compose up --build -d
```

Use these values in QuickFTP:

| Mode | Host | Port | Protocol | Username | Password | Remote root |
| --- | --- | ---: | --- | --- | --- | --- |
| FTP | `127.0.0.1` | `2120` | Standard FTP | `quickftp` | `quickftp-test-only` | `/site/wwwroot` |
| Explicit FTPS | `127.0.0.1` | `2121` | FTPS | `quickftp` | `quickftp-test-only` | `/site/wwwroot` |
| Implicit FTPS | `127.0.0.1` | `990` | FTPS | `quickftp` | `quickftp-test-only` | `/site/wwwroot` |

Turn off **Verify TLS Certificate** for the two FTPS entries because the server creates a self-signed certificate. Port `990` selects implicit TLS; port `2121` selects explicit TLS in QuickFTP.

The FTP server writes to a Docker volume named `ftp-test_ftp-data`. To stop the servers while preserving test files, run `docker compose down`. To also delete the test files, run `docker compose down -v`.

This server is bound on all host interfaces by Docker's port publishing. Use only on a trusted development machine/network, and do not reuse these credentials on a real server.
