param (
    [switch]$BuildServer,
    [switch]$BuildOrchestrator,
    [switch]$All
)

# If no specific switches are provided, build all
if (-not $BuildServer -and -not $BuildOrchestrator) {
    $All = $true
}

$VERSION = "dev"
try {
    $gitVersion = git describe --tags --always --dirty 2>$null
    if ($LASTEXITCODE -eq 0) {
        $VERSION = $gitVersion
    }
} catch {
    # Git not found, stick to "dev"
}

$LDFLAGS = "-X main.version=$VERSION -s -w"
$BUILD_DIR = ".\bin"

if (-not (Test-Path -Path $BUILD_DIR)) {
    New-Item -ItemType Directory -Force -Path $BUILD_DIR | Out-Null
}

if ($All -or $BuildServer) {
    Write-Host "==> Building dbx-server..." -ForegroundColor Green
    go build -ldflags "-X main.version=$VERSION -s -w" -o "$BUILD_DIR\dbx-server.exe" .\cmd\dbx-server
}

if ($All -or $BuildOrchestrator) {
    Write-Host "==> Building dbx-orchestrator..." -ForegroundColor Green
    go build -ldflags "-X main.version=$VERSION -s -w" -o "$BUILD_DIR\dbx-orchestrator.exe" .\cmd\dbx-orchestrator
}

Write-Host "==> Build complete! Binaries are in $BUILD_DIR" -ForegroundColor Cyan
