# Runs the Windows unit tests on an agent without Go installed: downloads the Go
# toolchain matching go.mod, then vets and tests. Run from the unpacked source root
# (go.mod, cmd, pkg). Called from the Jenkinsfile 'Run Windows tests' stage.
# The TestWindows run is verbose: it covers the Job Object / CTRL_BREAK
# process-tree restart. Integration tests need a Conjur server and sit behind a
# build tag, so they are not run here.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
function Log($msg) { Write-Output ("[{0}] {1}" -f (Get-Date -Format 'HH:mm:ss'), $msg) }

$goVersion = (Select-String -Path go.mod -Pattern '^go[ ]+([0-9.]+)').Matches[0].Groups[1].Value
$goDir = Join-Path (Get-Location) '.go-toolchain'
$goRoot = Join-Path $goDir 'go'
$goBin = Join-Path $goRoot 'bin'
if (-not (Test-Path (Join-Path $goBin 'go.exe'))) {
  Log "downloading go $goVersion"
  New-Item -ItemType Directory -Force $goDir | Out-Null
  $zip = Join-Path $goDir 'go.zip'
  Invoke-WebRequest -UseBasicParsing -Uri "https://go.dev/dl/go$goVersion.windows-amd64.zip" -OutFile $zip
  Log 'extracting go'
  tar.exe -xf $zip -C $goDir
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
$env:GOROOT = $goRoot
$env:PATH = "$goBin;$env:PATH"
$env:GOTOOLCHAIN = 'local'

go version
Log 'go vet'
go vet ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Log 'go test TestWindows'
go test -v -count=1 ./pkg/summon -run 'TestWindows'
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Log 'go test ./...'
go test -count=1 ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Log 'done'
