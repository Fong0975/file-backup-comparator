# Regenerates the Windows version-info resource from versioninfo.json, then
# builds the app, so every build reflects the current versioninfo.json.
go generate ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go build -ldflags="-s -w -H windowsgui" -o FileCompare.exe .
