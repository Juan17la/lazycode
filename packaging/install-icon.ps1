# Creates the app icon on Windows (Q-50): a Start menu shortcut. A console program
# opens in the system's default terminal. REL-1 calls this from the install script.
#
# Usage: install-icon.ps1 C:\absolute\path\to\lazycode.exe
param([Parameter(Mandatory)][string]$Bin)
$ErrorActionPreference = 'Stop'

if (-not [IO.Path]::IsPathRooted($Bin)) { throw "The path must be absolute: $Bin" }
if (-not (Test-Path -LiteralPath $Bin -PathType Leaf)) { throw "Not a file: $Bin" }

$path = Join-Path ([Environment]::GetFolderPath('Programs')) 'LazyCode.lnk'
$shortcut = (New-Object -ComObject WScript.Shell).CreateShortcut($path)
$shortcut.TargetPath = $Bin
$shortcut.WorkingDirectory = $env:USERPROFILE
$shortcut.Description = 'A light code editor that runs in your terminal'
$shortcut.Save()
$path
