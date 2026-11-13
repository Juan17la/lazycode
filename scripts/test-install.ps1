# Tests install.ps1 against a fake release served from this machine. It changes the
# real user PATH and Start menu of whoever runs it (it restores the PATH), so it is
# meant for CI runners and throwaway machines. Run from the repository root.
#
# Usage: .\scripts\test-install.ps1
$ErrorActionPreference = 'Stop'

$root = Resolve-Path "$PSScriptRoot\.."
$work = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
$asset = 'lazycode_windows_amd64.exe'
$tag = 'v0.0.0-installtest'
$oldPath = [Environment]::GetEnvironmentVariable('Path', 'User')

function Pass($m) { "ok: $m" }
function Fail($m) { throw "FAIL: $m" }
function Sha($f) { (Get-FileHash -Algorithm SHA256 -LiteralPath $f).Hash.ToLower() }

try {
    New-Item -ItemType Directory -Path "$work\www\good" | Out-Null
    Push-Location $root
    go build -trimpath -ldflags "-X main.version=$tag" -o "$work\www\good\$asset" .\cmd\lazycode
    Pop-Location
    Copy-Item "$root\packaging\install-icon.ps1" "$work\www\good\"
    Copy-Item "$root\install.ps1" "$work\www\"
    $lines = foreach ($f in $asset, 'install-icon.ps1') { "$(Sha "$work\www\good\$f")  $f" }
    Set-Content -Encoding ascii "$work\www\good\checksums.txt" $lines
    Copy-Item -Recurse "$work\www\good" "$work\www\badsum"
    [IO.File]::AppendAllText("$work\www\badsum\$asset", 'tampered')
    Copy-Item -Recurse "$work\www\good" "$work\www\noentry"
    Set-Content -Encoding ascii "$work\www\noentry\checksums.txt" ($lines | Where-Object { $_ -notmatch [regex]::Escape($asset) })

    # A tiny server, as a background job.
    $port = 18000 + (Get-Random -Maximum 1000)
    $server = Start-Job -ArgumentList "$work\www", $port {
        param($dir, $port)
        $l = [Net.HttpListener]::new(); $l.Prefixes.Add("http://127.0.0.1:$port/"); $l.Start()
        while ($true) {
            $c = $l.GetContext()
            $f = Join-Path $dir ($c.Request.Url.AbsolutePath.TrimStart('/') -replace '/', '\')
            if (Test-Path -LiteralPath $f -PathType Leaf) {
                $b = [IO.File]::ReadAllBytes($f); $c.Response.OutputStream.Write($b, 0, $b.Length)
            } else { $c.Response.StatusCode = 404 }
            $c.Response.Close()
        }
    }
    for ($i = 0; $i -lt 50; $i++) {
        try { Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$port/good/checksums.txt" | Out-Null; break } catch { Start-Sleep -Milliseconds 200 }
    }

    function Install($rel, $dir, $noPath) {
        $env:LAZYCODE_BASE_URL = "http://127.0.0.1:$port/$rel"
        $env:LAZYCODE_INSTALL_DIR = $dir
        $env:LAZYCODE_NO_MODIFY_PATH = $noPath
        # Run the script text through iex, as the README does.
        Invoke-Expression (Get-Content -Raw "$root\install.ps1")
    }
    function Count-OnPath($dir) {
        @([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_.TrimEnd('\') -ieq $dir }).Count
    }

    # 1 and 2. A good install runs, the PATH entry is added once across two runs.
    $d1 = "$work\bin1"
    Install 'good' $d1 ''
    $exe = "$d1\lazycode.exe"
    if (-not (Test-Path $exe)) { Fail 'binary not installed' }
    if ((Sha $exe) -ne (Sha "$work\www\good\$asset")) { Fail 'installed binary differs from the release' }
    if (Select-String -Path "$root\cmd\lazycode\main.go" -Pattern 'main.version' -Quiet) {
        if (-not ((& $exe --version) -match [regex]::Escape($tag))) { Fail "--version doesn't report $tag" }
        Pass "lazycode --version reports $tag"
    } else { 'NOTE: main.go has no version variable yet, so --version was not checked' }
    Pass 'installed, same bytes as the release'
    Install 'good' $d1 ''
    if ((Count-OnPath $d1) -ne 1) { Fail 'PATH entry not added exactly once' }
    Pass 'PATH entry added once across two runs'
    if (-not (Test-Path (Join-Path ([Environment]::GetFolderPath('Programs')) 'LazyCode.lnk'))) { Fail 'no Start menu shortcut' }
    Pass 'Start menu shortcut created'

    # 3 and 4. A wrong checksum and a missing entry are refused, and nothing is installed.
    foreach ($case in @(@('badsum', 'Checksum mismatch'), @('noentry', 'no entry'))) {
        $d = "$work\bin-$($case[0])"
        $failed = $false
        try { Install $case[0] $d '' } catch { $failed = $true; if ("$_" -notmatch $case[1]) { Fail "wrong error for $($case[0]): $_" } }
        if (-not $failed) { Fail "$($case[0]) was accepted" }
        if (Test-Path "$d\lazycode.exe") { Fail "$($case[0]) installed a binary" }
        if ((Count-OnPath $d) -ne 0) { Fail "$($case[0]) changed PATH" }
        Pass "$($case[0]) refused, nothing installed"
    }

    # 5. The opt-out leaves PATH alone.
    $d5 = "$work\bin5"
    Install 'good' $d5 '1'
    if (-not (Test-Path "$d5\lazycode.exe")) { Fail 'opt-out install failed' }
    if ((Count-OnPath $d5) -ne 0) { Fail 'LAZYCODE_NO_MODIFY_PATH ignored' }
    Pass 'LAZYCODE_NO_MODIFY_PATH=1 leaves PATH alone'

    # 6. The README's curl one-liner: curl.exe pipes the script into powershell's stdin.
    $d6 = "$work\bin6"
    $env:LAZYCODE_BASE_URL = "http://127.0.0.1:$port/good"
    $env:LAZYCODE_INSTALL_DIR = $d6
    $env:LAZYCODE_NO_MODIFY_PATH = '1'
    curl.exe -fsSL "http://127.0.0.1:$port/install.ps1" | powershell -NoProfile -Command -
    if ((Sha "$d6\lazycode.exe") -ne (Sha "$work\www\good\$asset")) { Fail 'curl | powershell install failed' }
    Pass 'curl.exe | powershell -Command - installs'
    'PASS'
}
finally {
    [Environment]::SetEnvironmentVariable('Path', $oldPath, 'User')
    Remove-Item Env:LAZYCODE_BASE_URL, Env:LAZYCODE_INSTALL_DIR, Env:LAZYCODE_NO_MODIFY_PATH -ErrorAction SilentlyContinue
    if ($server) { Stop-Job $server; Remove-Job $server }
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
