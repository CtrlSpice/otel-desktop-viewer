param(
    [Parameter(Mandatory = $true)]
    [string]$Executable
)

$ErrorActionPreference = 'Stop'
& $Executable --version
if ($LASTEXITCODE -ne 0) { throw 'Version command failed' }
$process = Start-Process -FilePath $Executable -ArgumentList '--open-browser=false --browser-port 18000 --http 14318 --grpc 14317' -PassThru
try {
    $ready = $false
    $deadline = (Get-Date).AddSeconds(30)
    while ((Get-Date) -lt $deadline) {
        if ($process.HasExited) { throw "Viewer exited during startup: $($process.ExitCode)" }
        try {
            $response = Invoke-WebRequest -Uri 'http://127.0.0.1:18000/' -TimeoutSec 2
            $ready = $response.StatusCode -eq 200 -and $response.Content -match '<html'
        } catch {
            # The HTTP listener may not be ready yet.
        }
        if ($ready) { break }
        Start-Sleep -Milliseconds 250
    }
    if (-not $ready) { throw 'Viewer did not serve its UI within 30 seconds' }
    if ($process.HasExited) { throw "Viewer exited after startup: $($process.ExitCode)" }
    Write-Host 'Viewer serves its embedded UI'
} finally {
    if (-not $process.HasExited) { Stop-Process -InputObject $process -Force }
}
