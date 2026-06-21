Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:RootDir = $PSScriptRoot
$script:FrontendDir = Join-Path $script:RootDir 'frontend'
$script:GoCmd = $null
$script:WailsProcess = $null
$script:CleanupStarted = $false

if (-not $script:RootDir) {
    $script:RootDir = (Get-Location).Path
    $script:FrontendDir = Join-Path $script:RootDir 'frontend'
}

Set-Location $script:RootDir

if (-not $env:WAILS_VERSION) { $env:WAILS_VERSION = 'latest' }
if (-not $env:GOTOOLCHAIN) { $env:GOTOOLCHAIN = 'auto' }
if (-not $env:GOPROXY) { $env:GOPROXY = 'https://goproxy.cn,direct' }
if (-not $env:GOSUMDB) { $env:GOSUMDB = 'sum.golang.google.cn' }

function Log-Start {
    param([Parameter(Mandatory = $true)][string]$Message)
    Write-Host "[start] $Message"
}

function Fail-Start {
    param([Parameter(Mandatory = $true)][string]$Message)
    throw "[start] 错误：$Message"
}

function Require-Command {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$Hint
    )

    if (-not (Get-Command -Name $Name -ErrorAction SilentlyContinue)) {
        Fail-Start "未找到 $Name，$Hint"
    }
}

function Ensure-PathContains {
    param([Parameter(Mandatory = $true)][string]$Directory)

    if (-not $Directory) {
        return
    }

    if (-not (Test-Path -LiteralPath $Directory)) {
        return
    }

    $pathEntries = ($env:PATH -split ';') | Where-Object { $_ }
    if ($pathEntries -notcontains $Directory) {
        $env:PATH = "$Directory;$env:PATH"
    }
}

function Resolve-GoCommand {
    $go = Get-Command -Name 'go' -ErrorAction SilentlyContinue
    if ($go) {
        return $go.Source
    }

    $candidates = @(
        'C:\Go\bin\go.exe',
        'C:\Program Files\Go\bin\go.exe',
        (Join-Path $env:USERPROFILE 'scoop\apps\go\current\bin\go.exe')
    )

    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate) {
            return $candidate
        }
    }

    return $null
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter()][string[]]$ArgumentList = @(),
        [string]$WorkingDirectory
    )

    $oldLocation = $null
    if ($WorkingDirectory) {
        $oldLocation = Get-Location
        Set-Location $WorkingDirectory
    }

    try {
        & $FilePath @ArgumentList
        if ($LASTEXITCODE -ne 0) {
            Fail-Start "命令执行失败：$FilePath $($ArgumentList -join ' ')（退出码 $LASTEXITCODE）"
        }
    }
    finally {
        if ($oldLocation) {
            Set-Location $oldLocation
        }
    }
}

function Get-StartChildProcessIds {
    param([Parameter(Mandatory = $true)][int]$ProcessId)

    try {
        return @(Get-CimInstance -ClassName Win32_Process -Filter "ParentProcessId=$ProcessId" -ErrorAction Stop | ForEach-Object { [int]$_.ProcessId })
    }
    catch {
        try {
            return @(Get-WmiObject -Class Win32_Process -Filter "ParentProcessId=$ProcessId" -ErrorAction Stop | ForEach-Object { [int]$_.ProcessId })
        }
        catch {
            return @()
        }
    }
}

function Stop-StartProcessTree {
    param(
        [Parameter(Mandatory = $true)][int]$ProcessId,
        [hashtable]$Visited
    )

    if ($ProcessId -le 0) {
        return
    }

    if (-not $Visited) {
        $Visited = @{}
    }

    if ($Visited.ContainsKey($ProcessId)) {
        return
    }
    $Visited[$ProcessId] = $true

    foreach ($childId in @(Get-StartChildProcessIds -ProcessId $ProcessId)) {
        Stop-StartProcessTree -ProcessId $childId -Visited $Visited
    }

    if ($ProcessId -eq $PID) {
        return
    }

    $process = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue
    if (-not $process) {
        return
    }

    try {
        if (-not $process.HasExited) {
            Log-Start "结束进程：$($process.ProcessName)($ProcessId)"
            Stop-Process -Id $ProcessId -Force -ErrorAction SilentlyContinue
            try {
                Wait-Process -Id $ProcessId -Timeout 5 -ErrorAction SilentlyContinue
            }
            catch {
                # 进程可能已退出；无需额外处理。
            }
        }
    }
    catch {
        Log-Start "结束进程 $ProcessId 失败：$($_.Exception.Message)"
    }
}

function Stop-StartedProcessTrees {
    if ($script:CleanupStarted) {
        return
    }
    $script:CleanupStarted = $true

    if (-not $script:WailsProcess) {
        return
    }

    $wailsProcessId = [int]$script:WailsProcess.Id
    $childIds = @(Get-StartChildProcessIds -ProcessId $wailsProcessId)
    $wailsAlive = $false

    try {
        $script:WailsProcess.Refresh()
        $wailsAlive = -not $script:WailsProcess.HasExited
    }
    catch {
        $wailsAlive = $false
    }

    if ($wailsAlive -or $childIds.Count -gt 0) {
        Log-Start '正在结束 Wails 开发模式进程树'
        Stop-StartProcessTree -ProcessId $wailsProcessId
    }
}

trap [System.Management.Automation.PipelineStoppedException] {
    Log-Start '收到 Ctrl+C，正在强制结束已启动的进程'
    Stop-StartedProcessTrees
    exit 130
}

function Ensure-Go {
    if ($script:GoCmd) {
        return $script:GoCmd
    }

    $goCmd = Resolve-GoCommand
    if ($goCmd) {
        $script:GoCmd = $goCmd
        Ensure-PathContains -Directory (Split-Path -Path $goCmd -Parent)
        return $goCmd
    }

    $winget = Get-Command -Name 'winget' -ErrorAction SilentlyContinue
    if (-not $winget) {
        Fail-Start '未找到 go，且当前系统不可用 winget 自动安装，请先安装 Go'
    }

    Log-Start '未找到 Go，开始使用 winget 安装 GoLang.Go'
    Invoke-Checked -FilePath $winget.Source -ArgumentList @(
        'install',
        '--exact',
        '--id', 'GoLang.Go',
        '--accept-package-agreements',
        '--accept-source-agreements',
        '--disable-interactivity'
    )

    $installPathCandidates = @(
        'C:\Program Files\Go\bin',
        'C:\Go\bin',
        (Join-Path $env:USERPROFILE 'scoop\apps\go\current\bin')
    )

    foreach ($installBin in $installPathCandidates) {
        Ensure-PathContains -Directory $installBin
    }

    $goCmd = Resolve-GoCommand
    if ($goCmd) {
        $script:GoCmd = $goCmd
        Ensure-PathContains -Directory (Split-Path -Path $goCmd -Parent)
        return $goCmd
    }

    Fail-Start 'Go 安装完成后仍不可用，请重开 PowerShell 后重试'
}

function Resolve-WailsCommand {
    if ($env:WAILS_BIN) {
        if (Test-Path -LiteralPath $env:WAILS_BIN) {
            return (Resolve-Path -LiteralPath $env:WAILS_BIN).Path
        }

        $wailsFromCommand = Get-Command -Name $env:WAILS_BIN -ErrorAction SilentlyContinue
        if ($wailsFromCommand) {
            return $wailsFromCommand.Source
        }

        Fail-Start "WAILS_BIN 指向的 Wails 不可用：$env:WAILS_BIN"
    }

    $wails = Get-Command -Name 'wails' -ErrorAction SilentlyContinue
    if ($wails) {
        return $wails.Source
    }

    $gopath = (& (Ensure-Go) env GOPATH).Trim()
    if ($LASTEXITCODE -ne 0) {
        Fail-Start '无法读取 GOPATH，请检查 Go 环境'
    }

    $candidates = @(
        Join-Path $gopath 'bin\wails.exe'
        Join-Path $gopath 'bin\wails'
    )

    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate) {
            return $candidate
        }
    }

    return $null
}

function Ensure-Wails {
    $wailsCmd = Resolve-WailsCommand
    if ($wailsCmd) {
        return $wailsCmd
    }

    Log-Start "未找到 Wails，开始安装 github.com/wailsapp/wails/v2/cmd/wails@$($env:WAILS_VERSION)"
    $goCmd = Ensure-Go
    Invoke-Checked -FilePath $goCmd -ArgumentList @('install', "github.com/wailsapp/wails/v2/cmd/wails@$($env:WAILS_VERSION)")

    $wailsCmd = Resolve-WailsCommand
    if ($wailsCmd) {
        return $wailsCmd
    }

    Fail-Start 'Wails 安装完成后仍不可用，请检查 GOPATH/bin 权限或 PATH 配置'
}

function Ensure-GoDependencies {
    $goMod = Join-Path $script:RootDir 'go.mod'
    if (-not (Test-Path -LiteralPath $goMod)) {
        return
    }

    Log-Start "检查 Go 依赖，GOTOOLCHAIN=$($env:GOTOOLCHAIN)，GOPROXY=$($env:GOPROXY)"
    $goCmd = Ensure-Go
    Invoke-Checked -FilePath $goCmd -ArgumentList @('mod', 'download')
}

function Test-FrontendDependencies {
    $packageJson = Join-Path $script:FrontendDir 'package.json'
    if (-not (Test-Path -LiteralPath $packageJson)) {
        return $false
    }

    $script = @'
const fs = require("fs");
const pkg = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
const sections = ["dependencies", "devDependencies", "optionalDependencies", "peerDependencies"];
const hasDeps = sections.some((section) => pkg[section] && Object.keys(pkg[section]).length > 0);
process.exit(hasDeps ? 0 : 1);
'@

    & node -e $script $packageJson
    return ($LASTEXITCODE -eq 0)
}

function Ensure-FrontendDependencies {
    $packageJson = Join-Path $script:FrontendDir 'package.json'
    if (-not (Test-Path -LiteralPath $packageJson)) {
        return
    }

    Require-Command -Name 'node' -Hint '请先安装 Node.js'
    Require-Command -Name 'npm' -Hint '请先安装 npm'

    if (-not (Test-FrontendDependencies)) {
        Log-Start '前端未声明 npm 依赖，跳过 npm install'
        return
    }

    $nodeModules = Join-Path $script:FrontendDir 'node_modules'
    $packageLock = Join-Path $script:FrontendDir 'package-lock.json'
    $needsInstall = -not (Test-Path -LiteralPath $nodeModules)

    if (-not $needsInstall) {
        $nodeModulesItem = Get-Item -LiteralPath $nodeModules
        $packageJsonItem = Get-Item -LiteralPath $packageJson

        if ($packageJsonItem.LastWriteTime -gt $nodeModulesItem.LastWriteTime) {
            $needsInstall = $true
        }
        elseif ((Test-Path -LiteralPath $packageLock) -and ((Get-Item -LiteralPath $packageLock).LastWriteTime -gt $nodeModulesItem.LastWriteTime)) {
            $needsInstall = $true
        }
    }

    if ($needsInstall) {
        Log-Start '安装前端依赖'
        Invoke-Checked -FilePath 'npm' -ArgumentList @('install') -WorkingDirectory $script:FrontendDir
    }
    else {
        Log-Start '前端依赖已就绪'
    }
}

$goCmd = Ensure-Go
Ensure-GoDependencies
Ensure-FrontendDependencies
$wailsCmd = Ensure-Wails

Log-Start '使用 Wails 开发模式启动项目（强制重新构建后端）'
$wailsArgs = @('dev', '-forcebuild') + $args
$exitCode = 0

try {
    $script:WailsProcess = Start-Process `
        -FilePath $wailsCmd `
        -ArgumentList $wailsArgs `
        -WorkingDirectory $script:RootDir `
        -NoNewWindow `
        -PassThru

    while ($true) {
        $script:WailsProcess.Refresh()
        if ($script:WailsProcess.HasExited) {
            $exitCode = $script:WailsProcess.ExitCode
            break
        }
        Start-Sleep -Milliseconds 500
    }
}
finally {
    Stop-StartedProcessTrees
}

exit $exitCode
