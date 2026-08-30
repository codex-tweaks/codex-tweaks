<#
.SYNOPSIS
    Verifies the notification area icon lifecycle of the running Codex Tweaks frontend.

.DESCRIPTION
    Enumerates the shell records behind the notification area (promoted area and
    overflow flyout) through the tray ToolbarWindow32 controls and asserts that the
    running frontend owns exactly one of them. The scenarios cover the reported
    regression: the shell dropping the record while the frontend keeps running, a
    Codex restart, two consecutive Codex restarts, a simulated and a real taskbar
    restart, a left click plus window restore after a re-registration and an
    explicit quit.

    The record loss is simulated with Shell_NotifyIcon(NIM_DELETE) for the icon GUID
    the frontend registers, which is how the shell itself drops a record. Deleting
    the tray toolbar button instead leaves the shell with an icon it still believes
    in, and every later NIM_ADD for that GUID fails until Explorer restarts.

    TaskbarCreated is posted to the frontend message window only, so the other
    notification area applications are left untouched. -IncludeExplorerRestart
    restarts Explorer for real and therefore affects the whole shell.

.EXAMPLE
    pwsh -File scripts/verify-windows-tray.ps1

.EXAMPLE
    pwsh -File scripts/verify-windows-tray.ps1 -IncludeShellRecordLoss -IncludeCodexRestart

.EXAMPLE
    pwsh -File scripts/verify-windows-tray.ps1 -IncludeExplorerRestart -IncludeQuit
#>
[CmdletBinding()]
param(
    [string]$ProcessName = 'CodexTweaks.Windows',
    [string]$CodexProcessName = 'ChatGPT',
    [string]$FrontendLogPath = (Join-Path $env:LOCALAPPDATA 'Codex Tweaks\Logs\windows-frontend.log'),
    [int]$SettleMilliseconds = 2500,
    [switch]$IncludeShellRecordLoss,
    [switch]$IncludeCodexRestart,
    [switch]$IncludeExplorerRestart,
    [switch]$IncludeQuit
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

Add-Type -Namespace CodexTweaksTray -Name Native -MemberDefinition @'
[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
public static extern IntPtr FindWindow(string className, string windowName);
[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
public static extern IntPtr FindWindowEx(IntPtr parent, IntPtr after, string className, string windowName);
[DllImport("user32.dll")]
public static extern bool EnumChildWindows(IntPtr parent, EnumWindowsProc callback, IntPtr parameter);
[DllImport("user32.dll", CharSet = CharSet.Unicode)]
public static extern int GetClassName(IntPtr window, System.Text.StringBuilder buffer, int maximumCount);
[DllImport("user32.dll")]
public static extern IntPtr SendMessage(IntPtr window, uint message, IntPtr wParam, IntPtr lParam);
[DllImport("user32.dll", SetLastError = true)]
public static extern bool PostMessage(IntPtr window, uint message, IntPtr wParam, IntPtr lParam);
[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
public static extern uint RegisterWindowMessage(string message);
[DllImport("user32.dll")]
public static extern uint GetWindowThreadProcessId(IntPtr window, out uint processId);
[DllImport("user32.dll")]
public static extern bool IsWindow(IntPtr window);
[DllImport("user32.dll", SetLastError = true)]
public static extern IntPtr CopyIcon(IntPtr icon);
[DllImport("user32.dll", SetLastError = true)]
public static extern bool DestroyIcon(IntPtr icon);
[DllImport("kernel32.dll", SetLastError = true)]
public static extern IntPtr OpenProcess(uint access, bool inheritHandle, uint processId);
[DllImport("kernel32.dll", SetLastError = true)]
public static extern bool CloseHandle(IntPtr handle);
[DllImport("kernel32.dll", SetLastError = true)]
public static extern IntPtr VirtualAllocEx(IntPtr process, IntPtr address, UIntPtr size, uint allocationType, uint protection);
[DllImport("kernel32.dll", SetLastError = true)]
public static extern bool VirtualFreeEx(IntPtr process, IntPtr address, UIntPtr size, uint freeType);
[DllImport("kernel32.dll", SetLastError = true)]
public static extern bool ReadProcessMemory(IntPtr process, IntPtr address, byte[] buffer, UIntPtr size, out UIntPtr read);
public delegate bool EnumWindowsProc(IntPtr window, IntPtr parameter);
[StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
public struct NotifyIconData
{
    public uint cbSize;
    public IntPtr hWnd;
    public uint uID;
    public uint uFlags;
    public uint uCallbackMessage;
    public IntPtr hIcon;
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 128)] public string szTip;
    public uint dwState;
    public uint dwStateMask;
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 256)] public string szInfo;
    public uint uVersion;
    [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 64)] public string szInfoTitle;
    public uint dwInfoFlags;
    public Guid guidItem;
    public IntPtr hBalloonIcon;
}
[DllImport("shell32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
public static extern bool Shell_NotifyIcon(uint message, ref NotifyIconData data);
'@

$TB_GETBUTTON = 0x0417
$TB_BUTTONCOUNT = 0x0418
$NIM_DELETE = 0x00000002
$NIF_GUID = 0x00000020
$TBSTATE_HIDDEN = 0x08
$WM_CLOSE = 0x0010
$WM_LBUTTONUP = 0x0202
# H.NotifyIcon registers every icon with this callback message.
$TRAY_CALLBACK_MESSAGE = 0x0400
# VM_OPERATION | VM_READ | VM_WRITE | QUERY_INFORMATION
$PROCESS_ACCESS = 0x0438
$script:collectedToolbars = New-Object System.Collections.ArrayList

function Write-Step([string]$Message) {
    Write-Host "[tray-verify] $Message"
}

function Get-NotificationAreaToolbars {
    $callback = [CodexTweaksTray.Native+EnumWindowsProc] {
        param([IntPtr]$window, [IntPtr]$parameter)
        $className = New-Object System.Text.StringBuilder 256
        [void][CodexTweaksTray.Native]::GetClassName($window, $className, 256)
        if ($className.ToString() -eq 'ToolbarWindow32') {
            [void]$script:collectedToolbars.Add($window)
        }
        return $true
    }

    $toolbars = New-Object System.Collections.ArrayList
    $tray = [CodexTweaksTray.Native]::FindWindow('Shell_TrayWnd', $null)
    if ($tray -eq [IntPtr]::Zero) {
        throw 'Shell_TrayWnd was not found; this shell exposes no notification area.'
    }
    $notify = [CodexTweaksTray.Native]::FindWindowEx($tray, [IntPtr]::Zero, 'TrayNotifyWnd', $null)
    if ($notify -eq [IntPtr]::Zero) {
        throw 'TrayNotifyWnd was not found.'
    }

    $script:collectedToolbars.Clear()
    [void][CodexTweaksTray.Native]::EnumChildWindows($notify, $callback, [IntPtr]::Zero)
    foreach ($handle in $script:collectedToolbars) {
        [void]$toolbars.Add([pscustomobject]@{ Placement = 'promoted'; Hwnd = $handle })
    }

    $overflow = [CodexTweaksTray.Native]::FindWindow('NotifyIconOverflowWindow', $null)
    if ($overflow -ne [IntPtr]::Zero) {
        $script:collectedToolbars.Clear()
        [void][CodexTweaksTray.Native]::EnumChildWindows($overflow, $callback, [IntPtr]::Zero)
        foreach ($handle in $script:collectedToolbars) {
            [void]$toolbars.Add([pscustomobject]@{ Placement = 'overflow'; Hwnd = $handle })
        }
    }

    return $toolbars
}

function Get-TrayRecords {
    $records = New-Object System.Collections.ArrayList
    foreach ($toolbar in Get-NotificationAreaToolbars) {
        $buttonCount = [int][CodexTweaksTray.Native]::SendMessage($toolbar.Hwnd, $TB_BUTTONCOUNT, [IntPtr]::Zero, [IntPtr]::Zero)
        if ($buttonCount -le 0) {
            continue
        }

        $shellProcessId = [uint32]0
        [void][CodexTweaksTray.Native]::GetWindowThreadProcessId($toolbar.Hwnd, [ref]$shellProcessId)
        $shellProcess = [CodexTweaksTray.Native]::OpenProcess($PROCESS_ACCESS, $false, $shellProcessId)
        if ($shellProcess -eq [IntPtr]::Zero) {
            throw "Cannot open shell process $shellProcessId to read the notification area."
        }
        $remote = [CodexTweaksTray.Native]::VirtualAllocEx($shellProcess, [IntPtr]::Zero, [UIntPtr]::new(4096), 0x1000, 0x04)
        if ($remote -eq [IntPtr]::Zero) {
            [void][CodexTweaksTray.Native]::CloseHandle($shellProcess)
            throw "Cannot allocate a scratch buffer in shell process $shellProcessId."
        }

        try {
            for ($index = 0; $index -lt $buttonCount; $index++) {
                if ([CodexTweaksTray.Native]::SendMessage($toolbar.Hwnd, $TB_GETBUTTON, [IntPtr]$index, $remote) -eq [IntPtr]::Zero) {
                    continue
                }
                $read = [UIntPtr]::Zero
                $button = New-Object byte[] 32
                if (-not [CodexTweaksTray.Native]::ReadProcessMemory($shellProcess, $remote, $button, [UIntPtr]::new(32), [ref]$read)) {
                    continue
                }
                $state = $button[8]
                # TBBUTTON.dwData points at the undocumented TRAYDATA the shell keeps per icon.
                $trayData = [BitConverter]::ToInt64($button, 16)
                if ($trayData -eq 0) {
                    continue
                }
                $payload = New-Object byte[] 40
                if (-not [CodexTweaksTray.Native]::ReadProcessMemory($shellProcess, [IntPtr]::new($trayData), $payload, [UIntPtr]::new(40), [ref]$read)) {
                    continue
                }

                $ownerWindow = [IntPtr][BitConverter]::ToInt64($payload, 0)
                $ownerProcessId = [uint32]0
                if ([CodexTweaksTray.Native]::IsWindow($ownerWindow)) {
                    [void][CodexTweaksTray.Native]::GetWindowThreadProcessId($ownerWindow, [ref]$ownerProcessId)
                }
                $iconHandle = [IntPtr][BitConverter]::ToInt64($payload, 24)
                $iconIsValid = $false
                if ($iconHandle -ne [IntPtr]::Zero) {
                    $iconCopy = [CodexTweaksTray.Native]::CopyIcon($iconHandle)
                    if ($iconCopy -ne [IntPtr]::Zero) {
                        $iconIsValid = $true
                        [void][CodexTweaksTray.Native]::DestroyIcon($iconCopy)
                    }
                }

                [void]$records.Add([pscustomobject]@{
                    Placement = $toolbar.Placement
                    Toolbar = $toolbar.Hwnd
                    Index = $index
                    OwnerWindow = $ownerWindow
                    OwnerProcessId = [int]$ownerProcessId
                    Uid = [BitConverter]::ToUInt32($payload, 8)
                    CallbackMessage = [BitConverter]::ToUInt32($payload, 12)
                    IconHandle = $iconHandle
                    IconIsValid = $iconIsValid
                    IsHidden = ($state -band $TBSTATE_HIDDEN) -ne 0
                })
            }
        }
        finally {
            [void][CodexTweaksTray.Native]::VirtualFreeEx($shellProcess, $remote, [UIntPtr]::Zero, 0x8000)
            [void][CodexTweaksTray.Native]::CloseHandle($shellProcess)
        }
    }

    return $records
}

function Get-FrontendProcess {
    $processes = @(Get-Process -Name $ProcessName -ErrorAction SilentlyContinue)
    if ($processes.Count -eq 0) {
        throw "$ProcessName is not running; start Codex Tweaks before running this script."
    }
    if ($processes.Count -gt 1) {
        throw "$ProcessName is running $($processes.Count) times; the frontend must stay a single instance."
    }
    return $processes[0]
}

function Get-FrontendTrayRecords([int]$FrontendProcessId) {
    return @(Get-TrayRecords | Where-Object { $_.OwnerProcessId -eq $FrontendProcessId })
}

function Assert-SingleTrayRecord([int]$FrontendProcessId, [string]$Scenario, [int]$TimeoutMilliseconds = 8000) {
    $deadline = [datetime]::UtcNow.AddMilliseconds($TimeoutMilliseconds)
    while ($true) {
        $records = @(Get-FrontendTrayRecords $FrontendProcessId)
        if ($records.Count -eq 1) {
            break
        }
        if ([datetime]::UtcNow -ge $deadline) {
            throw "$Scenario expected exactly one notification area record for pid $FrontendProcessId but found $($records.Count)."
        }
        Start-Sleep -Milliseconds 250
    }

    $record = $records[0]
    if (-not $record.IconIsValid) {
        throw "$Scenario left a notification area record whose icon handle is no longer valid."
    }
    if ($record.CallbackMessage -ne $TRAY_CALLBACK_MESSAGE) {
        throw "$Scenario left a notification area record with callback message 0x$('{0:X}' -f $record.CallbackMessage)."
    }
    Write-Step "$Scenario -> one record: placement=$($record.Placement) window=$($record.OwnerWindow) uid=$($record.Uid) hidden=$($record.IsHidden)"
    return $record
}

function Get-LogOffset {
    if (-not (Test-Path -LiteralPath $FrontendLogPath -PathType Leaf)) {
        return [int64]0
    }
    return [int64](Get-Item -LiteralPath $FrontendLogPath).Length
}

function Read-LogTail([int64]$Offset) {
    if (-not (Test-Path -LiteralPath $FrontendLogPath -PathType Leaf)) {
        return ''
    }
    # The frontend keeps the log open, so share read and write access with it.
    $stream = [System.IO.File]::Open($FrontendLogPath, 'Open', 'Read', 'ReadWrite')
    try {
        if ($stream.Length -le $Offset) {
            return ''
        }
        [void]$stream.Seek($Offset, 'Begin')
        $reader = New-Object System.IO.StreamReader $stream
        return $reader.ReadToEnd()
    }
    finally {
        $stream.Dispose()
    }
}

function Wait-ForLogText([int64]$Offset, [string]$Text, [int]$TimeoutMilliseconds) {
    $deadline = [datetime]::UtcNow.AddMilliseconds($TimeoutMilliseconds)
    while ($true) {
        if ((Read-LogTail $Offset).Contains($Text)) {
            return $true
        }
        if ([datetime]::UtcNow -ge $deadline) {
            return $false
        }
        Start-Sleep -Milliseconds 250
    }
}

function Assert-LogText([int64]$Offset, [string]$Text, [string]$Scenario, [int]$TimeoutMilliseconds = 8000) {
    if (-not (Wait-ForLogText $Offset $Text $TimeoutMilliseconds)) {
        throw "$Scenario did not append '$Text' to $FrontendLogPath."
    }
    Write-Step "$Scenario -> log says '$Text'"
}

function Show-OptionalLogText([int64]$Offset, [string]$Text) {
    if ((Read-LogTail $Offset).Contains($Text)) {
        Write-Step "log says '$Text'"
    }
    else {
        Write-Step "log stayed silent about '$Text' (only a build with the tray re-registration fix writes it)"
    }
}

function Send-TrayLeftClick([IntPtr]$MessageWindow) {
    if (-not [CodexTweaksTray.Native]::PostMessage($MessageWindow, $TRAY_CALLBACK_MESSAGE, [IntPtr]::Zero, [IntPtr]$WM_LBUTTONUP)) {
        throw "PostMessage(tray left click) failed with Win32 error $([Runtime.InteropServices.Marshal]::GetLastWin32Error())."
    }
}

function Hide-MainWindow([int]$FrontendProcessId, [int]$TimeoutMilliseconds = 8000) {
    $deadline = [datetime]::UtcNow.AddMilliseconds($TimeoutMilliseconds)
    while ($true) {
        $mainWindow = (Get-Process -Id $FrontendProcessId).MainWindowHandle
        if ($mainWindow -ne [IntPtr]::Zero) {
            break
        }
        if ([datetime]::UtcNow -ge $deadline) {
            throw 'The frontend exposes no main window.'
        }
        Start-Sleep -Milliseconds 250
    }
    if (-not [CodexTweaksTray.Native]::PostMessage($mainWindow, $WM_CLOSE, [IntPtr]::Zero, [IntPtr]::Zero)) {
        throw "PostMessage(WM_CLOSE) failed with Win32 error $([Runtime.InteropServices.Marshal]::GetLastWin32Error())."
    }
    Start-Sleep -Milliseconds $SettleMilliseconds
}

function Get-FrontendIconGuid([string]$ExecutablePath) {
    # H.NotifyIcon identifies the icon by a GUID derived from the executable path:
    # Guid(SHA256("<path>_")[0..15]). Reproducing it here is what lets the shell delete the very
    # record the frontend registered.
    $algorithm = [System.Security.Cryptography.SHA256]::Create()
    try {
        $hash = $algorithm.ComputeHash([System.Text.Encoding]::UTF8.GetBytes($ExecutablePath + '_'))
    }
    finally {
        $algorithm.Dispose()
    }
    return [Guid]::new([byte[]]($hash[0..15]))
}

function Remove-ShellTrayRecord([Guid]$IconGuid) {
    # NIM_DELETE with NIF_GUID is how the shell itself drops a record, so this leaves the frontend
    # in the reported state: it still believes it owns an icon the shell no longer knows about.
    $data = New-Object CodexTweaksTray.Native+NotifyIconData
    $data.cbSize = [uint32][System.Runtime.InteropServices.Marshal]::SizeOf([type]'CodexTweaksTray.Native+NotifyIconData')
    $data.uFlags = $NIF_GUID
    $data.guidItem = $IconGuid
    $data.szTip = ''
    $data.szInfo = ''
    $data.szInfoTitle = ''
    if (-not [CodexTweaksTray.Native]::Shell_NotifyIcon($NIM_DELETE, [ref]$data)) {
        throw "Shell_NotifyIcon(NIM_DELETE) failed with Win32 error $([Runtime.InteropServices.Marshal]::GetLastWin32Error())."
    }
}

$frontend = Get-FrontendProcess
Write-Step "frontend pid=$($frontend.Id) started=$($frontend.StartTime.ToString('s'))"
Write-Step "frontend log $FrontendLogPath"
$baseline = Assert-SingleTrayRecord $frontend.Id 'baseline'

if ($IncludeShellRecordLoss) {
    # The reported regression: the frontend keeps running while the shell forgets its record, so the
    # icon stays gone until the frontend notices and adds it again.
    $logOffset = Get-LogOffset
    $iconGuid = Get-FrontendIconGuid $frontend.Path
    Write-Step "asking the shell to drop the record of icon guid $iconGuid"
    Remove-ShellTrayRecord $iconGuid
    Start-Sleep -Milliseconds $SettleMilliseconds
    $dropped = @(Get-FrontendTrayRecords $frontend.Id)
    if ($dropped.Count -ne 0) {
        throw "The notification area still lists $($dropped.Count) record(s) for pid $($frontend.Id); the loss was not simulated."
    }
    Write-Step 'shell record dropped -> the notification area lists nothing for the frontend'

    # Refresh() runs after every tray command and is where the frontend asks the shell about its
    # record, so a left click is enough to make it notice the loss.
    Send-TrayLeftClick $baseline.OwnerWindow
    $recovered = Assert-SingleTrayRecord $frontend.Id 'lost shell record'
    if ($recovered.OwnerWindow -ne $baseline.OwnerWindow) {
        throw "The re-added record belongs to window $($recovered.OwnerWindow) instead of $($baseline.OwnerWindow)."
    }
    Assert-LogText $logOffset 'Notification area icon re-added.' 'lost shell record'
    Hide-MainWindow $frontend.Id

    # A record the shell still knows must be left alone: no second icon, no re-registration.
    $logOffset = Get-LogOffset
    foreach ($round in 1, 2) {
        Send-TrayLeftClick $recovered.OwnerWindow
        Start-Sleep -Milliseconds $SettleMilliseconds
        $unchanged = Assert-SingleTrayRecord $frontend.Id "healthy record refresh $round"
        if ($unchanged.Uid -ne $recovered.Uid -or $unchanged.OwnerWindow -ne $recovered.OwnerWindow) {
            throw "Refresh $round replaced the notification area record."
        }
        Hide-MainWindow $frontend.Id
    }
    if ((Read-LogTail $logOffset).Contains('Re-adding the notification area icon')) {
        throw 'The frontend re-added a notification area record that the shell still knew.'
    }
    Write-Step 'healthy record survived two refreshes without a re-registration'
    $baseline = $recovered
}

function Get-CodexMainProcessInfo {
    $processes = @(Get-Process -Name $CodexProcessName -ErrorAction SilentlyContinue)
    if ($processes.Count -eq 0) {
        throw "$CodexProcessName is not running; the Codex restart scenario needs a running Codex app."
    }
    foreach ($process in ($processes | Sort-Object StartTime)) {
        $commandLine = (Get-CimInstance Win32_Process -Filter "ProcessId = $($process.Id)").CommandLine
        if ($null -ne $commandLine -and -not $commandLine.Contains('--type=')) {
            return [pscustomobject]@{ Id = $process.Id; CommandLine = $commandLine }
        }
    }
    throw "Cannot tell which $CodexProcessName process is the main one."
}

function Start-CodexFromCommandLine([string]$CommandLine) {
    if ($CommandLine.StartsWith('"')) {
        $end = $CommandLine.IndexOf('"', 1)
        $executable = $CommandLine.Substring(1, $end - 1)
        $arguments = $CommandLine.Substring($end + 1).Trim()
    }
    else {
        $end = $CommandLine.IndexOf(' ')
        if ($end -lt 0) {
            $executable = $CommandLine
            $arguments = ''
        }
        else {
            $executable = $CommandLine.Substring(0, $end)
            $arguments = $CommandLine.Substring($end + 1).Trim()
        }
    }

    Write-Step "starting $executable $arguments"
    if ($arguments.Length -eq 0) {
        Start-Process -FilePath $executable | Out-Null
    }
    else {
        Start-Process -FilePath $executable -ArgumentList $arguments | Out-Null
    }
}

if ($IncludeCodexRestart) {
    # Two rounds: the reported regression only showed up on the second Codex restart.
    foreach ($round in 1, 2) {
        $codex = Get-CodexMainProcessInfo
        $logOffset = Get-LogOffset
        Write-Step "round $round : stopping $CodexProcessName"
        Get-Process -Name $CodexProcessName -ErrorAction SilentlyContinue | Stop-Process -Force
        Start-Sleep -Milliseconds $SettleMilliseconds
        $afterExit = Assert-SingleTrayRecord $frontend.Id "codex exit (round $round)"
        if ($afterExit.Uid -ne $baseline.Uid -or $afterExit.OwnerWindow -ne $baseline.OwnerWindow) {
            throw "A Codex exit replaced the notification area record (uid $($baseline.Uid) -> $($afterExit.Uid))."
        }

        Start-CodexFromCommandLine $codex.CommandLine
        $deadline = [datetime]::UtcNow.AddSeconds(60)
        while ([datetime]::UtcNow -lt $deadline) {
            if (@(Get-Process -Name $CodexProcessName -ErrorAction SilentlyContinue).Count -gt 0) {
                break
            }
            Start-Sleep -Milliseconds 500
        }
        if (@(Get-Process -Name $CodexProcessName -ErrorAction SilentlyContinue).Count -eq 0) {
            throw "$CodexProcessName did not come back up in round $round."
        }
        Start-Sleep -Milliseconds $SettleMilliseconds

        if ($null -eq (Get-Process -Id $frontend.Id -ErrorAction SilentlyContinue)) {
            throw "The frontend exited during round $round; the notification area icon went with it."
        }
        [void](Assert-SingleTrayRecord $frontend.Id "codex restart (round $round)")
        Show-OptionalLogText $logOffset 'Re-adding the notification area icon'
    }
}

$taskbarCreated = [CodexTweaksTray.Native]::RegisterWindowMessage('TaskbarCreated')
if ($taskbarCreated -eq 0) {
    throw 'RegisterWindowMessage(TaskbarCreated) failed.'
}

# Two rounds: the message also arrives while the record is still alive, and adding the icon again
# must not lose it.
$afterTaskbarCreated = $baseline
foreach ($round in 1, 2) {
    $logOffset = Get-LogOffset
    Write-Step "round $round : posting TaskbarCreated (0x$('{0:X}' -f $taskbarCreated)) to the frontend message window $($afterTaskbarCreated.OwnerWindow)"
    if (-not [CodexTweaksTray.Native]::PostMessage($afterTaskbarCreated.OwnerWindow, $taskbarCreated, [IntPtr]::Zero, [IntPtr]::Zero)) {
        throw "PostMessage(TaskbarCreated) failed with Win32 error $([Runtime.InteropServices.Marshal]::GetLastWin32Error())."
    }
    Start-Sleep -Milliseconds $SettleMilliseconds
    $afterTaskbarCreated = Assert-SingleTrayRecord $frontend.Id "simulated taskbar restart (round $round)"
    Assert-LogText $logOffset 'Taskbar was recreated' "simulated taskbar restart (round $round)"
    if ((Read-LogTail $logOffset).Contains('Re-adding the notification area icon failed')) {
        throw "Round $round left the frontend unable to add its notification area icon."
    }
}

if ($IncludeExplorerRestart) {
    # The real thing: Explorer forgets every record and broadcasts TaskbarCreated once it is back.
    $logOffset = Get-LogOffset
    Write-Step 'restarting explorer.exe'
    Get-Process -Name explorer -ErrorAction SilentlyContinue | Stop-Process -Force
    $deadline = [datetime]::UtcNow.AddSeconds(60)
    while ([datetime]::UtcNow -lt $deadline) {
        Start-Sleep -Milliseconds 500
        if ([CodexTweaksTray.Native]::FindWindow('Shell_TrayWnd', $null) -ne [IntPtr]::Zero) {
            break
        }
    }
    if ([CodexTweaksTray.Native]::FindWindow('Shell_TrayWnd', $null) -eq [IntPtr]::Zero) {
        throw 'The shell did not come back after explorer.exe was stopped.'
    }
    Write-Step 'the shell is back'
    $afterTaskbarCreated = Assert-SingleTrayRecord $frontend.Id 'explorer restart' 30000
    Assert-LogText $logOffset 'Taskbar was recreated' 'explorer restart'
    if ((Read-LogTail $logOffset).Contains('Re-adding the notification area icon failed')) {
        throw 'The frontend could not add its notification area icon after the Explorer restart.'
    }
}

$logOffset = Get-LogOffset
Write-Step 'posting a left click to the notification area record'
Send-TrayLeftClick $afterTaskbarCreated.OwnerWindow
Assert-LogText $logOffset 'Main window shown from the notification area.' 'left click after a taskbar restart'

Write-Step 'closing the main window so it goes back to the notification area'
Hide-MainWindow $frontend.Id
[void](Assert-SingleTrayRecord $frontend.Id 'after closing the main window')

if ($IncludeQuit) {
    Write-Step 'choose "Quit" in the notification area menu now; waiting up to 180 seconds for the frontend to exit'
    $deadline = [datetime]::UtcNow.AddSeconds(180)
    while (-not $frontend.HasExited -and [datetime]::UtcNow -lt $deadline) {
        Start-Sleep -Milliseconds 500
    }
    if (-not $frontend.HasExited) {
        throw 'The frontend is still running; the explicit quit scenario was not exercised.'
    }

    Start-Sleep -Milliseconds $SettleMilliseconds
    $leftBehind = @(Get-FrontendTrayRecords $frontend.Id)
    if ($leftBehind.Count -ne 0) {
        throw "The explicit quit left $($leftBehind.Count) notification area record(s) behind."
    }
    Write-Step 'explicit quit -> the record is gone'

    Start-Sleep -Seconds 10
    $reappeared = @(Get-FrontendTrayRecords $frontend.Id)
    if ($reappeared.Count -ne 0) {
        throw 'A notification area record came back after the explicit quit.'
    }
    Write-Step 'explicit quit -> nothing re-registered afterwards'
}

Write-Step 'all requested notification area scenarios passed'
