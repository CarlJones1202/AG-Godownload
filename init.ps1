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

# 6.1 Install local semantic embedding runtime and model
Write-Step "Setting up local SigLIP 2 semantic embeddings..."
$pythonLauncher = Get-Command "py" -ErrorAction SilentlyContinue
if (-not $pythonLauncher) {
    Write-Host "Installing Python 3.13 via winget..."
    winget install --id Python.Python.3.13 --silent --accept-package-agreements --accept-source-agreements
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" +
        [System.Environment]::GetEnvironmentVariable("Path", "User")
    $pythonLauncher = Get-Command "py" -ErrorAction SilentlyContinue
}
if (-not $pythonLauncher) {
    throw "Python launcher is unavailable after installation."
}

& $pythonLauncher.Source -3.13 --version
if ($LASTEXITCODE -ne 0) {
    Write-Host "Installing Python 3.13 via winget..."
    winget install --id Python.Python.3.13 --silent --accept-package-agreements --accept-source-agreements
    if ($LASTEXITCODE -ne 0) { throw "Failed to install Python 3.13." }
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" +
        [System.Environment]::GetEnvironmentVariable("Path", "User")
    $pythonLauncher = Get-Command "py" -ErrorAction SilentlyContinue
    if (-not $pythonLauncher) { throw "Python launcher is unavailable after installing Python 3.13." }
    & $pythonLauncher.Source -3.13 --version
    if ($LASTEXITCODE -ne 0) { throw "Python 3.13 is required for the local semantic embedding runtime." }
}

$embeddingVenv = Join-Path $PWD "bin\embedding-venv"
$embeddingPython = Join-Path $embeddingVenv "Scripts\python.exe"
if (-not (Test-Path $embeddingPython)) {
    & $pythonLauncher.Source -3.13 -m venv $embeddingVenv
    if ($LASTEXITCODE -ne 0) { throw "Failed to create the semantic embedding virtual environment." }
}
& $embeddingPython -m pip install --upgrade pip
if ($LASTEXITCODE -ne 0) { throw "Failed to update pip in the semantic embedding environment." }
& $embeddingPython -m pip install -r (Join-Path $PWD "requirements-embedding.txt")
if ($LASTEXITCODE -ne 0) { throw "Failed to install semantic embedding dependencies." }

$modelDirectory = Join-Path $PWD "models"
if (-not (Test-Path $modelDirectory)) {
    New-Item -ItemType Directory -Path $modelDirectory -Force | Out-Null
}
$semanticModel = Join-Path $modelDirectory "siglip2-base-patch16-224-int8.onnx"
$semanticModelSha256 = "0dd31785a2713f1113ef2272472165c69d580473dae38d7b47568ac587795e70"
if (Test-Path $semanticModel) {
    $actualHash = (Get-FileHash -Path $semanticModel -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $semanticModelSha256) {
        throw "Existing semantic model has an unexpected SHA-256 hash: $semanticModel"
    }
} else {
    $modelDownload = "$semanticModel.download"
    $modelUrl = "https://huggingface.co/onnx-community/siglip2-base-patch16-224-ONNX/resolve/ba1f3b0843f24bc5417d38e19c37b287d719b2f4/onnx/vision_model_int8.onnx"
    Write-Host "Downloading the pinned 95 MB SigLIP 2 vision model..."
    Invoke-WebRequest -UseBasicParsing -Uri $modelUrl -OutFile $modelDownload
    $actualHash = (Get-FileHash -Path $modelDownload -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $semanticModelSha256) {
        Remove-Item -LiteralPath $modelDownload -Force
        throw "Downloaded semantic model failed SHA-256 verification."
    }
    Move-Item -LiteralPath $modelDownload -Destination $semanticModel
}

$envPath = Join-Path $PWD ".env"
$envLines = @()
if (Test-Path $envPath) {
    $envLines = Get-Content -Path $envPath
}
$semanticDefaults = @{
    EMBED_MODEL_PATH = "models\siglip2-base-patch16-224-int8.onnx"
    EMBED_MODEL_NAME = "siglip2-base-patch16-224-int8-v1"
    EMBED_DIM = "768"
    EMBED_PYTHON = "bin\embedding-venv\Scripts\python.exe"
}
$configuredModelPath = ""
foreach ($line in $envLines) {
    if ($line -match "^EMBED_MODEL_PATH=(.*)$") {
        $configuredModelPath = $Matches[1].Trim().Trim('"')
        break
    }
}
$useSiglipDefaults = [string]::IsNullOrWhiteSpace($configuredModelPath) -or
    $configuredModelPath -eq $semanticDefaults.EMBED_MODEL_PATH
foreach ($key in $semanticDefaults.Keys) {
    $pattern = "^" + [regex]::Escape($key) + "="
    $matched = $false
    $updated = @()
    foreach ($line in $envLines) {
        if ($line -match $pattern) {
            $matched = $true
            $forceUpdate = $useSiglipDefaults -and $key -ne "EMBED_MODEL_PATH"
            if (-not $forceUpdate -and $line.Substring($key.Length + 1).Trim().Length -gt 0) {
                $updated += $line
            } else {
                $updated += "$key=$($semanticDefaults[$key])"
            }
        } else {
            $updated += $line
        }
    }
    if (-not $matched) {
        $updated += "$key=$($semanticDefaults[$key])"
    }
    $envLines = $updated
}
Set-Content -Path $envPath -Value $envLines -Encoding Ascii
Write-Success "Local semantic embeddings are configured. Images are processed on this machine."

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
