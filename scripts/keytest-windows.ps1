# M0-07 in Windows Terminal, using simulated key presses. It needs Windows Terminal
# installed and a desktop session, or keytest has to be run by hand.
param([Parameter(Mandatory)][string]$Bin)
$ErrorActionPreference = 'Stop'

$wt = Get-Command wt.exe -ErrorAction SilentlyContinue
if (-not $wt) { throw 'Windows Terminal is not installed here' }
"Windows Terminal: $($wt.Source)"

Add-Type -AssemblyName System.Windows.Forms
Set-Clipboard -Value 'z' # Windows Terminal takes Ctrl+V to paste, which needs something to paste.
$report = Join-Path $env:TEMP 'keytest-report.txt'
Remove-Item $report -ErrorAction SilentlyContinue
Start-Process wt.exe -ArgumentList @('--', $Bin, '--report', $report)
Start-Sleep -Seconds 8

$shell = New-Object -ComObject WScript.Shell
[void]$shell.AppActivate('keytest')
Start-Sleep -Seconds 1

# SendKeys: ^ is Ctrl, % is Alt and + is Shift. The same keys as checks() in cmd/keytest/main.go.
$keys = '^\', '^ ', '^a', '^b', '^c', '^d', '^f', '^n', '^q', '^r', '^o', '^p', '^s', '^v', '^w', '^x', '^y', '^z',
'{F1}', '{F2}', '{F3}', '{F10}', '{F12}', '{ESC}',
'%d', '%p', '%r', '%f', '%g', '%a', '%e', '%t', '%m', '%x', '%z', '%v', '%s', '%k', '%w', '%,', '%.',
'%+p', '%+f', '%+t', '%+a', '%+s', '%+b', '^{F2}', 'q'
foreach ($k in $keys) {
    [System.Windows.Forms.SendKeys]::SendWait($k)
    Start-Sleep -Milliseconds 250
}

for ($i = 0; $i -lt 15 -and -not (Test-Path $report); $i++) { Start-Sleep -Seconds 1 }
if (-not (Test-Path $report)) { throw 'No report, so keytest never saw q' }
Get-Content $report
if (Select-String -Path $report -Pattern 'MISSING' -Quiet) { throw 'Some keys did not arrive' }
