# Installs or updates clumyap on Windows. In PowerShell:
#   irm https://raw.githubusercontent.com/adamwahyuh/clumyap/main/install.ps1 | iex
# Files go to %LOCALAPPDATA%\Programs\clumyap, which is added to your user PATH; no admin needed.
# Uninstall: clumyap uninstall
# This file is kept ASCII; the symbols it draws are built from character codes.
$ErrorActionPreference = 'Stop'

$repo = 'adamwahyuh/clumyap'
# Only an x64 build exists; Windows on ARM runs it through its built-in emulation.
$url = if ($env:CLUMYAP_URL) { $env:CLUMYAP_URL } else { "https://github.com/$repo/releases/latest/download/clumyap-windows-amd64.zip" }
$dir = Join-Path $env:LOCALAPPDATA 'Programs\clumyap'

# Animation only in an interactive console; `clumyap update` in the TUI and CI get plain lines.
# CLUMYAP_FANCY=1 forces it on, so CI can run the drawing code too.
$fancy = ($env:CLUMYAP_FANCY -eq '1') -or -not [Console]::IsOutputRedirected
# Older consoles are not UTF-8 and would show '?' for these, so they fall back to ASCII.
if ([Console]::OutputEncoding.CodePage -eq 65001) {
    $tick = [string][char]0x2713; $note = [string][char]0x266A; $full = [string][char]0x2588; $empty = [string][char]0x2591
    $frames = 0x280B, 0x2819, 0x2839, 0x2838, 0x283C, 0x2834, 0x2826, 0x2827, 0x2807, 0x280F | ForEach-Object { [string][char]$_ }
} else {
    $tick = 'OK'; $note = '*'; $full = '#'; $empty = '-'; $frames = '|', '/', '-', '\'
}

function Ok($text, $detail) {
    Write-Host '  ' -NoNewline
    Write-Host $tick -ForegroundColor Green -NoNewline
    Write-Host " $text" -NoNewline
    if ($detail) { Write-Host " $detail" -ForegroundColor DarkGray } else { Write-Host '' }
}

function Show-Progress([long]$got, $total, [int]$i) {
    Write-Host ("`r  " + $frames[$i % $frames.Count]) -ForegroundColor Magenta -NoNewline
    Write-Host ' Downloading  ' -NoNewline
    if ($total) {
        $filled = [int][math]::Floor($got * 20 / $total)
        Write-Host (($full * $filled) + ($empty * (20 - $filled))) -ForegroundColor Magenta -NoNewline
        Write-Host ('  {0,3}%  ' -f [int][math]::Floor($got * 100 / $total)) -NoNewline
        Write-Host ('{0}/{1} MB   ' -f [int][math]::Floor($got / 1MB), [int][math]::Floor($total / 1MB)) -ForegroundColor DarkGray -NoNewline
    } else {
        Write-Host ('{0} MB   ' -f [int][math]::Floor($got / 1MB)) -ForegroundColor DarkGray -NoNewline
    }
}

# Save-Download streams the file and draws its own progress: PowerShell's built-in progress bar
# makes Invoke-WebRequest many times slower.
function Save-Download($from, $to) {
    if (Test-Path -LiteralPath $from) { Copy-Item -LiteralPath $from -Destination $to; return } # a local archive, used by the tests
    if (-not $fancy) { Write-Host "  Downloading $from" }
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    Add-Type -AssemblyName System.Net.Http
    $client = New-Object System.Net.Http.HttpClient
    try {
        $resp = $client.GetAsync($from, [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead).GetAwaiter().GetResult()
        if (-not $resp.IsSuccessStatusCode) { throw "clumyap: download failed ($([int]$resp.StatusCode)): $from" }
        $total = $resp.Content.Headers.ContentLength
        $in = $resp.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
        $out = [IO.File]::Create($to)
        try { [Console]::CursorVisible = $false } catch {}
        try {
            $buf = New-Object byte[] 81920
            $got = 0L; $i = 0
            $clock = [Diagnostics.Stopwatch]::StartNew()
            if ($fancy) { Show-Progress 0 $total 0 }
            while (($n = $in.Read($buf, 0, $buf.Length)) -gt 0) {
                $out.Write($buf, 0, $n)
                $got += $n
                if ($fancy -and $clock.ElapsedMilliseconds -ge 100) { $clock.Restart(); $i++; Show-Progress $got $total $i }
            }
        } finally {
            $out.Dispose(); $in.Dispose()
            if ($fancy) { Write-Host ("`r" + (' ' * 72) + "`r") -NoNewline }
            try { [Console]::CursorVisible = $true } catch {}
        }
    } finally {
        $client.Dispose()
    }
}

Write-Host ''
Write-Host "  $note clumyap" -ForegroundColor Magenta -NoNewline
Write-Host ' installer' -ForegroundColor DarkGray
Write-Host ''
Ok 'Found Windows (x64)'

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("clumyap-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Save-Download $url "$tmp\clumyap.zip"
    Ok 'Downloaded clumyap' ('({0} MB)' -f [int][math]::Floor((Get-Item "$tmp\clumyap.zip").Length / 1MB))
    Expand-Archive -Path "$tmp\clumyap.zip" -DestinationPath $tmp -Force
    if (-not (Test-Path "$tmp\clumyap\clumyap.exe")) { throw "clumyap: the download is not a clumyap archive" }

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
Ok "Installed $version" "-> $dir"

$newTerminal = $false
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (($userPath -split ';') -contains $dir)) {
    $newPath = if ($userPath) { "$userPath;$dir" } else { $dir }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
    Ok 'Added clumyap to your PATH'
    $newTerminal = $true
}
$env:Path = "$env:Path;$dir"

Write-Host ''
if ($newTerminal) { Write-Host '  Open a new terminal, then:' }
Write-Host '  Get started:  ' -NoNewline
Write-Host 'clumyap start' -ForegroundColor Magenta
Write-Host '  The first run downloads the voice model (about 350 MB).' -ForegroundColor DarkGray
Write-Host ''
