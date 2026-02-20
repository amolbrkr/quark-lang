param(
    [int]$N = 5000,
    [int]$Iterations = 5000,
    [int]$Runs = 5,
    [switch]$Rebuild,
    [switch]$SkipPython
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$quarkDir = Join-Path $root "src/core/quark"
$quarkExe = Join-Path $quarkDir "quark.exe"
$generatedDir = Join-Path $PSScriptRoot "generated"
$listFile = Join-Path $generatedDir "bench_list.qrk"
$vectorFile = Join-Path $generatedDir "bench_vector.qrk"
$listBin = Join-Path $generatedDir "bench_list.exe"
$vectorBin = Join-Path $generatedDir "bench_vector.exe"
$pythonScript = Join-Path $PSScriptRoot "bench_python.py"

$totalOps = [long]$N * [long]$Iterations

if (!(Test-Path $generatedDir)) {
    New-Item -ItemType Directory -Path $generatedDir | Out-Null
}

Write-Host ""
Write-Host "=== Quark Benchmark: List vs Vector ==="
Write-Host "N=$N elements, $Iterations iterations ($($totalOps.ToString('N0')) element updates), $Runs runs"
Write-Host ""

# --- Generate Quark source files ---

$nums = (1..$N) -join ", "

# Quark vector benchmark:
#   nums = vector [1, 2, ..., N]
#   iter = 0
#   while iter < ITERATIONS:
#       nums = nums + 1
#       iter = iter + 1
#   println(sum(nums))
$vectorProgram = @"
// Auto-generated benchmark: vector scalar update
nums = vector [$nums]

iter = 0
while iter < ${Iterations}:
    nums = nums + 1
    iter = iter + 1

println(sum(nums))
"@

# Quark list benchmark:
#   nums = list [1, 2, ..., N]
#   iter = 0
#   while iter < ITERATIONS:
#       idx1 = 0
#       while idx1 < len(nums):
#           set(nums, idx1, get(nums, idx1) + 1)
#           idx1 = idx1 + 1
#       iter = iter + 1
#   checksum = 0
#   idx2 = 0
#   while idx2 < len(nums):
#       checksum = checksum + get(nums, idx2)
#       idx2 = idx2 + 1
#   println(checksum)
$listProgram = @"
// Auto-generated benchmark: list scalar loop update
nums = list [$nums]

iter = 0
while iter < ${Iterations}:
    idx1 = 0
    while idx1 < len(nums):
        set(nums, idx1, get(nums, idx1) + 1)
        idx1 = idx1 + 1
    iter = iter + 1

checksum = 0
idx2 = 0
while idx2 < len(nums):
    checksum = checksum + get(nums, idx2)
    idx2 = idx2 + 1

println(checksum)
"@

Set-Content -Path $vectorFile -Value $vectorProgram -Encoding ASCII
Set-Content -Path $listFile -Value $listProgram -Encoding ASCII

# --- Build ---

Push-Location $quarkDir
try {
    if ($Rebuild -or !(Test-Path $quarkExe)) {
        Write-Host "Building quark.exe..."
        go build -o quark.exe .
    }

    Write-Host "Compiling benchmarks (one-time cost)..."

    $swVecBuild = [System.Diagnostics.Stopwatch]::StartNew()
    & $quarkExe build $vectorFile -o $vectorBin 2>&1 | Out-Null
    $swVecBuild.Stop()
    if ($LASTEXITCODE -ne 0) { throw "Build failed for vector benchmark" }
    Write-Host ("  vector compiled in {0:F1}s" -f ($swVecBuild.Elapsed.TotalSeconds))

    $swListBuild = [System.Diagnostics.Stopwatch]::StartNew()
    & $quarkExe build $listFile -o $listBin 2>&1 | Out-Null
    $swListBuild.Stop()
    if ($LASTEXITCODE -ne 0) { throw "Build failed for list benchmark" }
    Write-Host ("  list   compiled in {0:F1}s" -f ($swListBuild.Elapsed.TotalSeconds))

    Write-Host ""

    # --- Timed runner ---

    function Run-Timed([string]$label, [string]$exePath) {
        $times = @()

        # Warm-up run
        & $exePath *> $null
        if ($LASTEXITCODE -ne 0) { throw "Warm-up failed for $exePath" }

        for ($r = 1; $r -le $Runs; $r++) {
            $sw = [System.Diagnostics.Stopwatch]::StartNew()
            & $exePath *> $null
            $sw.Stop()
            if ($LASTEXITCODE -ne 0) { throw "Run #$r failed for $exePath" }
            $times += $sw.Elapsed.TotalMilliseconds
        }

        $avg = ($times | Measure-Object -Average).Average
        $min = ($times | Measure-Object -Minimum).Minimum
        $max = ($times | Measure-Object -Maximum).Maximum

        return [PSCustomObject]@{
            Label = $label
            AvgMs = [Math]::Round($avg, 1)
            MinMs = [Math]::Round($min, 1)
            MaxMs = [Math]::Round($max, 1)
        }
    }

    # --- Run Quark benchmarks ---

    Write-Host "Running Quark benchmarks ($Runs runs, 1 warmup)..."
    $vectorStats = Run-Timed "Quark vector" $vectorBin
    $listStats   = Run-Timed "Quark list"   $listBin

    # --- Run Python baselines ---

    $pythonListMs  = $null
    $numpyMs       = $null

    if (!$SkipPython -and (Test-Path $pythonScript)) {
        $pythonExe = $null
        if (Get-Command python -ErrorAction SilentlyContinue) { $pythonExe = "python" }
        elseif (Get-Command python3 -ErrorAction SilentlyContinue) { $pythonExe = "python3" }

        if ($pythonExe) {
            Write-Host "Running Python baselines..."
            $pyOutput = & $pythonExe $pythonScript --n $N --iter $Iterations 2>&1
            foreach ($line in $pyOutput) {
                if ($line -match 'Python list\s*:\s*([\d.]+)\s*ms') {
                    $pythonListMs = [double]$Matches[1]
                }
                if ($line -match 'NumPy array\s*:\s*([\d.]+)\s*ms') {
                    $numpyMs = [double]$Matches[1]
                }
            }
        } else {
            Write-Host "(python not found, skipping Python baselines)"
        }
    }

    # --- Results ---

    Write-Host ""
    Write-Host "=== Results ==="
    Write-Host ("  N={0}, iterations={1}, total ops={2}" -f $N, $Iterations, $totalOps.ToString('N0'))
    Write-Host ""
    Write-Host ("  {0,-20} {1,10} {2,10} {3,10}" -f "Implementation", "avg (ms)", "min (ms)", "max (ms)")
    Write-Host ("  {0,-20} {1,10} {2,10} {3,10}" -f ("=" * 20), ("=" * 10), ("=" * 10), ("=" * 10))
    Write-Host ("  {0,-20} {1,10} {2,10} {3,10}" -f $vectorStats.Label, $vectorStats.AvgMs, $vectorStats.MinMs, $vectorStats.MaxMs)
    Write-Host ("  {0,-20} {1,10} {2,10} {3,10}" -f $listStats.Label, $listStats.AvgMs, $listStats.MinMs, $listStats.MaxMs)

    if ($numpyMs) {
        Write-Host ("  {0,-20} {1,10}" -f "NumPy", [Math]::Round($numpyMs, 1))
    }
    if ($pythonListMs) {
        Write-Host ("  {0,-20} {1,10}" -f "Python list", [Math]::Round($pythonListMs, 1))
    }

    Write-Host ""
    $vecVsList = if ($vectorStats.AvgMs -gt 0) { [Math]::Round($listStats.AvgMs / $vectorStats.AvgMs, 1) } else { 0 }
    Write-Host "  Quark vector vs list : ${vecVsList}x"
    if ($pythonListMs -and $listStats.AvgMs -gt 0) {
        $listVsPy = [Math]::Round($pythonListMs / $listStats.AvgMs, 0)
        Write-Host "  Quark list vs Python : ${listVsPy}x"
    }
    if ($pythonListMs -and $vectorStats.AvgMs -gt 0) {
        $vecVsPy = [Math]::Round($pythonListMs / $vectorStats.AvgMs, 0)
        Write-Host "  Quark vector vs Python: ${vecVsPy}x"
    }
    Write-Host ""
}
finally {
    Pop-Location
}
