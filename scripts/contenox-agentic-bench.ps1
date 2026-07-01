param(
  [string]$BenchRoot = "$PWD\.bench\contenox-agentic",
  [string]$Contenox = "contenox",
  [string]$ContenoxSource = "",
  [string]$Go = "go",
  [string]$Modeld = "modeld",
  [string]$Provider = "openvino",
  [string]$Model = "qwen2.5-coder-0.5b-ov",
  [string]$Backend = "openvino",
  [string]$Device = "CPU",
  [string]$Chain = "",
  [string]$PromptFile = "",
  [string]$ModelsDir = "",
  [int[]]$Contexts = @(4096),
  [int]$Rounds = 1,
  [int]$MaxTokens = 128,
  [string[]]$Workloads = @("run", "chat", "tools"),
  [switch]$PullModel,
  [switch]$StartModeld
)

$ErrorActionPreference = "Stop"

function New-Dir($Path) {
  New-Item -ItemType Directory -Force $Path | Out-Null
}

function Set-Utf8NoBom($Path, [string]$Value) {
  $encoding = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($Path, $Value, $encoding)
}

function Get-UserHomeDir {
  if ($env:OS -eq "Windows_NT" -and -not [string]::IsNullOrWhiteSpace($env:USERPROFILE)) {
    return $env:USERPROFILE
  }
  if (-not [string]::IsNullOrWhiteSpace($env:HOME)) {
    return $env:HOME
  }
  return [Environment]::GetFolderPath("UserProfile")
}

function Default-ModelsDir($ProviderName, $Root) {
  if ($ProviderName -eq "llama" -or $ProviderName -eq "openvino") {
    return Join-Path (Join-Path (Join-Path (Get-UserHomeDir) ".contenox") "models") $ProviderName
  }
  return Join-Path (Join-Path $Root "models") $ProviderName
}

function Escape-JsonLine($Value) {
  if ($null -eq $Value) { return "" }
  return ($Value -replace "\\", "\\" -replace "`r", "\r" -replace "`n", "\n")
}

function Convert-GoDurationToMs($Duration) {
  if ([string]::IsNullOrWhiteSpace($Duration)) { return $null }
  $s = $Duration.Trim()
  $total = 0.0
  $matched = $false
  foreach ($m in [regex]::Matches($s, "([0-9]+(?:\.[0-9]+)?)(ns|us|µs|ms|s|m|h)")) {
    $matched = $true
    $n = [double]$m.Groups[1].Value
    switch ($m.Groups[2].Value) {
      "ns" { $total += $n / 1000000.0 }
      "us" { $total += $n / 1000.0 }
      "µs" { $total += $n / 1000.0 }
      "ms" { $total += $n }
      "s"  { $total += $n * 1000.0 }
      "m"  { $total += $n * 60000.0 }
      "h"  { $total += $n * 3600000.0 }
    }
  }
  if (-not $matched) { return $null }
  return [math]::Round($total, 3)
}

function Quote-WindowsArg($Value) {
  $s = [string]$Value
  if ($s -eq "") { return '""' }
  $quoted = '"'
  $slashes = 0
  foreach ($c in $s.ToCharArray()) {
    if ($c -eq '\') {
      $slashes++
    } elseif ($c -eq '"') {
      $quoted += ('\' * (($slashes * 2) + 1)) + '"'
      $slashes = 0
    } else {
      if ($slashes -gt 0) {
        $quoted += '\' * $slashes
        $slashes = 0
      }
      $quoted += $c
    }
  }
  if ($slashes -gt 0) {
    $quoted += '\' * ($slashes * 2)
  }
  $quoted += '"'
  return $quoted
}

function Invoke-Capture {
  param(
    [string]$Name,
    [string]$Exe,
    [string[]]$CommandArgs,
    [string]$OutPath,
    [string]$ErrPath
  )
  $sw = [Diagnostics.Stopwatch]::StartNew()
  if ($env:OS -eq "Windows_NT") {
    $cmd = ((@($Exe) + $CommandArgs | ForEach-Object { Quote-WindowsArg $_ }) -join " ")
    $cmd = "$cmd > $(Quote-WindowsArg $OutPath) 2> $(Quote-WindowsArg $ErrPath)"
    $comspec = $env:ComSpec
    if ([string]::IsNullOrWhiteSpace($comspec)) { $comspec = "cmd.exe" }
    & $comspec /d /s /c $cmd
  } else {
    & $Exe @CommandArgs > $OutPath 2> $ErrPath
  }
  $code = $LASTEXITCODE
  $sw.Stop()
  [pscustomobject]@{
    Name = $Name
    ExitCode = $code
    WallMs = [math]::Round($sw.Elapsed.TotalMilliseconds, 3)
    OutPath = $OutPath
    ErrPath = $ErrPath
  }
}

function Invoke-ContenoxCapture {
  param(
    [string]$Name,
    [string[]]$CommandArgs,
    [string]$OutPath,
    [string]$ErrPath
  )
  if ($ContenoxSource -ne "") {
    $goArgs = @("run", $script:ContenoxRunPackage) + $CommandArgs
    return Invoke-Capture -Name $Name -Exe $Go -CommandArgs $goArgs -OutPath $OutPath -ErrPath $ErrPath
  }
  return Invoke-Capture -Name $Name -Exe $Contenox -CommandArgs $CommandArgs -OutPath $OutPath -ErrPath $ErrPath
}

function Read-TraceRows($ErrPath) {
  if (-not (Test-Path $ErrPath)) { return @() }
  $rows = @()
  foreach ($line in Get-Content $ErrPath) {
    if ($line -notmatch "^\[trace\] ") { continue }
    $row = [ordered]@{
      raw = $line
      task = ""
      handler = ""
      retry = $null
      duration = ""
      duration_ms = $null
      transition = ""
      model = ""
      provider = ""
      prompt_tokens = $null
      completion_tokens = $null
      total_tokens = $null
      error = ""
    }
    if ($line -match "task=([^ ]+)") { $row.task = $Matches[1] }
    if ($line -match "handler=([^ ]+)") { $row.handler = $Matches[1] }
    if ($line -match "retry=([0-9]+)") { $row.retry = [int]$Matches[1] }
    if ($line -match "dur=([^ ]+)") {
      $row.duration = $Matches[1]
      $row.duration_ms = Convert-GoDurationToMs $Matches[1]
    }
    if ($line -match "trans=([^ ]+)") { $row.transition = $Matches[1] }
    if ($line -match "model=([^ ]+)") { $row.model = $Matches[1] }
    if ($line -match "provider=([^ ]+)") { $row.provider = $Matches[1] }
    if ($line -match "tokens=([0-9]+)\+([0-9]+)=([0-9]+)") {
      $row.prompt_tokens = [int]$Matches[1]
      $row.completion_tokens = [int]$Matches[2]
      $row.total_tokens = [int]$Matches[3]
    }
    if ($line -match " ERROR: (.*)$") { $row.error = $Matches[1] }
    $rows += [pscustomobject]$row
  }
  return $rows
}

function Read-TokenUsageRows($ErrPath) {
  if (-not (Test-Path $ErrPath)) { return @() }
  $rows = @()
  foreach ($line in Get-Content $ErrPath) {
    if ($line -notmatch "change_id=token_usage") { continue }
    $row = [ordered]@{
      limit = $null
      messages_tokens = $null
      tool_tokens = $null
      total_tokens = $null
    }
    if ($line -match "limit:([0-9]+)") { $row.limit = [int]$Matches[1] }
    if ($line -match "messages_tokens:([0-9]+)") { $row.messages_tokens = [int]$Matches[1] }
    if ($line -match "tool_tokens:([0-9]+)") { $row.tool_tokens = [int]$Matches[1] }
    if ($line -match "total_tokens:([0-9]+)") { $row.total_tokens = [int]$Matches[1] }
    $rows += [pscustomobject]$row
  }
  return $rows
}

function Write-ResultRow($Path, $Run, $TraceRows, [string]$Platform, [int]$Context, [string]$Workload, [int]$Round) {
  $llmRows = @($TraceRows | Where-Object { $_.handler -eq "chat_completion" })
  $tokenRows = @(Read-TokenUsageRows $Run.ErrPath)
  $prompt = $null
  $completion = $null
  $total = $null
  $traceMs = 0.0
  $sawUsage = $false
  foreach ($r in $llmRows) {
    if ($null -ne $r.prompt_tokens) {
      if ($null -eq $prompt) { $prompt = 0 }
      $prompt += [int]$r.prompt_tokens
      $sawUsage = $true
    }
    if ($null -ne $r.completion_tokens) {
      if ($null -eq $completion) { $completion = 0 }
      $completion += [int]$r.completion_tokens
      $sawUsage = $true
    }
    if ($null -ne $r.total_tokens) {
      if ($null -eq $total) { $total = 0 }
      $total += [int]$r.total_tokens
      $sawUsage = $true
    }
    if ($null -ne $r.duration_ms) { $traceMs += [double]$r.duration_ms }
  }
  if (-not $sawUsage) {
    $prompt = $null
    $completion = $null
    $total = $null
  }
  $assembledMessages = 0
  $assembledTools = 0
  $assembledTotal = 0
  $contextLimit = $null
  foreach ($r in $tokenRows) {
    if ($null -ne $r.messages_tokens) { $assembledMessages += [int]$r.messages_tokens }
    if ($null -ne $r.tool_tokens) { $assembledTools += [int]$r.tool_tokens }
    if ($null -ne $r.total_tokens) { $assembledTotal += [int]$r.total_tokens }
    if ($null -ne $r.limit -and ($null -eq $contextLimit -or [int]$r.limit -gt $contextLimit)) {
      $contextLimit = [int]$r.limit
    }
  }
  if ($tokenRows.Count -eq 0) {
    $assembledMessages = $null
    $assembledTools = $null
    $assembledTotal = $null
  }
  $traceErrors = @($llmRows | Where-Object { $_.error -ne "" } | ForEach-Object { $_.error })
  $tps = $null
  if ($Run.WallMs -gt 0 -and $null -ne $completion -and $completion -gt 0) {
    $tps = [math]::Round($completion / ($Run.WallMs / 1000.0), 3)
  }
  $row = [ordered]@{
    timestamp = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    platform = $Platform
    backend = $Backend
    device = $Device
    provider = $Provider
    model = $Model
    context = $Context
    workload = $Workload
    round = $Round
    exit_code = $Run.ExitCode
    wall_ms = $Run.WallMs
    trace_task_ms = [math]::Round($traceMs, 3)
    prompt_tokens = $prompt
    completion_tokens = $completion
    total_tokens = $total
    tokens_per_second_e2e = $tps
    assembled_messages_tokens = $assembledMessages
    assembled_tool_tokens = $assembledTools
    assembled_total_tokens = $assembledTotal
    context_limit_tokens = $contextLimit
    trace_error_count = $traceErrors.Count
    trace_errors = $traceErrors
    workflow_success = ($Run.ExitCode -eq 0 -and $traceErrors.Count -eq 0)
    chain = $Chain
    prompt_file = $PromptFile
    models_dir = $ModelsDir
    modeld_data_root = $ModeldDataRoot
    stdout = $Run.OutPath
    stderr = $Run.ErrPath
    contenox_invocation = $(if ($ContenoxSource -ne "") { "go-run-workspace" } else { "binary" })
  }
  ($row | ConvertTo-Json -Compress -Depth 8) | Add-Content -Path $Path
}

function Write-PromptFiles($PromptDir) {
  New-Dir $PromptDir
  $repoMap = @"
Repository: sample-agent-workspace
Files:
- README.md: project overview and expected CLI behavior
- main.go: small calculator package
- main_test.go: unit tests for Add and Mul
- CHANGELOG.md: recent edits and constraints

Task: read the context and answer with a concise engineering summary. Mention the
most relevant file names and one likely next test to run. Do not invent files.
"@
  $codeBlock = @"
// main.go
package calc

func Add(a, b int) int { return a + b }
func Mul(a, b int) int { return a * b }

// main_test.go
package calc

import "testing"

func TestAdd(t *testing.T) {
  if Add(2, 3) != 5 { t.Fatal("bad add") }
}
"@
  $payload = $repoMap + "`n`n"
  for ($i = 0; $i -lt 30; $i++) {
    $payload += "Context chunk $i`n$codeBlock`n"
  }
  Set-Utf8NoBom (Join-Path $PromptDir "repo-summary.txt") $payload
  for ($i = 1; $i -le 5; $i++) {
    $turn = "Turn ${i}: Given the same sample project, propose the next small engineering action. Keep it under 80 words.`n`n$payload"
    Set-Utf8NoBom (Join-Path $PromptDir ("turn-{0:D2}.txt" -f $i)) $turn
  }
}

function Write-SampleProject($Workspace) {
  Set-Utf8NoBom (Join-Path $Workspace "README.md") "# sample-agent-workspace`nA tiny project used by the Contenox runtime benchmark."
  Set-Utf8NoBom (Join-Path $Workspace "go.mod") "module sample-agent-workspace`n`ngo 1.22"
  Set-Utf8NoBom (Join-Path $Workspace "main.go") "package calc`n`nfunc Add(a, b int) int { return a + b }`nfunc Mul(a, b int) int { return a * b }"
  Set-Utf8NoBom (Join-Path $Workspace "main_test.go") "package calc`n`nimport `"testing`"`n`nfunc TestAdd(t *testing.T) { if Add(2, 3) != 5 { t.Fatal(`"bad add`") } }"
}

function Write-GoWorkspace($Workspace, $Source) {
  if ($Source -eq "") { return }
  $sourceMod = Join-Path $Source "go.mod"
  if (-not (Test-Path $sourceMod)) {
    throw "ContenoxSource must point at the runtime module root; missing $sourceMod"
  }
  $moduleLine = Get-Content $sourceMod | Where-Object { $_ -match "^module\s+" } | Select-Object -First 1
  if ($moduleLine -notmatch "^module\s+(\S+)") {
    throw "could not parse module path from $sourceMod"
  }
  $script:ContenoxRunPackage = "$($Matches[1])/cmd/contenox"
  $sourcePath = (Resolve-Path $Source).Path -replace "\\", "/"
  Set-Utf8NoBom (Join-Path $Workspace "go.work") "go 1.25.0`n`nuse .`nuse $sourcePath"
}

$Workspace = Join-Path $BenchRoot "workspace"
$DataDir = Join-Path $BenchRoot "data"
$ModeldDataRoot = Join-Path $BenchRoot "modeld"
$LogDir = Join-Path $BenchRoot "logs"
$PromptDir = Join-Path $BenchRoot "prompts"
$ResultPath = Join-Path $BenchRoot "results.jsonl"
New-Dir $Workspace
New-Dir $DataDir
New-Dir $ModeldDataRoot
New-Dir $LogDir
Write-PromptFiles $PromptDir
if ($PromptFile -ne "") {
  Set-Utf8NoBom (Join-Path $PromptDir "repo-summary.txt") ([System.IO.File]::ReadAllText($PromptFile))
}
Write-SampleProject $Workspace
Write-GoWorkspace $Workspace $ContenoxSource

if ($env:OS -eq "Windows_NT") {
  $platform = "windows-amd64"
} else {
  $platform = "$(uname -s)-$(uname -m)"
}

$env:CONTENOX_DATA_ROOT = $ModeldDataRoot

Push-Location $Workspace
try {
  Invoke-ContenoxCapture -Name "version" -CommandArgs @("version") -OutPath (Join-Path $LogDir "contenox-version.out") -ErrPath (Join-Path $LogDir "contenox-version.err") | Out-Null
  Invoke-ContenoxCapture -Name "init" -CommandArgs @("--data-dir", $DataDir, "init") -OutPath (Join-Path $LogDir "init.out") -ErrPath (Join-Path $LogDir "init.err") | Out-Null

  if ($ModelsDir -eq "") {
    $ModelsDir = Default-ModelsDir $Provider $BenchRoot
  }
  New-Dir $ModelsDir
  Invoke-ContenoxCapture -Name "backend-add" -CommandArgs @("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "backend", "add", $Provider, "--type", $Provider, "--url", $ModelsDir) -OutPath (Join-Path $LogDir "backend-add.out") -ErrPath (Join-Path $LogDir "backend-add.err") | Out-Null
  Invoke-ContenoxCapture -Name "config-provider" -CommandArgs @("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "config", "set", "default-provider", $Provider) -OutPath (Join-Path $LogDir "config-provider.out") -ErrPath (Join-Path $LogDir "config-provider.err") | Out-Null
  Invoke-ContenoxCapture -Name "config-model" -CommandArgs @("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "config", "set", "default-model", $Model) -OutPath (Join-Path $LogDir "config-model.out") -ErrPath (Join-Path $LogDir "config-model.err") | Out-Null
  Invoke-ContenoxCapture -Name "config-tokens" -CommandArgs @("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "config", "set", "default-max-tokens", "$MaxTokens") -OutPath (Join-Path $LogDir "config-tokens.out") -ErrPath (Join-Path $LogDir "config-tokens.err") | Out-Null
  Invoke-ContenoxCapture -Name "config-think" -CommandArgs @("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "config", "set", "default-think", "off") -OutPath (Join-Path $LogDir "config-think.out") -ErrPath (Join-Path $LogDir "config-think.err") | Out-Null

  if ($PullModel) {
    Invoke-ContenoxCapture -Name "model-pull" -CommandArgs @("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "model", "pull", $Model) -OutPath (Join-Path $LogDir "model-pull.out") -ErrPath (Join-Path $LogDir "model-pull.err") | Out-Null
  }

  $modeldProcess = $null
  if ($StartModeld) {
    $env:CONTENOX_MODELD_BACKEND = $Backend
    if ($Device -ne "") { $env:CONTENOX_OPENVINO_DEVICE = $Device }
    $modeldOut = Join-Path $LogDir "modeld.out"
    $modeldErr = Join-Path $LogDir "modeld.err"
    $modeldProcess = Start-Process -FilePath $Modeld -ArgumentList @("serve", "--data-root", $ModeldDataRoot) -RedirectStandardOutput $modeldOut -RedirectStandardError $modeldErr -PassThru
    Start-Sleep -Seconds 3
  }

  Invoke-Capture -Name "modeld-version" -Exe $Modeld -CommandArgs @("version", "--json") -OutPath (Join-Path $LogDir "modeld-version.out") -ErrPath (Join-Path $LogDir "modeld-version.err") | Out-Null
  Invoke-Capture -Name "modeld-status" -Exe $Modeld -CommandArgs @("status", "--json", "--data-root", $ModeldDataRoot) -OutPath (Join-Path $LogDir "modeld-status.out") -ErrPath (Join-Path $LogDir "modeld-status.err") | Out-Null
  Invoke-ContenoxCapture -Name "doctor" -CommandArgs @("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "doctor") -OutPath (Join-Path $LogDir "doctor.out") -ErrPath (Join-Path $LogDir "doctor.err") | Out-Null
  Invoke-ContenoxCapture -Name "model-list" -CommandArgs @("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "model", "list") -OutPath (Join-Path $LogDir "model-list.out") -ErrPath (Join-Path $LogDir "model-list.err") | Out-Null

  if (Test-Path $ResultPath) { Remove-Item $ResultPath -Force }
  foreach ($ctx in $Contexts) {
    foreach ($round in 1..$Rounds) {
      if ($Workloads -contains "run") {
        $name = "run-ctx-$ctx-round-$round"
        $chainArgs = @()
        if ($Chain -ne "") { $chainArgs = @("--chain", $Chain) }
        $run = Invoke-ContenoxCapture -Name $name -CommandArgs (@("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "--trace", "--raw", "--context", "$ctx", "--max-tokens", "$MaxTokens") + $chainArgs + @("run", "--input", ("@" + (Join-Path $PromptDir "repo-summary.txt")))) -OutPath (Join-Path $LogDir "$name.out") -ErrPath (Join-Path $LogDir "$name.err")
        Write-ResultRow $ResultPath $run (Read-TraceRows $run.ErrPath) $platform $ctx "run" $round
      }
      if ($Workloads -contains "chat") {
        Invoke-ContenoxCapture -Name "session-$ctx-$round" -CommandArgs @("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "session", "new", "bench-$ctx-$round") -OutPath (Join-Path $LogDir "session-$ctx-$round.out") -ErrPath (Join-Path $LogDir "session-$ctx-$round.err") | Out-Null
        for ($turn = 1; $turn -le 3; $turn++) {
          $name = "chat-ctx-$ctx-round-$round-turn-$turn"
          $turnPath = Join-Path $PromptDir ("turn-{0:D2}.txt" -f $turn)
          $chainArgs = @()
          if ($Chain -ne "") { $chainArgs = @("--chain", $Chain) }
          $run = Invoke-ContenoxCapture -Name $name -CommandArgs (@("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "--trace", "--raw", "--context", "$ctx", "--max-tokens", "$MaxTokens") + $chainArgs + @("chat", "--input", ("@" + $turnPath))) -OutPath (Join-Path $LogDir "$name.out") -ErrPath (Join-Path $LogDir "$name.err")
          Write-ResultRow $ResultPath $run (Read-TraceRows $run.ErrPath) $platform $ctx "chat" $turn
        }
      }
      if ($Workloads -contains "tools") {
        $name = "tools-ctx-$ctx-round-$round"
        $chainArgs = @()
        if ($Chain -ne "") { $chainArgs = @("--chain", $Chain) }
        $run = Invoke-ContenoxCapture -Name $name -CommandArgs (@("--db", (Join-Path $BenchRoot "local.db"), "--data-dir", $DataDir, "--trace", "--raw", "--steps", "--shell", "--auto", "--local-exec-allowed-dir", $Workspace, "--context", "$ctx", "--max-tokens", "$MaxTokens") + $chainArgs + @("chat", "Inspect this repository, run the relevant tests if available, and report the result.")) -OutPath (Join-Path $LogDir "$name.out") -ErrPath (Join-Path $LogDir "$name.err")
        Write-ResultRow $ResultPath $run (Read-TraceRows $run.ErrPath) $platform $ctx "tools" $round
      }
    }
  }

  Write-Host "results=$ResultPath"
  if ($modeldProcess -ne $null -and -not $modeldProcess.HasExited) {
    Stop-Process -Id $modeldProcess.Id -Force
  }
} finally {
  Pop-Location
}
