# Starts the portable PostgreSQL database
$pgData = Join-Path $PSScriptRoot "bin\pgdata"
$pgBinDir = Join-Path $PSScriptRoot "bin\pgsql\bin"
$pgCtl = Join-Path $pgBinDir "pg_ctl.exe"
$pgIsReady = Join-Path $pgBinDir "pg_isready.exe"
$pgLog = Join-Path $PSScriptRoot "bin\pg.log"

if (-not (Test-Path $pgCtl)) {
    Write-Error "PostgreSQL binaries not found at $pgBinDir. Run ./init.ps1 first."
    exit 1
}

& $pgIsReady -h 127.0.0.1 -p 5432 | Out-Null
if ($LASTEXITCODE -eq 0) {
    Write-Host "PostgreSQL is already running and accepting connections on port 5432." -ForegroundColor Green
    exit 0
}

Write-Host "Starting PostgreSQL..." -ForegroundColor Cyan
& $pgCtl -D $pgData -l $pgLog start

$ready = $false
for ($i = 0; $i -lt 15; $i++) {
    & $pgIsReady -h 127.0.0.1 -p 5432 | Out-Null
    if ($LASTEXITCODE -eq 0) {
        $ready = $true
        break
    }
    Start-Sleep -Seconds 1
}

if ($ready) {
    Write-Host "PostgreSQL started successfully and is accepting connections." -ForegroundColor Green
} else {
    Write-Warning "PostgreSQL failed to respond within 15 seconds. Check $pgLog for details."
}
