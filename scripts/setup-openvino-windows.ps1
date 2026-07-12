<#
.SYNOPSIS
    Sets up the .openvino virtualenv and GenAI sources on Windows using native Python.

.DESCRIPTION
    This script uses a real Windows Python for venv creation and pip installs.
    It also handles the GenAI source clone/worktree + extra header downloads cleanly.

    Run this from regular PowerShell or cmd.exe.

.EXAMPLE
    .\scripts\setup-openvino-windows.ps1

.EXAMPLE
    .\scripts\setup-openvino-windows.ps1 -PythonPath "C:\Python312\python.exe" -Version "2026.2.0.0"
#>
param(
    [string]$PythonPath = "",
    [string]$Version = "2026.2.0.0"
)

$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

if (-not $PythonPath) {
    # Common default on user machines (adjust if yours is different)
    $PythonPath = "C:\Users\$env:USERNAME\AppData\Local\Programs\Python\Python312\python.exe"
}

if (-not (Test-Path $PythonPath)) {
    Write-Error "Python not found at '$PythonPath'. Pass -PythonPath to the full path of your native python.exe"
}

Write-Host "Using native Python: $PythonPath" -ForegroundColor Green

$VenvDir = Join-Path $Root ".openvino\venv"
$GenaiRepo = Join-Path $Root ".openvino\openvino.genai"
$GenaiSrc = Join-Path $Root ".openvino\genai-$Version"

# 1. Create venv
Write-Host "Creating venv at $VenvDir ..."
& $PythonPath -m venv $VenvDir

$VenvPython = Join-Path $VenvDir "Scripts\python.exe"
$VenvPip = Join-Path $VenvDir "Scripts\pip.exe"

# 2. Upgrade pip and install packages
Write-Host "Upgrading pip and installing OpenVINO packages ..."
& $VenvPython -m pip install --upgrade pip
& $VenvPip install openvino openvino-genai==$Version huggingface_hub

Write-Host "OpenVINO venv ready." -ForegroundColor Green

# 3. GenAI sources + headers (replicates genai-src target)
Write-Host "Setting up GenAI sources at $GenaiSrc ..."

if (-not (Test-Path (Join-Path $GenaiRepo ".git"))) {
    git clone https://github.com/openvinotoolkit/openvino.genai.git $GenaiRepo
}
git -C $GenaiRepo fetch --tags --force

if (-not (Test-Path $GenaiSrc)) {
    git -C $GenaiRepo worktree add --detach $GenaiSrc $Version
}

$DepsDir = Join-Path $GenaiSrc "build\_deps"
New-Item -ItemType Directory -Path $DepsDir -Force | Out-Null

# nlohmann/json
$JsonHeader = Join-Path $DepsDir "nlohmann_json-src\single_include\nlohmann\json.hpp"
if (-not (Test-Path $JsonHeader)) {
    $url = "https://github.com/nlohmann/json/archive/refs/tags/v3.11.3.tar.gz"
    $tar = Join-Path $DepsDir "json.tar.gz"
    Write-Host "Downloading nlohmann/json ..."
    Invoke-WebRequest -Uri $url -OutFile $tar -UseBasicParsing
    tar -xzf $tar -C $DepsDir
    Move-Item (Join-Path $DepsDir "json-3.11.3") (Join-Path $DepsDir "nlohmann_json-src") -Force
}

# minja
$MinjaHeader = Join-Path $DepsDir "minja-src\include\minja\minja.hpp"
if (-not (Test-Path $MinjaHeader)) {
    Write-Host "Cloning minja ..."
    git clone https://github.com/google/minja.git (Join-Path $DepsDir "minja-src")
    git -C (Join-Path $DepsDir "minja-src") checkout 3e4c61c616eda133cfb1e440fc7a14bf1729bbee
}

# safetensors.h
$SafeHeader = Join-Path $DepsDir "safetensors.h-src\safetensors.h"
if (-not (Test-Path $SafeHeader)) {
    $url = "https://github.com/hsnyder/safetensors.h/archive/974a85d7dfd6e010558353226638bb26d6b9d756.tar.gz"
    $tar = Join-Path $DepsDir "safetensors.tar.gz"
    Write-Host "Downloading safetensors.h ..."
    Invoke-WebRequest -Uri $url -OutFile $tar -UseBasicParsing
    tar -xzf $tar -C $DepsDir
    Move-Item (Join-Path $DepsDir "safetensors.h-974a85d7dfd6e010558353226638bb26d6b9d756") (Join-Path $DepsDir "safetensors.h-src") -Force
}

# gguflib
$GguflibHeader = Join-Path $DepsDir "gguflib-src\gguflib.h"
if (-not (Test-Path $GguflibHeader)) {
    Write-Host "Cloning gguflib ..."
    git clone https://github.com/Lourdle/gguf-tools.git (Join-Path $DepsDir "gguflib-src")
    git -C (Join-Path $DepsDir "gguflib-src") checkout bac796ada809ac293e685db59b075971181cb008
}

Write-Host ""
Write-Host "OpenVINO setup complete!" -ForegroundColor Green
Write-Host "Venv: $VenvDir"
Write-Host "GenAI sources: $GenaiSrc"
Write-Host ""
Write-Host "Next steps (with MinGW-w64 or VS Build Tools + Clang in PATH):"
Write-Host "  make -f Makefile.llamacpp-direct runtime"
Write-Host "  make build-modeld"
Write-Host "  CONTENOX_MODELD_BACKEND=openvino ./bin/modeld.exe serve"
Write-Host ""
Write-Host "Tip: run this script again any time you want a clean reinstall."
