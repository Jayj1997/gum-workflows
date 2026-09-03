package redaction

import "regexp"

// sensitiveHeaderPattern matches a sensitive header name followed by its
// value, tolerating any non-newline spacing around the colon, so both log
// formats ("Authorization: Bearer x") and dumped wire text are covered.
// The value stops at a double quote so a JSON log line like
// "error":"Authorization: Bearer x","runId":"..." keeps its closing quote and
// every following field: known Secret values are removed by exact substring
// replacement first, so limiting this structural pass never keeps a
// registered canary.
var sensitiveHeaderPattern = regexp.MustCompile(`(?i)\b(authorization|proxy-authorization|cookie|set-cookie|x-api-key)(\s*:\s*)([^"\r\n]*)`)
