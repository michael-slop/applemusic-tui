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
    # Same version label as the Makefile: exact tag, else short SHA, else dev.
    # Windows PowerShell 5.1 turns a native command's stderr into a
    # terminating error under "Stop", so git runs with errors tolerated.
    $version = & {
      $ErrorActionPreference = "Continue"
      $v = git describe --tags --exact-match 2>$null
      if (-not $v) { $v = git rev-parse --short HEAD 2>$null }
      $v
    }
    if (-not $version) { $version = "dev" }
    go build -trimpath -ldflags="-s -w -X main.version=$version" -o $staged .
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

# Windows can rename a running exe but not overwrite or delete it, so an amtui
# that is still open moves aside to a unique name instead of failing the
# upgrade. Aside copies whose amtui has since exited are swept on each install.
Get-ChildItem $Prefix -Filter "amtui.exe.old*" -ErrorAction SilentlyContinue |
  Remove-Item -Force -ErrorAction SilentlyContinue
if (Test-Path $target) { Move-Item $target "$target.old-$([DateTime]::Now.Ticks)" -Force }
Move-Item $staged $target -Force
Write-Host "`nInstalled: $target"

if (-not $NoPath) {
  # Read and write the raw registry value: [Environment]'s Path API expands
  # %USERPROFILE%-style entries and stores the result as REG_SZ, flattening
  # the stock Windows user PATH.
  $envKey = Get-Item HKCU:\Environment
  $userPath = $envKey.GetValue("Path", "", "DoNotExpandEnvironmentNames")
  $parts = @($userPath -split ";" | Where-Object { $_ })
  if ($parts -notcontains $Prefix) {
    Set-ItemProperty HKCU:\Environment -Name Path -Value (($parts + $Prefix) -join ";") -Type ExpandString
    # Tell Explorer the environment changed so new terminals see the new PATH.
    $sig = '[DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr SendMessageTimeout(IntPtr h, uint m, UIntPtr w, string l, uint f, uint t, out UIntPtr r);'
    $user32 = Add-Type -MemberDefinition $sig -Name Env -Namespace AmtuiInstall -PassThru
    $r = [UIntPtr]::Zero
    [void]$user32::SendMessageTimeout([IntPtr]0xffff, 0x1A, [UIntPtr]::Zero, "Environment", 2, 5000, [ref]$r)
    Write-Host "Added $Prefix to your user PATH. Open a new terminal to use 'amtui'."
  }
}

if (-not (Find-Chrome)) {
  Write-Warning "Google Chrome was not found. amtui needs it:  winget install Google.Chrome"
  Write-Warning "(Or point AMTUI_CHROME at a Widevine-capable Chrome/Edge executable.)"
}
Write-Host "`nRun: amtui   (Windows Terminal recommended)"
