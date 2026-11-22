# Installs or updates clumyap on Windows. In PowerShell:
#   irm https://raw.githubusercontent.com/adamwahyuh/clumyap/main/install.ps1 | iex
# Files go to %LOCALAPPDATA%\Programs\clumyap, which is added to your user PATH; no admin needed.
# Uninstall: delete that folder and remove it from PATH in "Edit environment variables for your account".
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue' # the progress bar makes Invoke-WebRequest many times slower

$repo = 'adamwahyuh/clumyap'
# Only an x64 build exists; Windows on ARM runs it through its built-in emulation.
$url = if ($env:CLUMYAP_URL) { $env:CLUMYAP_URL } else { "https://github.com/$repo/releases/latest/download/clumyap-windows-amd64.zip" }
$dir = Join-Path $env:LOCALAPPDATA 'Programs\clumyap'
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("clumyap-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    Write-Host "Downloading clumyap for Windows..."
    if (Test-Path -LiteralPath $url) {
        Copy-Item -LiteralPath $url -Destination "$tmp\clumyap.zip" # a local archive, used by the tests
    } else {
        Invoke-WebRequest -Uri $url -OutFile "$tmp\clumyap.zip" -UseBasicParsing
    }
    Expand-Archive -Path "$tmp\clumyap.zip" -DestinationPath $tmp -Force
    if (-not (Test-Path "$tmp\clumyap\clumyap.exe")) { throw "the download is not a clumyap archive" }

    # A running clumyap (as during /update) keeps its .exe and .dll files locked: Windows will not
    # delete or overwrite them, but it does allow renaming. So old files are moved aside and new ones
    # copied in; the leftovers are removed on the next install, once nothing uses them.
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    Get-ChildItem -Path $dir -Filter '*.old' | Remove-Item -Force -ErrorAction SilentlyContinue
    Get-ChildItem -Path $dir -File | Where-Object { $_.Extension -ne '.old' } | ForEach-Object {
        Rename-Item -LiteralPath $_.FullName -NewName ("{0}.{1}.old" -f $_.Name, [guid]::NewGuid().ToString('N').Substring(0, 8))
    }
    Copy-Item -Path "$tmp\clumyap\*" -Destination $dir -Recurse -Force
} finally {
    Remove-Item -Path $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

$version = & "$dir\clumyap.exe" --version
Write-Host "Installed $version to $dir"

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (($userPath -split ';') -contains $dir)) {
    $newPath = if ($userPath) { "$userPath;$dir" } else { $dir }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
    Write-Host "Added $dir to your PATH. Open a new terminal so the clumyap command is found."
}
$env:Path = "$env:Path;$dir"

Write-Host ""
Write-Host "Get started:  clumyap start"
Write-Host "The first run downloads the voice model (about 350 MB)."
