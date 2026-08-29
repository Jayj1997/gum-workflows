#!/bin/sh

workspace=$1
tool_output=$2

cd "$workspace" || exit 125
: > "$tool_output/packages.txt" || exit 125
: > "$tool_output/vet.json" || exit 125
: > "$tool_output/vet-exit.txt" || exit 125
go version > "$tool_output/go-version.txt" || exit 125
go env GOVERSION GOROOT GOOS GOARCH CGO_ENABLED > "$tool_output/go-env.txt" || exit 125

printf 'running go static analysis\n'
go list ./... > "$tool_output/packages.txt"
list_status=$?
if [ "$list_status" -ne 0 ]; then
  exit "$list_status"
fi
if [ ! -s "$tool_output/packages.txt" ]; then
  exit 0
fi

(go vet -json ./...; printf '%s\n' "$?" > "$tool_output/vet-exit.txt") 2>&1 |
  tee "$tool_output/vet.json" >&2
stream_status=$?
if [ "$stream_status" -ne 0 ]; then
  exit 125
fi
IFS= read -r vet_status < "$tool_output/vet-exit.txt" || exit 125
exit "$vet_status"
