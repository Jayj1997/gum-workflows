#!/bin/sh

workspace=$1
tool_output=$2

cd "$workspace" || exit 125
: > "$tool_output/packages.txt" || exit 125
: > "$tool_output/test.json" || exit 125
: > "$tool_output/test-exit.txt" || exit 125
go version > "$tool_output/go-version.txt" || exit 125
go env GOVERSION GOROOT GOOS GOARCH CGO_ENABLED > "$tool_output/go-env.txt" || exit 125

printf 'running go coverage check\n'
go list ./... > "$tool_output/packages.txt"
list_status=$?
if [ "$list_status" -ne 0 ]; then
  printf '%s\n' "$list_status" > "$tool_output/test-exit.txt" || exit 125
  exit "$list_status"
fi
if [ ! -s "$tool_output/packages.txt" ]; then
  printf 'mode: atomic\n' > "$tool_output/coverage.out" || exit 125
  printf '0\n' > "$tool_output/test-exit.txt" || exit 125
  exit 0
fi

(go test -count=1 -json -covermode=atomic -coverprofile="$tool_output/coverage.out" ./...; printf '%s\n' "$?" > "$tool_output/test-exit.txt") |
  tee "$tool_output/test.json"
stream_status=$?
if [ "$stream_status" -ne 0 ]; then
  exit 125
fi
IFS= read -r test_status < "$tool_output/test-exit.txt" || exit 125
exit "$test_status"
