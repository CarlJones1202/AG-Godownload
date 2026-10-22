# Stops the portable PostgreSQL database
$pgData = Join-Path $PSScriptRoot "bin\pgdata"
$pgBinDir = Join-Path $PSScriptRoot "bin\pgsql\bin"
$pgCtl = Join-Path $pgBinDir "pg_ctl.exe"

if (-not (Test-Path $pgCtl)) {
    Write-Error "PostgreSQL binaries not found at $pgBinDir."
    exit 1
}

Write-Host "Stopping PostgreSQL..." -ForegroundColor Cyan
& $pgCtl -D $pgData stop -m fast
