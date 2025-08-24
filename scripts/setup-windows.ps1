# Windows Setup Script for otel-desktop-viewer
# This script sets up MSYS2 and required packages for CGO compilation

Write-Host "Setting up Windows environment for otel-desktop-viewer..." -ForegroundColor Green

# Check if MSYS2 is installed
$msys2Path = "C:\msys64\ucrt64\bin\gcc.exe"
if (Test-Path $msys2Path) {
    Write-Host "MSYS2 GCC found at $msys2Path" -ForegroundColor Yellow
} else {
    Write-Host "MSYS2 not found. Please install MSYS2 from https://www.msys2.org/" -ForegroundColor Red
    Write-Host "After installing MSYS2, run this script again." -ForegroundColor Red
    exit 1
}

# Check if required packages are installed
$gccPath = "C:\msys64\ucrt64\bin\gcc.exe"
$gppPath = "C:\msys64\ucrt64\bin\g++.exe"

if (-not (Test-Path $gccPath) -or -not (Test-Path $gppPath)) {
    Write-Host "Installing required MSYS2 packages..." -ForegroundColor Yellow
    
    # Run pacman to install packages
    $pacmanCmd = "C:\msys64\usr\bin\pacman.exe -S --noconfirm mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-toolchain"
    
    Write-Host "Running: $pacmanCmd" -ForegroundColor Cyan
    Start-Process -FilePath "C:\msys64\usr\bin\bash.exe" -ArgumentList "-c", $pacmanCmd -Wait -NoNewWindow
    
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Failed to install MSYS2 packages. Please run manually:" -ForegroundColor Red
        Write-Host "1. Open MSYS2 UCRT64 terminal" -ForegroundColor Red
        Write-Host "2. Run: pacman -S mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-toolchain" -ForegroundColor Red
        exit 1
    }
    
    Write-Host "MSYS2 packages installed successfully!" -ForegroundColor Green
} else {
    Write-Host "Required MSYS2 packages are already installed." -ForegroundColor Green
}

# Add MSYS2 to PATH if not already there
$currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
$msys2BinPath = "C:\msys64\ucrt64\bin"

if ($currentPath -notlike "*$msys2BinPath*") {
    Write-Host "Adding MSYS2 to PATH..." -ForegroundColor Yellow
    $newPath = "$currentPath;$msys2BinPath"
    [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
    Write-Host "MSYS2 added to PATH. Please restart your terminal for changes to take effect." -ForegroundColor Green
} else {
    Write-Host "MSYS2 is already in PATH." -ForegroundColor Green
}

# Test if everything works
Write-Host "Testing setup..." -ForegroundColor Yellow
$env:PATH += ";$msys2BinPath"

try {
    $gccVersion = & "C:\msys64\ucrt64\bin\gcc.exe" --version 2>&1 | Select-Object -First 1
    Write-Host "GCC version: $gccVersion" -ForegroundColor Green
    
    $gppVersion = & "C:\msys64\ucrt64\bin\g++.exe" --version 2>&1 | Select-Object -First 1
    Write-Host "G++ version: $gppVersion" -ForegroundColor Green
    
    Write-Host "Setup completed successfully!" -ForegroundColor Green
    Write-Host "You can now run: go install github.com/CtrlSpice/otel-desktop-viewer@latest" -ForegroundColor Cyan
    
} catch {
    Write-Host "Error testing setup: $_" -ForegroundColor Red
    exit 1
}
