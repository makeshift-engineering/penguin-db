# PenguinDB Colorized Differential SQL Test Runner (check.ps1)
$ErrorActionPreference = "Stop"

$PgHost = $env:PGHOST; if (-not $PgHost) { $PgHost = "127.0.0.1" }
$PgPort = $env:PGPORT; if (-not $PgPort) { $PgPort = "5432" }
$PgUser = $env:PGUSER; if (-not $PgUser) { $PgUser = "postgres" }
$PgDb   = $env:PGDATABASE; if (-not $PgDb) { $PgDb = "postgres" }

$PengHost = "127.0.0.1"
$PengPort = "5433"

$SuiteDir = "tests/sql_suite"

Write-Host "=======================================================" -ForegroundColor Cyan
Write-Host "   PenguinDB Colorized Differential SQL Test Suite     " -ForegroundColor Cyan
Write-Host "=======================================================" -ForegroundColor Cyan

# UTF-8 Encoding without Byte Order Mark (BOM) to prevent binary diff mismatches
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)

# Clean previous temporary test artifacts (.pengout and .diff)
Get-ChildItem -Path $SuiteDir -Filter "*.pengout" | Remove-Item -Force -ErrorAction SilentlyContinue
Get-ChildItem -Path $SuiteDir -Filter "*.diff" | Remove-Item -Force -ErrorAction SilentlyContinue

$SqlFiles = Get-ChildItem -Path $SuiteDir -Filter "*.sql" | Sort-Object Name
$Passed = 0
$Failed = 0
$Total = 0
$Idx = 0

foreach ($file in $SqlFiles) {
    $Idx++
    $Total++
    $testName = $file.BaseName
    $outFile = Join-Path $SuiteDir "$testName.out"
    $pengoutFile = Join-Path $SuiteDir "$testName.pengout"
    $diffFile = Join-Path $SuiteDir "$testName.diff"

    Write-Host -NoNewline "Test $Idx`: "
    Write-Host -NoNewline "$testName " -ForegroundColor Yellow
    Write-Host -NoNewline "... "

    # Step 1: Run psql directly with stdout ONLY (quiet mode -q) to generate clean CSV .out ground truth
    $psqlCmd = Get-Command psql -ErrorAction SilentlyContinue
    if ($psqlCmd) {
        $env:PGPASSWORD = if ($env:PGPASSWORD) { $env:PGPASSWORD } else { "postgres" }
        # Capture STDOUT only (not stderr connection notices or table drop warnings)
        $rawPsql = & psql -q -h $PgHost -p $PgPort -U $PgUser -d $PgDb -X -P format=csv -f "$($file.FullName)" 2>$null
        # Filter out any non-tabular status headers or connection lines
        $cleanPsql = @($rawPsql | Where-Object { $_ -and ($_ -notmatch '^(CREATE|DROP|INSERT|UPDATE|DELETE|SET|USE|database|NOTICE|WARNING|You are now connected)') })
        [System.IO.File]::WriteAllLines($outFile, $cleanPsql, $utf8NoBom)
    } elseif (-not (Test-Path $outFile)) {
        # If psql is not installed and .out baseline doesn't exist yet, generate baseline using pengrunner
        Write-Host -NoNewline "[CREATING BASELINE .out] " -ForegroundColor Yellow
        go run ./tests/sql_suite/main.go -file "$($file.FullName)" -out "$outFile" -host $PengHost -port $PengPort
    }

    # Step 2: Run Go pengrunner CLI to generate CSV .pengout (tabular SELECT queries only)
    go run ./tests/sql_suite/main.go -file "$($file.FullName)" -out "$pengoutFile" -host $PengHost -port $PengPort

    # Step 3: Compare .out and .pengout CSVs with canonical rowsort for result blocks
    function Get-NormalizedLines($filePath) {
        if (-not (Test-Path $filePath)) { return @() }
        $rawLines = @(Get-Content -Path $filePath -Encoding UTF8 | Where-Object { $_.Trim() -ne "" })
        $result = @()
        $currentBlock = @()
        
        foreach ($line in $rawLines) {
            if ($line.StartsWith("ERROR:")) {
                if ($currentBlock.Count -gt 0) {
                    $result += ($currentBlock | Sort-Object)
                    $currentBlock = @()
                }
                $result += $line
            } elseif ($line -match '^(id|count|min|max|sum|avg|category|country|status|genre|dept_|building|floor|severity|course_|grade|author_|first_|dept_|project_|monthly_|plan|flight_|book_|borrower_|returned|location_|device_|sensor_|reading_)') {
                if ($currentBlock.Count -gt 0) {
                    $result += ($currentBlock | Sort-Object)
                    $currentBlock = @()
                }
                $result += $line
            } else {
                $currentBlock += $line
            }
        }
        if ($currentBlock.Count -gt 0) {
            $result += ($currentBlock | Sort-Object)
        }
        return $result
    }

    $expLines = Get-NormalizedLines $outFile
    $actLines = Get-NormalizedLines $pengoutFile

    $maxCount = [Math]::Max($expLines.Count, $actLines.Count)
    $mismatches = @()
    $diffTextLines = @()

    for ($i = 0; $i -lt $maxCount; $i++) {
        $exp = if ($i -lt $expLines.Count) { $expLines[$i] } else { "<EOF>" }
        $act = if ($i -lt $actLines.Count) { $actLines[$i] } else { "<EOF>" }

        if ($exp -ne $act) {
            $mismatches += [PSCustomObject]@{
                LineNo   = $i + 1
                Expected = $exp
                Actual   = $act
            }
        }
    }

    if ($mismatches.Count -eq 0) {
        Write-Host "[PASS]" -ForegroundColor Green
        if (Test-Path $diffFile) { Remove-Item $diffFile }
        $Passed++
    } else {
        Write-Host "[FAIL]" -ForegroundColor Red
        $Failed++
        
        $diffTextLines += "--- tests/sql_suite/$testName.out"
        $diffTextLines += "+++ tests/sql_suite/$testName.pengout"

        Write-Host "--- Git-Style Diff Output for Test $Idx ($testName) ---" -ForegroundColor Magenta
        
        $prevLine = -1
        foreach ($m in $mismatches) {
            if ($m.LineNo -ne ($prevLine + 1)) {
                $chunkHeader = "@@ Line $($m.LineNo) @@"
                Write-Host $chunkHeader -ForegroundColor Cyan
                $diffTextLines += $chunkHeader
            }
            $prevLine = $m.LineNo
            if ($m.Expected -ne "<EOF>") {
                $expLineStr = "- L$($m.LineNo): $($m.Expected)"
                Write-Host $expLineStr -ForegroundColor Red
                $diffTextLines += $expLineStr
            }
            if ($m.Actual -ne "<EOF>") {
                $actLineStr = "+ L$($m.LineNo): $($m.Actual)"
                Write-Host $actLineStr -ForegroundColor Green
                $diffTextLines += $actLineStr
            }
        }
        Write-Host "--------------------------------------------------------" -ForegroundColor Magenta
        
        # Write human-readable text diff file (UTF-8 without BOM)
        [System.IO.File]::WriteAllLines($diffFile, $diffTextLines, $utf8NoBom)
    }
}

Write-Host "=======================================================" -ForegroundColor Cyan
Write-Host -NoNewline "Test Results: "
Write-Host -NoNewline "$Passed Passed, " -ForegroundColor Green
Write-Host -NoNewline "$Failed Failed, " -ForegroundColor Red
Write-Host "$Total Total" -ForegroundColor Yellow
Write-Host "=======================================================" -ForegroundColor Cyan

if ($Failed -gt 0) {
    exit 1
}
