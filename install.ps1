# Installs LazyCode on Windows (64-bit Intel, AMD or ARM).
#
#   curl.exe -fsSL https://raw.githubusercontent.com/Juan17la/lazycode/main/install.ps1 | powershell -NoProfile -Command -
#   irm https://raw.githubusercontent.com/Juan17la/lazycode/main/install.ps1 | iex
#
# It downloads one program from GitHub Releases, checks its SHA-256 against
# checksums.txt, and puts it in %LOCALAPPDATA%\Programs\lazycode. It adds that
# folder to your user PATH (not the machine's), and makes a Start menu shortcut.
# It needs no administrator rights. LAZYCODE_NO_MODIFY_PATH=1 skips the PATH change.
#
# Everything is inside a function, called on the last line, so a download that stops
# half way runs nothing. Errors are thrown, never `exit`, so they can't close your shell.

function Install-LazyCode {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue' # Makes downloads much faster in Windows PowerShell.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    # LAZYCODE_BASE_URL and LAZYCODE_INSTALL_DIR are only for tests.
    $base = if ($env:LAZYCODE_BASE_URL) { $env:LAZYCODE_BASE_URL } else { 'https://github.com/Juan17la/lazycode/releases/latest/download' }
    $dir = if ($env:LAZYCODE_INSTALL_DIR) { $env:LAZYCODE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\lazycode' }

    $cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    # Windows 11 on ARM runs x64 programs, so ARM64 gets the same file.
    if ($cpu -notin 'AMD64', 'ARM64') { throw "Unsupported CPU: $cpu. LazyCode for Windows needs a 64-bit Intel, AMD or ARM CPU." }
    $asset = 'lazycode_windows_amd64.exe'

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading $asset from $base"
        Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')
        $sums = Get-Content (Join-Path $tmp 'checksums.txt')

        # Download a file, and refuse to go on unless its SHA-256 is listed and matches.
        function Get-Verified($name) {
            $file = Join-Path $tmp $name
            Invoke-WebRequest -UseBasicParsing -Uri "$base/$name" -OutFile $file
            $line = $sums | Where-Object { ($_ -split '\s+', 2)[1] -replace '^\*', '' -eq $name } | Select-Object -First 1
            if (-not $line) { throw "checksums.txt has no entry for $name. Nothing was installed." }
            $want = ($line -split '\s+')[0]
            $got = (Get-FileHash -Algorithm SHA256 -LiteralPath $file).Hash
            if ($got -ne $want) { throw "Checksum mismatch for $name (wanted $want, got $got). Nothing was installed." }
            $file
        }
        $exe = Get-Verified $asset
        $icon = Get-Verified 'install-icon.ps1'

        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        $target = Join-Path $dir 'lazycode.exe'
        try { Copy-Item -LiteralPath $exe -Destination $target -Force }
        catch { throw "Couldn't replace $target. If LazyCode is running, quit it and run this again. ($_)" }
        Write-Host "Installed $target"

        # A failed shortcut is only a warning. It runs as a script block because Windows' default
        # execution policy refuses .ps1 files, though not this script, which arrives through iex.
        $makeIcon = [scriptblock]::Create((Get-Content -Raw -LiteralPath $icon))
        try { Write-Host "Created the Start menu shortcut: $(& $makeIcon -Bin $target)" }
        catch { Write-Warning "Couldn't create the Start menu shortcut: $_" }

        if ($env:LAZYCODE_NO_MODIFY_PATH -eq '1') {
            Write-Host "$dir isn't on your PATH. Add it yourself to run lazycode by name."
        } else {
            $user = [Environment]::GetEnvironmentVariable('Path', 'User')
            $parts = @($user -split ';' | Where-Object { $_ })
            if ($parts | Where-Object { $_.TrimEnd('\') -ieq $dir.TrimEnd('\') }) {
                Write-Host "$dir is already on your user PATH."
            } else {
                [Environment]::SetEnvironmentVariable('Path', (($parts + $dir) -join ';'), 'User')
                Write-Host "Added $dir to your user PATH. Open a new terminal to use it."
            }
        }
        Write-Host 'Done. Run: lazycode'
    }
    finally {
        Remove-Item -Recurse -Force -LiteralPath $tmp -ErrorAction SilentlyContinue
    }
}

Install-LazyCode
