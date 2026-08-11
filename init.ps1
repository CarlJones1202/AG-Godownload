# AG-Godownload Repository Initialization Script
# This script installs necessary dependencies and sets up the project.

$ErrorActionPreference = "Stop"

function Write-Step($message) {
    Write-Host "`n[STEP] $message" -ForegroundColor Cyan
}

function Write-Success($message) {
    Write-Host "[SUCCESS] $message" -ForegroundColor Green
}

function Write-Warning-Host($message) {
    Write-Host "[WARNING] $message" -ForegroundColor Yellow
}

# 1. Check for Admin Privileges
Write-Step "Checking for administrative privileges..."
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Warning-Host "This script may need administrative privileges to install packages via winget."
    Write-Warning-Host "If it fails, please run PowerShell as Administrator."
} else {
    Write-Success "Running as Administrator."
}

# 2. Check for winget
Write-Step "Checking for winget..."
if (-not (Get-Command "winget" -ErrorAction SilentlyContinue)) {
    Write-Error "winget is not installed. Please install 'App Installer' from the Microsoft Store."
}
Write-Success "winget found."

# 3. Install FFmpeg
Write-Step "Checking for FFmpeg..."
if (-not (Get-Command "ffmpeg" -ErrorAction SilentlyContinue)) {
    Write-Host "Installing FFmpeg via winget..."
    winget install --id Gyan.FFmpeg --silent --accept-package-agreements --accept-source-agreements
    # Refresh path
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
}
if (Get-Command "ffmpeg" -ErrorAction SilentlyContinue) {
    Write-Success "FFmpeg is ready."
} else {
    Write-Warning-Host "FFmpeg was installed but 'ffmpeg' command is not in the current session path."
    Write-Warning-Host "Restart your terminal after this script completes."
}

# 4. Install yt-dlp
Write-Step "Checking for yt-dlp..."
if (-not (Get-Command "yt-dlp" -ErrorAction SilentlyContinue)) {
    Write-Host "Installing yt-dlp via winget..."
    winget install --id yt-dlp.yt-dlp --silent --accept-package-agreements --accept-source-agreements --source winget
    # Refresh path
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
}
if (Get-Command "yt-dlp" -ErrorAction SilentlyContinue) {
    Write-Success "yt-dlp is ready."
} else {
    Write-Warning-Host "yt-dlp was installed but 'yt-dlp' command is not in the current session path."
}

# 5. Install Go
Write-Step "Checking for Go..."
if (-not (Get-Command "go" -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Go via winget..."
    winget install --id GoLang.Go --silent --accept-package-agreements --accept-source-agreements
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
}
if (Get-Command "go" -ErrorAction SilentlyContinue) {
    Write-Success "Go is ready ($(go version))."
} else {
    Write-Warning-Host "Go was installed but 'go' command is not in the current session path."
}

# 5. Install Node.js
Write-Step "Checking for Node.js..."
if (-not (Get-Command "node" -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Node.js via winget..."
    winget install --id OpenJS.NodeJS.LTS --silent --accept-package-agreements --accept-source-agreements
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
}

# 6. Install Python + gallery-dl
Write-Step "Checking for Python..."
if (-not (Get-Command "python" -ErrorAction SilentlyContinue)) {
    Write-Host "Python not found. Attempting to install via winget..."
    if (Get-Command "winget" -ErrorAction SilentlyContinue) {
        winget install --id Python.Python.3 -e --silent --accept-package-agreements --accept-source-agreements
        # Refresh path
        $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
    } else {
        Write-Warning-Host "winget not available; please install Python 3.8+ manually and ensure 'python' is on PATH."
    }
}

if (Get-Command "python" -ErrorAction SilentlyContinue) {
    Write-Success "Python is available ($(python --version 2>&1).Trim())"

    Write-Step "Installing/upgrading pip and gallery-dl (user install)..."
    python -m pip install --upgrade pip
    python -m pip install --user --upgrade gallery-dl

    # Ensure user scripts dir is in PATH for this session
    $userBase = & python -c "import site; print(site.USER_BASE)"
    $userBase = $userBase.Trim()
    if ($userBase -ne "") {
        $scriptDir = Join-Path $userBase "Scripts"
        if (Test-Path $scriptDir) {
            if ($env:PATH -notlike "*$scriptDir*") {
                $env:PATH = "$env:PATH;$scriptDir"
            }
        }
    }

    # Copy repo gallery-dl config to user config if present
    if (Test-Path ".gallery-dl\config.json") {
        $appData = $env:APPDATA
        if ($appData -eq $null -or $appData -eq '') {
            Write-Warning-Host "Could not determine APPDATA; skipping gallery-dl config copy."
        } else {
            $targetDir = Join-Path $appData "gallery-dl"
            if (-not (Test-Path $targetDir)) { New-Item -ItemType Directory -Path $targetDir -Force | Out-Null }
            Copy-Item -Path ".gallery-dl\config.json" -Destination (Join-Path $targetDir "config.json") -Force
            Write-Success "Copied .gallery-dl/config.json to $targetDir\config.json"
        }
    }
} else {
    Write-Warning-Host "Python is not available; skipping gallery-dl installation."
}
if (Get-Command "node" -ErrorAction SilentlyContinue) {
    Write-Success "Node.js is ready ($(node -v))."
} else {
    Write-Warning-Host "Node.js was installed but 'node' command is not in the current session path."
}

# 7. PostgreSQL
Write-Step "Setting up PostgreSQL (portable, local)..."

# --- PostgreSQL configuration variables ---
$pgPassword = if ($env:PG_PASSWORD) { $env:PG_PASSWORD } else { "postgres" }
$pgPort = if ($env:PG_PORT) { $env:PG_PORT.Trim() } else { "5432" }
$pgData = Join-Path $PWD "bin\pgdata"
$pgBinDir = Join-Path $PWD "bin\pgsql\bin"
$pgInitdb = Join-Path $pgBinDir "initdb.exe"
$pgCtl = Join-Path $pgBinDir "pg_ctl.exe"
$pgPsql = Join-Path $pgBinDir "psql.exe"
$pgIsReady = Join-Path $pgBinDir "pg_isready.exe"

# Ensure the bin directory exists
if (-not (Test-Path (Join-Path $PWD "bin"))) {
    New-Item -ItemType Directory -Path (Join-Path $PWD "bin") -Force | Out-Null
    Write-Success "Created 'bin' directory."
}

# --- VC++ runtime (required by PostgreSQL 17 MSVC binaries) ---
Write-Host "Checking for Microsoft Visual C++ Redistributable runtime..."
$system32 = Join-Path $env:WINDIR "System32"
$vcDllsMissing = -not (Test-Path (Join-Path $system32 "vcruntime140.dll")) -or
                 -not (Test-Path (Join-Path $system32 "vcruntime140_1.dll")) -or
                 -not (Test-Path (Join-Path $system32 "msvcp140.dll"))
if ($vcDllsMissing) {
    Write-Host "Installing Microsoft Visual C++ Redistributable (x64) via winget..."
    winget install --id Microsoft.VCRedist.2015+.x64 --silent --accept-package-agreements --accept-source-agreements
    Write-Host "winget finished (exit code $LASTEXITCODE)."
    if (-not (Test-Path (Join-Path $system32 "vcruntime140_1.dll"))) {
        Write-Warning-Host "VC++ runtime may still be missing; PostgreSQL may fail to start (STATUS_DLL_NOT_FOUND)."
    } else {
        Write-Success "VC++ runtime is ready."
    }
} else {
    Write-Success "VC++ runtime already present."
}

# --- Download and extract portable PostgreSQL binaries ---
if (-not (Test-Path $pgInitdb)) {
    Write-Host "Downloading PostgreSQL 17.4 portable binaries..."
    $pgZipUrl = "https://get.enterprisedb.com/postgresql/postgresql-17.4-1-windows-x64-binaries.zip"
    $pgZip = Join-Path $env:TEMP "postgresql-17.4-1-windows-x64-binaries.zip"
    try {
        Invoke-WebRequest -UseBasicParsing -Uri $pgZipUrl -OutFile $pgZip
    } catch {
        Write-Warning-Host "Failed to download PostgreSQL binaries: $_"
    }
    if (Test-Path $pgZip) {
        Write-Host "Extracting PostgreSQL binaries into bin\pgsql..."
        try {
            Expand-Archive -Path $pgZip -DestinationPath (Join-Path $PWD "bin") -Force
            Remove-Item -Path $pgZip -Force
        } catch {
            Write-Warning-Host "Failed to extract PostgreSQL binaries: $_"
        }
    }
} else {
    Write-Success "PostgreSQL binaries already present ($pgBinDir)."
}

if (-not (Test-Path $pgInitdb)) {
    Write-Warning-Host "PostgreSQL binaries are unavailable; skipping PostgreSQL setup."
} else {

    # --- Initialize the data directory ---
    if (-not (Test-Path (Join-Path $pgData "PG_VERSION"))) {
        Write-Host "Initializing PostgreSQL data directory at $pgData ..."
        if (-not (Test-Path $pgData)) {
            New-Item -ItemType Directory -Path $pgData -Force | Out-Null
        }
        $pwFile = New-TemporaryFile
        try {
            Set-Content -Path $pwFile.FullName -Value $pgPassword -NoNewline -Encoding Ascii
            & $pgInitdb -D $pgData -U postgres -A scram-sha-256 --pwfile=$($pwFile.FullName) -E UTF8 --no-locale
            if (Test-Path (Join-Path $pgData "PG_VERSION")) {
                Write-Success "PostgreSQL data directory initialized."
            } else {
                Write-Warning-Host "initdb did not complete successfully (exit code $LASTEXITCODE)."
            }
        } catch {
            Write-Warning-Host "initdb failed: $_"
        } finally {
            if (Test-Path $pwFile.FullName) { Remove-Item -Path $pwFile.FullName -Force }
        }
    } else {
        Write-Success "PostgreSQL data directory already initialized."
    }

    # --- Ensure server config listens on localhost with the chosen port ---
    $pgConf = Join-Path $pgData "postgresql.conf"
    if (Test-Path $pgConf) {
        if (-not (Select-String -Path $pgConf -Pattern '^\s*port\s*=' -Quiet)) {
            Add-Content -Path $pgConf -Value "port = $pgPort"
        }
        if (-not (Select-String -Path $pgConf -Pattern '^\s*listen_addresses\s*=' -Quiet)) {
            Add-Content -Path $pgConf -Value "listen_addresses = '127.0.0.1'"
        }
    }

    # --- Start the server if not already running ---
    & $pgIsReady -h 127.0.0.1 -p $pgPort | Out-Null
    if ($LASTEXITCODE -eq 0) {
        Write-Success "PostgreSQL is already accepting connections on port $pgPort."
    } else {
        Write-Host "Starting PostgreSQL via pg_ctl..."
        $pgLog = Join-Path $PWD "bin\pg.log"
        & $pgCtl -D $pgData -l $pgLog start
        $ready = $false
        for ($i = 0; $i -lt 30; $i++) {
            & $pgIsReady -h 127.0.0.1 -p $pgPort | Out-Null
            if ($LASTEXITCODE -eq 0) { $ready = $true; break }
            Start-Sleep -Seconds 1
        }
        if ($ready) {
            Write-Success "PostgreSQL started and accepting connections on port $pgPort."
        } else {
            Write-Warning-Host "PostgreSQL did not become ready within 30 seconds. Check $pgLog"
        }
    }

    # --- Create the gallery database if missing ---
    $hadPgPasswordEnv = Test-Path Env:\PGPASSWORD
    $savedPgPasswordEnv = $env:PGPASSWORD
    $env:PGPASSWORD = $pgPassword
    try {
        $existingDb = & $pgPsql -h 127.0.0.1 -p $pgPort -U postgres -tAc "SELECT 1 FROM pg_database WHERE datname='gallery'"
        if ($LASTEXITCODE -eq 0 -and $existingDb -and $existingDb.Trim() -eq "1") {
            Write-Success "Database 'gallery' already exists."
        } else {
            Write-Host "Creating 'gallery' database..."
            & $pgPsql -h 127.0.0.1 -p $pgPort -U postgres -c "CREATE DATABASE gallery"
            if ($LASTEXITCODE -eq 0) {
                Write-Success "Database 'gallery' created."
            } else {
                Write-Warning-Host "Failed to create 'gallery' database."
            }
        }
    } finally {
        if ($hadPgPasswordEnv) { $env:PGPASSWORD = $savedPgPasswordEnv }
        else { Remove-Item Env:\PGPASSWORD -ErrorAction SilentlyContinue }
    }

    # --- Upsert PostgreSQL settings into .env without clobbering existing keys ---
    function Update-EnvFile {
        param(
            [string]$EnvPath,
            [hashtable]$Values
        )
        $lines = @()
        if (Test-Path $EnvPath) {
            $lines = Get-Content -Path $EnvPath
        }
        foreach ($key in $Values.Keys) {
            $pattern = "^" + [regex]::Escape($key) + "="
            $matched = $false
            $updated = @()
            foreach ($line in $lines) {
                if ($line -match $pattern) {
                    $updated += "$key=$($Values[$key])"
                    $matched = $true
                } else {
                    $updated += $line
                }
            }
            if (-not $matched) {
                $updated += "$key=$($Values[$key])"
            }
            $lines = $updated
        }
        Set-Content -Path $EnvPath -Value $lines -Encoding Ascii
    }

    $databaseUrl = "postgres://postgres:$pgPassword@127.0.0.1:$pgPort/gallery?sslmode=disable"
    $envFile = Join-Path $PWD ".env"
    Update-EnvFile -EnvPath $envFile -Values @{
        "DATABASE_URL" = $databaseUrl
        "PGHOST"       = "127.0.0.1"
        "PGPORT"       = $pgPort
        "PGUSER"       = "postgres"
        "PGPASSWORD"   = $pgPassword
        "PGDATABASE"   = "gallery"
        "PGBIN"        = $pgBinDir
    }
    Write-Success "Updated $envFile with PostgreSQL connection settings."

    Write-Success "PostgreSQL setup complete: binaries in $pgBinDir, data in $pgData, database 'gallery' on port $pgPort."
}

# 6. Initialize Backend
Write-Step "Initializing backend (Go)..."
go mod download
go mod tidy
Write-Success "Backend initialized."

# 7. Initialize Frontend
if (Test-Path "frontend") {
    Write-Step "Initializing frontend (Node.js)..."
    Push-Location frontend
    npm install
    Pop-Location
    Write-Success "Frontend initialized."
}

# 8. Create necessary directories
Write-Step "Creating directories..."
if (-not (Test-Path "uploads")) {
    New-Item -ItemType Directory -Path "uploads" -Force | Out-Null
    Write-Success "Created 'uploads' directory."
} else {
    Write-Host "'uploads' directory already exists."
}

if (-not (Test-Path "bin")) {
    New-Item -ItemType Directory -Path "bin" -Force | Out-Null
    Write-Success "Created 'bin' directory."
} else {
    Write-Host "'bin' directory already exists."
}

# 9. Reminders
Write-Step "Post-initialization reminders:"
if (-not (Test-Path "wireguard.conf")) {
    Write-Warning-Host "Reminder: You may need a 'wireguard.conf' in the root directory if you plan to use VPN-based scraping."
    Write-Warning-Host "See WIREGUARD_SETUP.md for details."
}

Write-Host "`nInitialization Complete!" -ForegroundColor Green
Write-Host "If some commands were not found, please restart your terminal session."
