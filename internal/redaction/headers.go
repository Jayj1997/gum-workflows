package redaction

import "regexp"

// sensitiveHeaderPattern matches a sensitive header name followed by its
// value, tolerating any non-newline spacing around the colon, so both log
// formats ("Authorization: Bearer x") and dumped wire text are covered.
// The name is kept; only the value is replaced.
var sensitiveHeaderPattern = regexp.MustCompile(`(?i)\b(authorization|proxy-authorization|cookie|set-cookie|x-api-key)(\s*:\s*)([^\r\n]*)`)
