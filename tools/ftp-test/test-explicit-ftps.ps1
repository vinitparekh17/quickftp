param(
    [int]$Files = 80
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$composeFile = Join-Path $PSScriptRoot 'compose.yaml'

Push-Location $repoRoot
try {
    $runningServices = docker compose -f $composeFile ps --status running --services
    if ($LASTEXITCODE -ne 0) {
        throw 'Could not inspect the FTP test containers.'
    }
    if ($runningServices -notcontains 'ftps-explicit') {
        docker compose -f $composeFile up -d ftps-explicit
        if ($LASTEXITCODE -ne 0) {
            throw 'Could not start the explicit FTPS test container.'
        }

        $deadline = (Get-Date).AddSeconds(30)
        do {
            Start-Sleep -Milliseconds 500
            $runningServices = docker compose -f $composeFile ps --status running --services
            if ($LASTEXITCODE -ne 0) {
                throw 'Could not inspect the explicit FTPS test container.'
            }
        } while ($runningServices -notcontains 'ftps-explicit' -and (Get-Date) -lt $deadline)
        if ($runningServices -notcontains 'ftps-explicit') {
            throw 'The explicit FTPS test container did not become ready on port 2121.'
        }
    }

    $env:QUICKFTP_FTP_INTEGRATION = '1'
    $env:QUICKFTP_FTP_INTEGRATION_FILES = "$Files"
    $env:QUICKFTP_FTP_HOST = '127.0.0.1'
    $env:QUICKFTP_FTP_PORT = '2121'
    $env:QUICKFTP_FTP_PROTOCOL = 'ftps_explicit'
    $env:QUICKFTP_FTP_TLS_VERIFY = 'false'
    $env:QUICKFTP_FTP_USERNAME = 'quickftp'
    $env:QUICKFTP_FTP_PASSWORD = 'quickftp-test-only'

    go test ./internal/transfer -run '^TestFTPDirectoryActivityIntegration$' -count=1 -v
    if ($LASTEXITCODE -ne 0) {
        throw "Explicit FTPS integration test failed with exit code $LASTEXITCODE."
    }
}
finally {
    Pop-Location
}
