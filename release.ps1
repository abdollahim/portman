#Requires -Version 5.1
$ErrorActionPreference = "Stop"

Set-Location -Path $PSScriptRoot

# Defaults for a repo with no tags or no commits yet.
$version = "0.0.0-dev"
$commit  = "no-commit"

$inRepo = $false
try {
    git rev-parse --is-inside-work-tree 2>$null | Out-Null
    $inRepo = ($LASTEXITCODE -eq 0)
} catch {}

if ($inRepo) {
    try {
        $head = git rev-parse --short HEAD 2>$null
        if ($LASTEXITCODE -eq 0 -and $head) { $commit = $head.Trim() }

        # Version = latest tag (v0.1.0 -> 0.1.0).
        $tag = git describe --tags --abbrev=0 2>$null
        if ($LASTEXITCODE -eq 0 -and $tag) { $version = $tag.Trim().TrimStart("v") }
    } catch {}

    # Uncommitted changes make the build non-reproducible, so mark it.
    if (git status --porcelain 2>$null) {
        $commit = "$commit-dirty"
        Write-Warning "Working tree has uncommitted changes; binaries will be marked dirty."
    }
} else {
    Write-Warning "Not a git repository; building as $version."
}

$date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

$ldflags = "-s -w -X main.version=$version -X main.commit=$commit -X main.date=$date"

$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Ext = ".exe" },
    @{ GOOS = "linux";   GOARCH = "amd64"; Ext = "" }
)

# Checks before building: vet, formatting, then tests.
go vet ./...
if ($LASTEXITCODE -ne 0) { throw "go vet failed" }

$unformatted = gofmt -l .
if ($unformatted) { throw "gofmt needed on:`n$unformatted" }

go test -count=1 ./...
if ($LASTEXITCODE -ne 0) { throw "go test failed" }

New-Item -ItemType Directory -Force -Path "dist" | Out-Null

$sums = @()
try {
    foreach ($t in $targets) {
        $name = "portman-$version-$($t.GOOS)-$($t.GOARCH)$($t.Ext)"
        $out  = Join-Path "dist" $name

        $env:GOOS        = $t.GOOS
        $env:GOARCH      = $t.GOARCH
        $env:CGO_ENABLED = "0"

        go build -trimpath -ldflags $ldflags -o $out ./cmd/portman
        if ($LASTEXITCODE -ne 0) { throw "build failed for $($t.GOOS)/$($t.GOARCH)" }

        $hash = (Get-FileHash -Algorithm SHA256 -Path $out).Hash.ToLower()
        $sums += "$hash  $name"
        Write-Host "built $out"
    }
}
finally {
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

$sumFile = "dist/checksums-$version.txt"
$sums | Set-Content -Path $sumFile -Encoding ascii
Write-Host "wrote $sumFile"
Write-Host "release $version ready in dist/"