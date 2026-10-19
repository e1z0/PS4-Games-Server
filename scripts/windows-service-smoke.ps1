# SPDX-License-Identifier: GPL-2.0-only
# Run from the repository root in elevated PowerShell (Windows CI is elevated).
$ErrorActionPreference = 'Stop'
if (-not $env:RUNNER_TEMP) { throw 'RUNNER_TEMP must point to an isolated scratch directory' }
$root = Join-Path $env:RUNNER_TEMP ('PS4 service smoke ' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $root | Out-Null
$exe = Join-Path $root 'ps4-server.exe'
$installed = $false
$oldPublic = $env:PS4_PUBLIC_URL
$oldUser = $env:PS4_ADMIN_USERNAME
$oldPassword = $env:PS4_ADMIN_PASSWORD
try {
    go build -o $exe ./cmd/ps4-server
    if ($LASTEXITCODE) { throw 'Windows build failed' }
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = $listener.LocalEndpoint.Port
    $listener.Stop()
    $env:PS4_PUBLIC_URL = "http://127.0.0.1:$port"
    $env:PS4_ADMIN_USERNAME = 'smoke-admin'
    $env:PS4_ADMIN_PASSWORD = [guid]::NewGuid().ToString()
    Push-Location $root
    try {
        & $exe init
        if ($LASTEXITCODE) { throw 'Config init failed' }
        $cfg = Get-Content config.json -Raw | ConvertFrom-Json
        $cfg.listen = "127.0.0.1:$port"
        $cfg.data_dir = Join-Path $root 'data'
        $cfg | ConvertTo-Json -Depth 20 | Set-Content -Encoding utf8 config.json
        # Windows PowerShell 5 writes a BOM; Go JSON expects no BOM.
        $text = Get-Content config.json -Raw
        [IO.File]::WriteAllText((Join-Path $root 'config.json'), $text, [Text.UTF8Encoding]::new($false))
        & icacls.exe $root /grant '*S-1-5-19:(OI)(CI)M' /T /Q
        if ($LASTEXITCODE) { throw 'Local Service permission grant failed' }
        # Verify Explorer-style, no-argument launch finds the EXE's config even
        # when Windows supplies a different process working directory.
        $console = Start-Process -FilePath $exe -WorkingDirectory ([IO.Path]::GetTempPath()) -PassThru -RedirectStandardOutput (Join-Path $root 'console.log') -RedirectStandardError (Join-Path $root 'console-error.log')
        try {
            $healthy = $false
            foreach ($poll in 1..100) {
                try {
                    $response = Invoke-WebRequest "http://127.0.0.1:$port/health" -TimeoutSec 1
                    if ($response.StatusCode -eq 200) { $healthy = $true; break }
                } catch { Start-Sleep -Milliseconds 200 }
            }
            if (-not $healthy) { throw 'No-argument EXE startup failed' }
        } finally {
            if (-not $console.HasExited) { Stop-Process -Id $console.Id -Force }
            $console.WaitForExit()
        }
        & $exe service install
        if ($LASTEXITCODE) { throw 'Service installation failed' }
        $installed = $true
        $service = Get-CimInstance Win32_Service -Filter "Name='PS4GamesServer'"
        if ($service.StartName -ne 'NT AUTHORITY\LocalService' -or $service.StartMode -ne 'Auto') { throw 'Unexpected service account or startup mode' }
        foreach ($attempt in 1..2) {
            & $exe service start
            if ($LASTEXITCODE) { throw 'Service start failed' }
            & $exe health
            if ($LASTEXITCODE) { throw 'Service health failed' }
            $status = & $exe service status
            if ($LASTEXITCODE -or $status -ne 'running') { throw 'Service status failed' }
            & $exe service stop
            if ($LASTEXITCODE) { throw 'Service stop failed' }
        }
        & $exe service uninstall
        if ($LASTEXITCODE) { throw 'Service uninstall failed' }
        $installed = $false
        if (-not (Test-Path config.json) -or -not (Test-Path data)) { throw 'Uninstall removed persistent files' }
    } finally { Pop-Location }
} finally {
    if ($installed) { & $exe service uninstall }
    $env:PS4_PUBLIC_URL = $oldPublic
    $env:PS4_ADMIN_USERNAME = $oldUser
    $env:PS4_ADMIN_PASSWORD = $oldPassword
    # Retain the isolated scratch folder/log for CI failure inspection.
    if (Test-Path (Join-Path $root 'service.log')) { Get-Content (Join-Path $root 'service.log') }
}
