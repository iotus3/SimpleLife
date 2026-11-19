# Real-machine verification of the single-instance guard: build once, then behave like
# a user who has the program in the status area and clicks the icon again.
#
#   just instance-check
#
# It runs two copies in two folders on purpose. That is how the bug looked in the
# field: `dist/` keeps a dated folder per build and `build/simplelife.exe` sits beside
# it, so "the icon" and "the program that is running" are often different files. The
# guard must refuse the second start anyway and bring the running window back.
#
# Messages are ASCII on purpose: Windows PowerShell 5.1 reads a .ps1 without a BOM in
# the ANSI code page, so a UTF-8 file with Chinese text does not even parse.
param(
    [Parameter(Mandatory = $true)][string]$Exe,
    [string]$WorkDir = 'build\instance-check'
)

$ErrorActionPreference = 'Stop'

Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class Win {
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int cmd);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    public static extern int GetClassName(IntPtr h, StringBuilder name, int max);
    [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
    [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr p);
    public delegate bool EnumProc(IntPtr h, IntPtr p);
}
"@

function Get-Windows {
    $script:found = New-Object System.Collections.ArrayList
    $callback = [Win+EnumProc]{
        param($h, $p)
        $sb = New-Object System.Text.StringBuilder 128
        [void][Win]::GetClassName($h, $sb, 128)
        if ($sb.ToString() -eq 'VivaNativeWindow') {
            $owner = 0
            [void][Win]::GetWindowThreadProcessId($h, [ref]$owner)
            $proc = Get-Process -Id $owner -ErrorAction SilentlyContinue
            [void]$script:found.Add([pscustomobject]@{
                Hwnd    = $h
                Pid     = $owner
                Path    = if ($proc) { $proc.Path } else { '' }
                Visible = [Win]::IsWindowVisible($h)
            })
        }
        return $true
    }
    [void][Win]::EnumWindows($callback, [IntPtr]::Zero)
    return $script:found
}

function Get-WindowVisible([IntPtr]$Hwnd) {
    return [Win]::IsWindowVisible($Hwnd)
}

function Get-CopyWindow([string]$Path) {
    $full = (Get-Item $Path).FullName
    Get-Windows | Where-Object { $_.Path -eq $full } | Select-Object -First 1
}

function Get-CopyProcesses([string]$Root) {
    @(Get-Process -Name 'simplelife' -ErrorAction SilentlyContinue | Where-Object {
            $_.Path -and $_.Path.StartsWith((Get-Item $Root).FullName, 'OrdinalIgnoreCase')
        })
}

function Stop-CopyProcesses([string]$Root) {
    foreach ($p in Get-CopyProcesses $Root) {
        if (-not $p.HasExited) { [void]$p.Kill(); $p.WaitForExit() }
    }
}

$root = Join-Path (Get-Location) $WorkDir
if (Test-Path $root) { Remove-Item -Recurse -Force $root }
$dirA = Join-Path $root 'a'
$dirB = Join-Path $root 'b'
New-Item -ItemType Directory -Path $dirA, $dirB -Force | Out-Null
$exeA = Join-Path $dirA 'simplelife.exe'
$exeB = Join-Path $dirB 'simplelife.exe'
Copy-Item $Exe $exeA
Copy-Item $Exe $exeB

# Another SimpleLife (the developer's own) holds the claim and would make every check
# below meaningless, so say so instead of reporting a failure of the wrong thing.
$others = @(Get-Process -Name 'simplelife' -ErrorAction SilentlyContinue | Where-Object {
        -not ($_.Path -and $_.Path.StartsWith((Get-Item $root).FullName, 'OrdinalIgnoreCase'))
    })
if ($others.Count -gt 0) {
    Write-Host "SKIP a SimpleLife is already running (pid $($others[0].Id)); quit it and run this again"
    Remove-Item -Recurse -Force $root
    exit 2
}

Write-Host "[1] start copy A from $dirA"
$procA = Start-Process -FilePath $exeA -PassThru
Start-Sleep -Seconds 5
$count = (Get-CopyProcesses $root).Count
Write-Host "    processes = $count (want 1)"
if ($count -ne 1) { Stop-CopyProcesses $root; throw "the first start did not leave exactly one process" }

$windowA = Get-CopyWindow $exeA
Write-Host "    A window visible = $($windowA.Visible) (want True)"
if (-not $windowA -or -not $windowA.Visible) { Stop-CopyProcesses $root; throw "the first start drew no visible window" }

Write-Host "[2] hide A the way close-to-tray does"
[void][Win]::ShowWindow($windowA.Hwnd, 0)
Start-Sleep -Milliseconds 400
$visible = (Get-WindowVisible $windowA.Hwnd)
Write-Host "    visible = $visible (want False)"
if ($visible) { Stop-CopyProcesses $root; throw "the window could not be hidden; the rest of the check proves nothing" }

Write-Host "[3] start copy B from another folder, the way a stale shortcut would"
$procB = Start-Process -FilePath $exeB -PassThru
$exited = $procB.WaitForExit(8000)
Write-Host "    B exited within 8s = $exited (want True)"
if (-not $exited) {
    $bWindow = Get-CopyWindow $exeB
    Stop-CopyProcesses $root
    throw "the second copy kept running (window: $($bWindow.Hwnd)): the guard failed"
}
Write-Host "    B exit code = $($procB.ExitCode) (want 0)"
if ($procB.ExitCode -ne 0) { Stop-CopyProcesses $root; throw "the second copy ended with a non-zero code" }

Start-Sleep -Milliseconds 900
$count = (Get-CopyProcesses $root).Count
Write-Host "    processes = $count (want 1: no second window, no second tray icon)"
if ($count -ne 1) { Stop-CopyProcesses $root; throw "the second copy left its own process behind" }

$visible = (Get-WindowVisible $windowA.Hwnd)
Write-Host "    A window visible = $visible (want True: the second start woke it)"
if (-not $visible) { Stop-CopyProcesses $root; throw "the second start did not bring the window back" }

Write-Host "[4] start copy A again: the same copy must also be refused"
$procA2 = Start-Process -FilePath $exeA -PassThru
$again = $procA2.WaitForExit(8000)
Write-Host "    second A exited within 8s = $again (want True)"
if (-not $again) { Stop-CopyProcesses $root; throw "clicking the running copy started a second window" }

Write-Host "[5] cleanup"
Stop-CopyProcesses $root
Start-Sleep -Milliseconds 500
Write-Host "    processes = $((Get-CopyProcesses $root).Count) (want 0)"
Remove-Item -Recurse -Force $root

Write-Host "OK single instance guard verified"
