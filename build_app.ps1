<#
.SYNOPSIS
  构建当前 Wails 项目为 Windows 可运行的 exe。

.DESCRIPTION
  默认执行：
    1. 检查 Go / npm / Wails CLI；
    2. 运行 go test ./...；
    3. 执行 wails build -platform windows/amd64；
    4. 将 build/bin 中最新生成的 exe 复制为 build/KiroX-windows-amd64.exe。

  如果系统未安装 Wails CLI，脚本会使用 Go 自动安装：
    go install github.com/wailsapp/wails/v2/cmd/wails@latest

.EXAMPLE
  .\build_app.ps1

.EXAMPLE
  .\build_app.ps1 -Clean -SkipTests

.EXAMPLE
  .\build_app.ps1 -Platform windows/arm64 -WebView2Strategy embed
#>

[CmdletBinding()]
param(
    [ValidateSet('windows/amd64', 'windows/arm64', 'windows/386')]
    [string]$Platform = 'windows/amd64',

    [ValidateSet('download', 'embed', 'browser', 'error')]
    [string]$WebView2Strategy = 'download',

    [string]$OutputDir = 'build',

    [string]$WailsVersion = $(if ($env:WAILS_VERSION) { $env:WAILS_VERSION } else { 'latest' }),

    [switch]$Clean,

    [switch]$SkipTests,

    [switch]$KeepConsole,

    [switch]$VerboseBuild
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:RootDir = if ($PSScriptRoot) {
    $PSScriptRoot
}
else {
    (Get-Location).Path
}

function Write-Step {
    param([Parameter(Mandatory = $true)][string]$Message)
    Write-Host "[build_app] $Message" -ForegroundColor Cyan
}

function Fail-Build {
    param([Parameter(Mandatory = $true)][string]$Message)
    throw "[build_app] $Message"
}

function Add-PathIfExists {
    param([Parameter(Mandatory = $true)][string]$Directory)

    if (-not (Test-Path -LiteralPath $Directory)) {
        return
    }

    $entries = @($env:PATH -split ';' | Where-Object { $_ })
    if ($entries -notcontains $Directory) {
        $env:PATH = "$Directory;$env:PATH"
    }
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter()][string[]]$ArgumentList = @(),
        [Parameter()][string]$WorkingDirectory = $script:RootDir
    )

    $display = "$FilePath $($ArgumentList -join ' ')".Trim()
    Write-Step "执行：$display"

    Push-Location $WorkingDirectory
    try {
        & $FilePath @ArgumentList
        if ($LASTEXITCODE -ne 0) {
            Fail-Build "命令失败（退出码 $LASTEXITCODE）：$display"
        }
    }
    finally {
        Pop-Location
    }
}

function Resolve-CommandPath {
    param([Parameter(Mandatory = $true)][string]$Name)

    $cmd = Get-Command -Name $Name -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }

    return $null
}

function Resolve-Go {
    $go = Resolve-CommandPath -Name 'go'
    if ($go) {
        Add-PathIfExists -Directory (Split-Path -Path $go -Parent)
        return $go
    }

    $candidates = @(
        'C:\Program Files\Go\bin\go.exe',
        'C:\Go\bin\go.exe',
        (Join-Path $env:USERPROFILE 'scoop\apps\go\current\bin\go.exe')
    )

    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate) {
            Add-PathIfExists -Directory (Split-Path -Path $candidate -Parent)
            return $candidate
        }
    }

    Fail-Build '未找到 Go。请先安装 Go，或将 go.exe 加入 PATH。'
}

function Get-GoPath {
    param([Parameter(Mandatory = $true)][string]$GoCmd)

    $gopath = (& $GoCmd env GOPATH).Trim()
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($gopath)) {
        Fail-Build '无法读取 GOPATH，请检查 Go 环境。'
    }

    return $gopath
}

function Resolve-Wails {
    param([Parameter(Mandatory = $true)][string]$GoCmd)

    if ($env:WAILS_BIN) {
        if (Test-Path -LiteralPath $env:WAILS_BIN) {
            return (Resolve-Path -LiteralPath $env:WAILS_BIN).Path
        }

        $fromEnv = Resolve-CommandPath -Name $env:WAILS_BIN
        if ($fromEnv) {
            return $fromEnv
        }

        Fail-Build "WAILS_BIN 指向的 Wails 不可用：$env:WAILS_BIN"
    }

    $wails = Resolve-CommandPath -Name 'wails'
    if ($wails) {
        return $wails
    }

    $gopath = Get-GoPath -GoCmd $GoCmd
    $goBin = Join-Path $gopath 'bin'
    Add-PathIfExists -Directory $goBin

    foreach ($candidate in @((Join-Path $goBin 'wails.exe'), (Join-Path $goBin 'wails'))) {
        if (Test-Path -LiteralPath $candidate) {
            return $candidate
        }
    }

    Write-Step "未找到 Wails CLI，开始安装 github.com/wailsapp/wails/v2/cmd/wails@$WailsVersion"
    Invoke-Checked -FilePath $GoCmd -ArgumentList @('install', "github.com/wailsapp/wails/v2/cmd/wails@$WailsVersion")

    Add-PathIfExists -Directory $goBin
    foreach ($candidate in @((Join-Path $goBin 'wails.exe'), (Join-Path $goBin 'wails'))) {
        if (Test-Path -LiteralPath $candidate) {
            return $candidate
        }
    }

    Fail-Build 'Wails 安装完成后仍不可用，请检查 GOPATH/bin 权限或 PATH 配置。'
}

function Assert-ProjectRoot {
    $wailsConfig = Join-Path $script:RootDir 'wails.json'
    $goMod = Join-Path $script:RootDir 'go.mod'

    if (-not (Test-Path -LiteralPath $wailsConfig)) {
        Fail-Build "未找到 wails.json：$wailsConfig"
    }

    if (-not (Test-Path -LiteralPath $goMod)) {
        Fail-Build "未找到 go.mod：$goMod"
    }
}

function Get-WailsOutputName {
    $wailsConfig = Join-Path $script:RootDir 'wails.json'
    try {
        $config = Get-Content -LiteralPath $wailsConfig -Raw | ConvertFrom-Json
        if ($config.outputfilename) {
            return [string]$config.outputfilename
        }
    }
    catch {
        Write-Step "读取 wails.json outputfilename 失败，将回退为 kirox：$($_.Exception.Message)"
    }

    return 'kirox'
}

function Find-BuiltExe {
    $binDir = Join-Path $script:RootDir 'build\bin'
    if (-not (Test-Path -LiteralPath $binDir)) {
        Fail-Build "构建目录不存在：$binDir"
    }

    $exe = Get-ChildItem -LiteralPath $binDir -Filter '*.exe' -File |
        Sort-Object LastWriteTimeUtc -Descending |
        Select-Object -First 1

    if (-not $exe) {
        Fail-Build "未在 $binDir 找到 exe 构建产物。"
    }

    return $exe.FullName
}

function Copy-FinalExe {
    param(
        [Parameter(Mandatory = $true)][string]$SourceExe,
        [Parameter(Mandatory = $true)][string]$OutputName
    )

    $outDir = if ([System.IO.Path]::IsPathRooted($OutputDir)) {
        $OutputDir
    }
    else {
        Join-Path $script:RootDir $OutputDir
    }

    New-Item -ItemType Directory -Path $outDir -Force | Out-Null

    $safePlatform = $Platform -replace '/', '-'
    $targetExe = Join-Path $outDir "$OutputName-$safePlatform.exe"

    Copy-Item -LiteralPath $SourceExe -Destination $targetExe -Force
    return $targetExe
}

Assert-ProjectRoot

$goCmd = Resolve-Go
Write-Step "Go：$goCmd"

$npmCmd = Resolve-CommandPath -Name 'npm'
if (-not $npmCmd) {
    Fail-Build '未找到 npm。Wails 前端构建需要 Node.js/npm，请先安装 Node.js。'
}
Write-Step "npm：$npmCmd"

$wailsCmd = Resolve-Wails -GoCmd $goCmd
Write-Step "Wails：$wailsCmd"

if (-not $SkipTests) {
    Invoke-Checked -FilePath $goCmd -ArgumentList @('test', './...')
}
else {
    Write-Step '跳过 Go 测试（-SkipTests）'
}

$wailsArgs = @('build', '-platform', $Platform, '-webview2', $WebView2Strategy, '-trimpath')

if ($Clean) {
    $wailsArgs += '-clean'
}

if ($KeepConsole) {
    $wailsArgs += '-windowsconsole'
}

if ($VerboseBuild) {
    $wailsArgs += @('-v', '2')
}

Invoke-Checked -FilePath $wailsCmd -ArgumentList $wailsArgs

$builtExe = Find-BuiltExe
$outputName = Get-WailsOutputName
$finalExe = Copy-FinalExe -SourceExe $builtExe -OutputName $outputName

Write-Step "Wails 原始产物：$builtExe"
Write-Step "Windows exe：$finalExe"
Write-Host "构建完成：$finalExe" -ForegroundColor Green
