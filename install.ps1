<#
.SYNOPSIS
  Install amtui (Apple Music TUI) on Windows.

.DESCRIPTION
  Builds amtui from this source checkout when Go is installed, otherwise
  downloads the latest Windows release from GitHub. Installs to
  %LOCALAPPDATA%\Programs\amtui and adds that folder to your user PATH.

  amtui needs Google Chrome (it plays music through Apple's web player, and
  only a Widevine-capable browser can decrypt it):  winget install Google.Chrome

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File install.ps1
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File install.ps1 -Binary .\amtui.exe
#>
param(
  [string]$Prefix = (Join-Path $env:LOCALAPPDATA "Programs\amtui"),
  [string]$Binary = "",                 # install this exe instead of building/downloading
  [string]$Repo = "michael-slop/applemusic-tui",
  [switch]$NoPath
)
$ErrorActionPreference = "Stop"

function Find-Chrome {
  $candidates = @(
    (Join-Path $env:ProgramFiles "Google\Chrome\Application\chrome.exe"),
    (Join-Path ${env:ProgramFiles(x86)} "Google\Chrome\Application\chrome.exe"),
    (Join-Path $env:LOCALAPPDATA "Google\Chrome\Application\chrome.exe")
  )
  foreach ($c in $candidates) { if ($c -and (Test-Path $c)) { return $c } }
  return $null
}

New-Item -ItemType Directory -Force $Prefix | Out-Null
$target = Join-Path $Prefix "amtui.exe"
$staged = "$target.new"

if ($Binary) {
  Copy-Item $Binary $staged -Force
} elseif ((Get-Command go -ErrorAction SilentlyContinue) -and (Test-Path (Join-Path $PSScriptRoot "go.mod"))) {
  Write-Host "Building amtui with $(go version)..."
  Push-Location $PSScriptRoot
  try {
    $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags="-s -w" -o $staged .
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
  } finally { Pop-Location }
} else {
  $arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
  Write-Host "Go not found; downloading the latest release from github.com/$Repo ..."
  $rel = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
  $asset = $rel.assets | Where-Object { $_.name -like "amtui-*-windows-$arch.zip" } | Select-Object -First 1
  if (-not $asset) { throw "no windows-$arch build in the latest release of $Repo" }
  $zip = Join-Path $env:TEMP $asset.name
  Invoke-WebRequest $asset.browser_download_url -OutFile $zip
  $unz = Join-Path $env:TEMP "amtui-unzip"
  if (Test-Path $unz) { Remove-Item -Recurse -Force $unz }
  Expand-Archive $zip $unz
  Copy-Item (Get-ChildItem $unz -Recurse -Filter amtui.exe | Select-Object -First 1).FullName $staged -Force
  Remove-Item -Recurse -Force $unz, $zip
}

Move-Item $staged $target -Force
Write-Host "`nInstalled: $target"

if (-not $NoPath) {
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $parts = @($userPath -split ";" | Where-Object { $_ })
  if ($parts -notcontains $Prefix) {
    [Environment]::SetEnvironmentVariable("Path", (($parts + $Prefix) -join ";"), "User")
    Write-Host "Added $Prefix to your user PATH. Open a new terminal to use 'amtui'."
  }
}

if (-not (Find-Chrome)) {
  Write-Warning "Google Chrome was not found. amtui needs it:  winget install Google.Chrome"
  Write-Warning "(Or point AMTUI_CHROME at a Widevine-capable Chrome/Edge executable.)"
}
Write-Host "`nRun: amtui   (Windows Terminal recommended)"
