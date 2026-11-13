# M0-06 on Windows: installs the Start menu shortcut, opens it as the system would,
# and checks that lazycode really started.
param([Parameter(Mandatory)][string]$Bin)
$ErrorActionPreference = 'Stop'

$lnk = & "$PSScriptRoot\..\packaging\install-icon.ps1" -Bin $Bin
"Installed: $lnk"
Start-Process -FilePath $lnk

for ($i = 0; $i -lt 30; $i++) {
    $p = Get-Process lazycode -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($p) { break }
    Start-Sleep -Seconds 1
}
if (-not $p) { throw 'lazycode never started' }
$parent = (Get-CimInstance Win32_Process -Filter "ProcessId=$($p.Id)").ParentProcessId
"lazycode started, pid $($p.Id), parent: $((Get-Process -Id $parent -ErrorAction SilentlyContinue).ProcessName)"
Get-Process | Where-Object { $_.ProcessName -match 'conhost|OpenConsole|WindowsTerminal' } | ForEach-Object { "Terminal host running: $($_.ProcessName)" }
Stop-Process -Id $p.Id
'PASS'
