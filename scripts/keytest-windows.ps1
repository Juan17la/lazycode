# M0-07 in Windows Terminal, using simulated key presses. It needs Windows Terminal
# installed and a desktop session, or keytest has to be run by hand.
param([Parameter(Mandatory)][string]$Bin)
$ErrorActionPreference = 'Stop'

$wt = Get-Command wt.exe -ErrorAction SilentlyContinue
if (-not $wt) { throw 'Windows Terminal is not installed here' }
"Windows Terminal: $($wt.Source)"

Add-Type -AssemblyName System.Windows.Forms
$report = Join-Path $env:TEMP 'keytest-report.txt'
Remove-Item $report -ErrorAction SilentlyContinue
Start-Process wt.exe -ArgumentList @('--', $Bin, '--report', $report)
Start-Sleep -Seconds 8

$shell = New-Object -ComObject WScript.Shell
[void]$shell.AppActivate('keytest')
Start-Sleep -Seconds 1

# SendKeys: ^ is Ctrl and % is Alt.
$keys = '^\', '^b', '^c', '^d', '^f', '^n', '^o', '^p', '^s', '^v', '^w', '^x', '^y', '^z',
'{F2}', '{F12}', '{ESC}', '%d', '%o', '%p', '%f', '%h', '%s', '%,', '%.', '^{F2}', 'q'
foreach ($k in $keys) {
    [System.Windows.Forms.SendKeys]::SendWait($k)
    Start-Sleep -Milliseconds 250
}

for ($i = 0; $i -lt 15 -and -not (Test-Path $report); $i++) { Start-Sleep -Seconds 1 }
if (-not (Test-Path $report)) { throw 'No report, so keytest never saw q' }
Get-Content $report
if (Select-String -Path $report -Pattern 'MISSING' -Quiet) { throw 'Some keys did not arrive' }
