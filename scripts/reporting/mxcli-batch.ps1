<#
.SYNOPSIS
    Run mxcli across a fleet of .mpr projects in three modes (native PowerShell port
    of mxcli-batch.sh -- same behavior, same output shape, Windows-native).

.DESCRIPTION
    Modes:
      calibrate  Raw, unscored per-category penalty/rate data (same as the original
                 calibrate.sh) -- for deriving/tuning normalizationBaseline values.
                 Writes ONE file (default: calibration.json) directly into
                 -ProjectsDir.

      summary    The ACTUAL scored report (mxcli's real Score/Categories, using
                 whatever normalizationBaseline is currently built into the binary)
                 for every project, combined into ONE json file (default:
                 summary.json) directly into -ProjectsDir. Suitable as-is for
                 visualization (e.g. a per-app-per-category heatmap).

      full       Same combined summary.json as above, PLUS the complete, unmodified
                 mxcli report for each individual project saved as
                 "<app-name>-report.json". All of these land together in one output
                 FOLDER (default name: mxcli-reports) inside -ProjectsDir.

    Expects a directory containing one subfolder per project, each subfolder
    containing exactly one .mpr file, e.g.:
      MAIN\
        GBS - Human in the Loop (HITL)-main\CANValidator.mpr
        TOPS-main\SomeApp.mpr
        ...

.PARAMETER Mode
    One of: calibrate, summary, full.

.PARAMETER Mxcli
    Path to the mxcli executable (or a name resolvable on PATH, e.g. mxcli.exe).

.PARAMETER ProjectsDir
    Directory containing one subfolder per project.

.PARAMETER OutName
    Output file name (calibrate/summary) or output folder name (full). Defaults to
    calibration.json / summary.json / mxcli-reports depending on -Mode.

.PARAMETER ForceInit
    By default, a project's .claude\lint-rules\ is only (re)created via `mxcli init`
    if it's missing or empty -- an already-initialized project is left alone, even
    if the rules embedded in the mxcli binary have since changed (e.g. after editing
    a .star rule and rebuilding). Pass -ForceInit to always wipe and re-run
    `mxcli init` for every project, guaranteeing everyone is on the latest rules
    currently embedded in -Mxcli.

.PARAMETER ExcludeMarketplace
    Forwarded to every `mxcli report` call, excluding all Marketplace-sourced
    modules (and System) from calibration/summary/full output alike.

.PARAMETER Exclude
    Module name(s) forwarded to every `mxcli report` call as --exclude. Accepts a
    comma-separated list (e.g. -Exclude SharedUtils,LegacyImportModule). Combine
    with -ExcludeMarketplace to also drop specific local modules (e.g. internal
    shared libraries) from the score.

.EXAMPLE
    .\mxcli-batch.ps1 -Mode summary -Mxcli .\mxcli.exe -ProjectsDir .\MAIN -OutName summary.json -ExcludeMarketplace

.EXAMPLE
    .\mxcli-batch.ps1 -Mode full -Mxcli .\mxcli.exe -ProjectsDir .\MAIN -OutName mxcli-reports `
        -ExcludeMarketplace -Exclude SharedUtils,LegacyImportModule

.PARAMETER Help
    Show this help (equivalent to Get-Help -Detailed) and exit.

.EXAMPLE
    .\mxcli-batch.ps1 -Mode calibrate -Mxcli .\mxcli.exe -ProjectsDir .\MAIN -OutName calibration.json -ExcludeMarketplace -ForceInit

.EXAMPLE
    .\mxcli-batch.ps1 -Help
#>

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('calibrate', 'summary', 'full')]
    [string]$Mode,

    [Parameter(Position = 1)]
    [string]$Mxcli,

    [Parameter(Position = 2)]
    [string]$ProjectsDir,

    [Parameter(Position = 3)]
    [string]$OutName,

    [switch]$ForceInit,
    [switch]$ExcludeMarketplace,
    [string[]]$Exclude = @(),

    [Alias('h')]
    [switch]$Help
)

# Progress/diagnostic messages go to stderr (mirrors the bash script's ">&2" calls);
# the final "Wrote ..." / "Done: ..." lines go to stdout, so scripted callers can
# still capture just those if they want to.
function Write-Log {
    param([string]$Message)
    [Console]::Error.WriteLine($Message)
}

if ($Help) {
    Get-Help $MyInvocation.MyCommand.Path -Detailed
    exit 0
}

# Mode/Mxcli/ProjectsDir are validated manually (rather than via Mandatory=$true)
# so that -Help can be honored before the parameter binder would otherwise prompt
# interactively for each missing mandatory value.
if (-not $Mode -or -not $Mxcli -or -not $ProjectsDir) {
    Write-Error "Usage: mxcli-batch.ps1 <calibrate|summary|full> <mxcli-path> <projects-dir> [output-name] [-ForceInit] [-ExcludeMarketplace] [-Exclude <module,...>]`nRun with -Help for full parameter details."
    exit 1
}

if (-not (Get-Command $Mxcli -ErrorAction SilentlyContinue)) {
    Write-Error "mxcli not found: '$Mxcli' (use a path like .\mxcli.exe, ./mxcli, or a bare name resolvable on PATH)"
    exit 1
}

if (-not $OutName) {
    $OutName = switch ($Mode) {
        'calibrate' { 'calibration.json' }
        'summary' { 'summary.json' }
        'full' { 'mxcli-reports' }
    }
}

if (-not (Test-Path -LiteralPath $ProjectsDir -PathType Container)) {
    Write-Error "Projects directory not found: $ProjectsDir"
    exit 1
}
$ProjectsDir = (Resolve-Path -LiteralPath $ProjectsDir).Path.TrimEnd('\', '/')

# calibrate/summary write a single file directly into ProjectsDir.
# full writes a whole folder of files into ProjectsDir.
if ($Mode -eq 'full') {
    $ReportsDir = Join-Path $ProjectsDir $OutName
    New-Item -ItemType Directory -Path $ReportsDir -Force | Out-Null
    $OutFile = Join-Path $ReportsDir 'summary.json'
}
else {
    $OutFile = Join-Path $ProjectsDir $OutName
}

# Entries that mxcli init (and mxcli itself, during report/lint runs) can scaffold
# into a project directory. These are tooling artifacts, not part of the Mendix app
# -- make sure they never get committed to that project's own git repo.
$GitignoreEntries = @(
    '/.ai-context',
    '/.claude',
    '/.mxcli',
    '/.playwright',
    'AGENTS.md',
    'CLAUDE.md',
    '/.local',
    '/.devcontainer',
    '/node_modules',
    '/javascriptsource/**/android/',
    '/*.launch'
)

function Ensure-GitIgnore {
    param([string]$ProjectDir, [string]$ProjectName)

    $gitignoreFile = Join-Path $ProjectDir '.gitignore'
    if (-not (Test-Path -LiteralPath $gitignoreFile)) {
        New-Item -ItemType File -Path $gitignoreFile -Force | Out-Null
    }

    $existing = @(Get-Content -LiteralPath $gitignoreFile -ErrorAction SilentlyContinue)
    $toAdd = @($GitignoreEntries | Where-Object { $existing -notcontains $_ })

    if ($toAdd.Count -gt 0) {
        Add-Content -LiteralPath $gitignoreFile -Value ''
        Add-Content -LiteralPath $gitignoreFile -Value '# mxcli / AI-assisted tooling (added by mxcli-batch.ps1)'
        Add-Content -LiteralPath $gitignoreFile -Value $toAdd
        Write-Log "  Updated .gitignore in '$ProjectName' (+$($toAdd.Count) entries)"
    }
}

$TmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $TmpDir | Out-Null

$count = 0
$skipped = 0
$projects = @()

try {
    $projectDirs = Get-ChildItem -Path $ProjectsDir -Directory

    foreach ($projectDirItem in $projectDirs) {
        $projectDir = $projectDirItem.FullName
        $projectName = $projectDirItem.Name

        # In full mode, the output folder itself lives inside ProjectsDir and would
        # otherwise get picked up by this same enumeration as if it were a project
        # -- skip it silently, it's not a project.
        if ($Mode -eq 'full' -and $projectDir.TrimEnd('\', '/') -eq $ReportsDir.TrimEnd('\', '/')) {
            continue
        }

        $mprFile = Get-ChildItem -LiteralPath $projectDir -Filter '*.mpr' -File -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $mprFile) {
            Write-Log "  skipped '$projectName' (no .mpr found)"
            $skipped++
            continue
        }

        # Auto-initialize any project missing Starlark lint rules, so every
        # project contributes the same rule coverage regardless of mode.
        # -ForceInit bypasses the "already has rules" check entirely and wipes the
        # directory first, so a stale copy of an old .star rule can never survive a
        # forced re-init.
        $lintRulesDir = Join-Path $projectDir (Join-Path '.claude' 'lint-rules')
        $runInit = $false
        if ($ForceInit) {
            $runInit = $true
            Write-Log "  '$projectName': -ForceInit set, refreshing lint rules..."
            if (Test-Path -LiteralPath $lintRulesDir) {
                Remove-Item -LiteralPath $lintRulesDir -Recurse -Force
            }
        }
        elseif (-not (Test-Path -LiteralPath $lintRulesDir) -or
                (Get-ChildItem -LiteralPath $lintRulesDir -Force -ErrorAction SilentlyContinue | Measure-Object).Count -eq 0) {
            $runInit = $true
            Write-Log "  '$projectName' has no .claude/lint-rules -- running mxcli init..."
        }

        if ($runInit) {
            $initLog = Join-Path $TmpDir "${projectName}_init.log"
            & $Mxcli init $projectDir *> $initLog
            if ($LASTEXITCODE -ne 0) {
                Write-Log "  skipped '$projectName' (mxcli init failed -- see below)"
                Get-Content -LiteralPath $initLog -ErrorAction SilentlyContinue | ForEach-Object { Write-Log "    $_" }
                $skipped++
                continue
            }
        }
        Ensure-GitIgnore -ProjectDir $projectDir -ProjectName $projectName

        Write-Log "Assessing '$projectName'..."
        $safeName = ($projectName -replace '[^A-Za-z0-9_-]', '_')
        $outJson = Join-Path $TmpDir "$safeName.json"
        $errFile = Join-Path $TmpDir "$safeName.err"

        $reportFlags = @()
        if ($Mode -eq 'calibrate') {
            $reportFlags += @('--raw', '--format', 'json')
        }
        else {
            # summary/full: the ACTUAL scored report, not raw calibration data.
            $reportFlags += @('--format', 'json')
        }
        if ($ExcludeMarketplace) {
            $reportFlags += '--exclude-marketplace'
        }
        foreach ($m in $Exclude) {
            if ($m) {
                $reportFlags += @('--exclude', $m)
            }
        }

        & $Mxcli report -p $mprFile.FullName @reportFlags -o $outJson 2> $errFile
        if ($LASTEXITCODE -ne 0) {
            Write-Log "  skipped '$projectName' (report failed -- see below)"
            Get-Content -LiteralPath $errFile -ErrorAction SilentlyContinue | ForEach-Object { Write-Log "    $_" }
            $skipped++
            continue
        }

        if (-not (Test-Path -LiteralPath $outJson) -or (Get-Item -LiteralPath $outJson).Length -eq 0) {
            Write-Log "  skipped '$projectName' (mxcli exited 0 but wrote no output)"
            $skipped++
            continue
        }

        # full mode: keep the complete, unmodified per-app report alongside the
        # eventual summary, named "<app-name>-report.json".
        if ($Mode -eq 'full') {
            Copy-Item -LiteralPath $outJson -Destination (Join-Path $ReportsDir "$projectName-report.json") -Force
        }

        try {
            $projObj = Get-Content -LiteralPath $outJson -Raw | ConvertFrom-Json
        }
        catch {
            Write-Log "  skipped '$projectName' (invalid JSON output: $_)"
            $skipped++
            continue
        }

        $projects += $projObj
        $count++
    }

    Write-Log ''
    Write-Log "Collected $count project(s), skipped $skipped. Building output..."

    # Post-process the collected reports into the final output shape. calibrate
    # keeps its own per-category summary/average math (unscored rates);
    # summary/full pass mxcli's own already-scored report objects through
    # unchanged -- no re-scoring happens in this script.
    if ($Mode -eq 'calibrate') {
        $sums = [ordered]@{}
        foreach ($proj in $projects) {
            foreach ($cat in $proj.categories) {
                $name = $cat.name
                if (-not $name) { $name = 'Unknown' }
                if (-not $sums.Contains($name)) {
                    $sums[$name] = [ordered]@{
                        n               = 0
                        elementsChecked = 0.0
                        errors          = 0.0
                        warnings        = 0.0
                        infos           = 0.0
                        penalty         = 0.0
                        rate            = 0.0
                    }
                }
                $s = $sums[$name]
                $s.n++
                $s.elementsChecked += [double]($cat.elementsChecked)
                $s.errors += [double]($cat.errors)
                $s.warnings += [double]($cat.warnings)
                $s.infos += [double]($cat.infos)
                $s.penalty += [double]($cat.penalty)
                $s.rate += [double]($cat.rate)
            }
        }

        $summary = [ordered]@{}
        foreach ($name in ($sums.Keys | Sort-Object)) {
            $s = $sums[$name]
            $n = if ($s.n -gt 0) { $s.n } else { 1 }
            $summary[$name] = [ordered]@{
                projectsWithData  = $s.n
                avgElementsChecked = [math]::Round($s.elementsChecked / $n, 1)
                avgErrors          = [math]::Round($s.errors / $n, 2)
                avgWarnings        = [math]::Round($s.warnings / $n, 2)
                avgInfos           = [math]::Round($s.infos / $n, 2)
                avgPenalty         = [math]::Round($s.penalty / $n, 3)
                avgRate            = [math]::Round($s.rate / $n, 4)
            }
        }

        $output = [ordered]@{
            summary      = $summary
            projectCount = $projects.Count
            projects     = $projects
        }
    }
    else {
        # summary/full: mxcli's own scored Report objects, combined as-is. Also
        # compute a simple across-fleet average per category score, since that's
        # the one useful cross-project rollup that doesn't require re-deriving
        # anything mxcli already scored.
        $catScores = [ordered]@{}
        foreach ($proj in $projects) {
            foreach ($cat in $proj.categories) {
                $name = $cat.name
                if (-not $name) { $name = 'Unknown' }
                if (-not $catScores.Contains($name)) {
                    $catScores[$name] = [System.Collections.Generic.List[double]]::new()
                }
                $catScores[$name].Add([double]($cat.score))
            }
        }

        $summary = [ordered]@{}
        foreach ($name in ($catScores.Keys | Sort-Object)) {
            $scores = $catScores[$name]
            $summary[$name] = [ordered]@{
                projectsWithData = $scores.Count
                avgScore         = [math]::Round(($scores | Measure-Object -Sum).Sum / $scores.Count, 1)
            }
        }

        $output = [ordered]@{
            summary      = $summary
            projectCount = $projects.Count
            apps         = $projects
        }
    }

    # Written via .NET directly rather than Set-Content -Encoding: the BOM-less
    # UTF-8 encoding name ("utf8NoBOM") is only valid on PowerShell 6+ -- Windows
    # PowerShell 5.1's Set-Content binds -Encoding to the older
    # FileSystemCmdletProviderEncoding enum, which has no such value and throws a
    # ParameterBindingException. [System.Text.UTF8Encoding]::new($false) +
    # File.WriteAllText behave identically on 5.1 and 7+.
    $jsonText = $output | ConvertTo-Json -Depth 100
    [System.IO.File]::WriteAllText($OutFile, $jsonText, [System.Text.UTF8Encoding]::new($false))
    Write-Host "Wrote $OutFile"
}
finally {
    Remove-Item -LiteralPath $TmpDir -Recurse -Force -ErrorAction SilentlyContinue
}

if ($Mode -eq 'full') {
    Write-Host "Done: $count project(s) collected, $skipped skipped -> $ReportsDir\ (summary.json + per-app reports)"
}
else {
    Write-Host "Done: $count project(s) collected, $skipped skipped -> $OutFile"
}
