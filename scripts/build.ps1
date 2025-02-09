# Fail if we don't have gcc
Get-Command "gcc.exe"
# Fail if we don't have rsrc
Get-Command "rsrc.exe"

$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = 1

rsrc.exe -arch amd64 -manifest .\cmd\manager\G15Manager.exe.manifest -ico .\cmd\manager\go.ico -o .\cmd\manager\G15Manager.exe.syso

# go get golang.org/x/tools/cmd/stringer
go install .\...
go generate .\...
go build -ldflags="-H=windowsgui -s -w -X 'main.Version=v0.0.0-staging' -X 'main.IsDebug=no'" -o "build/G15Manager.exe" .\cmd\manager
go build -gcflags="-N -l" -ldflags="-X 'main.Version=v0.0.0-debug' -X 'main.IsDebug=yes'" -o "build/G15Manager.debug.exe" .\cmd\manager