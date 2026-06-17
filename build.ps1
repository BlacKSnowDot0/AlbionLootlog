# Local build helper for Windows + Npcap (PowerShell).
#
# Sets the CGO env vars needed to compile the capture package against the
# Npcap SDK, then builds the lootlogger binary.
#
# Usage:
#   .\build.ps1            # build
#   .\build.ps1 -Test      # build + run tests
#   .\build.ps1 -Run       # build + run (auto-detect adapter)

param(
    [switch]$Test,
    [switch]$Run
)

$ErrorActionPreference = "Stop"

# Locate the Npcap SDK / installed libs. Adjust if your install differs.
$npcapInclude = "C:\Program Files\Npcap\Include"
$npcapLib = "C:\Program Files\Npcap\Lib\x64"

if (-not (Test-Path "$npcapInclude\pcap.h")) {
    Write-Error "pcap.h not found at $npcapInclude. Install Npcap with SDK, or edit build.ps1."
}
if (-not (Test-Path "$npcapLib\wpcap.lib")) {
    Write-Error "wpcap.lib not found at $npcapLib. Edit build.ps1 to point at your Npcap Lib dir."
}

$env:CGO_ENABLED = "1"
$env:CGO_CFLAGS = "-I$npcapInclude"
$env:CGO_LDFLAGS = "-L$npcapLib -lwpcap -lPacket"

if ($Test) {
    Write-Host "Running tests..." -ForegroundColor Cyan
    go test ./...
    exit $LASTEXITCODE
}

Write-Host "Building AlbionLootlog.exe..." -ForegroundColor Cyan
go build -o AlbionLootlog.exe ./cmd/lootlogger
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "Built: $PWD\AlbionLootlog.exe" -ForegroundColor Green

if ($Run) {
    Write-Host "Running (Ctrl+C to stop)..." -ForegroundColor Cyan
    .\AlbionLootlog.exe
}
