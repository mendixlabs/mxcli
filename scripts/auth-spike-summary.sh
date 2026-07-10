#!/usr/bin/env bash
# Extracts a compact summary from /tmp/auth-spike-report.md — just the
# status codes and the first 20 lines of each JSON body, skipping the
# noisy Mendix CSP/security headers. Safe to paste.

set -u
REPORT=/tmp/auth-spike-report.md

if [[ ! -f "$REPORT" ]]; then
  echo "error: $REPORT not found — run scripts/auth-discovery-spike.sh first" >&2
  exit 1
fi

awk '
  /^### / { print ""; print $0; next }
  /^HTTP [0-9]+/ { print $0; in_body=0; body_lines=0; next }
  /^--- response body/ { in_body=1; body_lines=0; next }
  /^--- response headers/ { in_body=0; next }
  /^```$/ { in_body=0; next }
  in_body && body_lines < 20 {
    if (length($0) > 200) {
      print substr($0, 1, 200) " ...[truncated]"
    } else {
      print $0
    }
    body_lines++
  }
' "$REPORT"
