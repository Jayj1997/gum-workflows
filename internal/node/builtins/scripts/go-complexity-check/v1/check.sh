#!/bin/sh

workspace=$1
tool_output=$2
bundle_dir=${0%/*}

cd "$workspace" || exit 125
: > "$tool_output/complexity.json" || exit 125
: > "$tool_output/analyzer-exit.txt" || exit 125
go version > "$tool_output/go-version.txt" || exit 125
go env GOVERSION GOROOT GOOS GOARCH CGO_ENABLED > "$tool_output/go-env.txt" || exit 125

printf 'running go cyclomatic complexity check\n'
go run "$bundle_dir/analyzer.go" "$workspace" "$tool_output/complexity.json"
analyzer_status=$?
printf '%s\n' "$analyzer_status" > "$tool_output/analyzer-exit.txt" || exit 125
exit "$analyzer_status"
