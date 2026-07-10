#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# auth-discovery-spike.sh — Map the Mendix Marketplace download path.
#
# Round-4 scope. Auth + list/detail/versions endpoints are already
# confirmed. This round probes where to find the .mpk download URL:
#
#   - Does /v1/content/{id}/versions include a downloadUrl field? (Earlier
#     spike truncated the response after the first object.)
#   - Is files.appstore.mendix.com (the CDN observed in the browser
#     DevTools) a public static host, or does it need MxToken auth?
#   - Does the static URL pattern work for any content id?
#
# Uses CommunityCommons (id 170) as a simple second test case alongside
# the External Database Connector (id 2888) from earlier rounds.
#
# Reads $MENDIX_PAT from the environment; never logs it.
# Output: /tmp/auth-spike-report.md (redacted — safe to share).
#
# Usage:
#   export MENDIX_PAT='<your-pat>'
#   ./scripts/auth-discovery-spike.sh
#   ./scripts/auth-spike-summary.sh          # compact pasteable view

set -u

REPORT=/tmp/auth-spike-report.md
MARKETPLACE="https://marketplace-api.mendix.com"
FILES="https://files.appstore.mendix.com"

if [[ -z "${MENDIX_PAT:-}" ]]; then
  echo "error: MENDIX_PAT is not set" >&2
  exit 2
fi

redact() { sed "s|${MENDIX_PAT}|MENDIX_PAT_REDACTED|g"; }

# probe <label> <method> <url> [curl args...]
probe() {
  local label="$1" method="$2" url="$3"
  shift 3
  local tmp_body tmp_headers code
  tmp_body=$(mktemp)
  tmp_headers=$(mktemp)
  code=$(curl -sS -o "$tmp_body" -D "$tmp_headers" -w "%{http_code}" \
    -X "$method" "$@" "$url" 2>&1) || code="CURL_ERR"

  echo "### $label"
  echo
  echo '```'
  echo "$method $url"
  echo "HTTP $code"
  echo "--- response headers ---"
  head -20 "$tmp_headers" | redact
  echo "--- response body (first 120 lines) ---"
  head -120 "$tmp_body" | redact
  echo '```'
  echo

  rm -f "$tmp_body" "$tmp_headers"
}

# probe_head <label> <url> [curl args...] — HEAD only, for large downloads
probe_head() {
  local label="$1" url="$2"
  shift 2
  local tmp_headers code
  tmp_headers=$(mktemp)
  code=$(curl -sS -I -D "$tmp_headers" -o /dev/null -w "%{http_code}" \
    "$@" "$url" 2>&1) || code="CURL_ERR"

  echo "### $label"
  echo
  echo '```'
  echo "HEAD $url"
  echo "HTTP $code"
  echo "--- response headers ---"
  head -25 "$tmp_headers" | redact
  echo '```'
  echo

  rm -f "$tmp_headers"
}

AUTH=(-H "Authorization: MxToken $MENDIX_PAT" -H "Accept: application/json")

{
  echo "# Marketplace Download-URL Spike Report"
  echo
  echo "Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  echo "Marketplace API: \`$MARKETPLACE\`"
  echo "Files CDN:       \`$FILES\`"
  echo
  echo "Looking for: the path to the .mpk download for a given (contentId, version)."
  echo "PAT redacted as \`MENDIX_PAT_REDACTED\`."
  echo

  echo "## 1. Full versions response (looking for downloadUrl field)"
  echo
  echo "Previous spike truncated after the first version object. This one prints"
  echo "up to 120 lines so we can see all fields including any download URLs."
  echo

  probe "Versions — DB Connector (id 2888)"   GET "$MARKETPLACE/v1/content/2888/versions" "${AUTH[@]}"
  probe "Versions — CommunityCommons (id 170)" GET "$MARKETPLACE/v1/content/170/versions" "${AUTH[@]}"

  echo "## 2. Single version detail (often carries download URLs)"
  echo
  echo "Try a few path shapes in case there's a dedicated version endpoint."
  echo

  probe "Detail — /v1/content/170/versions/11.5.0"         GET "$MARKETPLACE/v1/content/170/versions/11.5.0" "${AUTH[@]}"
  probe "Detail — /v1/content/170 (full component body)"   GET "$MARKETPLACE/v1/content/170" "${AUTH[@]}"

  echo "## 3. Files CDN — is it public, or does it need auth?"
  echo
  echo "Browser DevTools showed: $FILES/5/170/11.5.0/CommunityCommons_11.5.0.mpk"
  echo "HEAD probes so we don't actually download the whole .mpk."
  echo

  probe_head "HEAD CDN (no auth)"  "$FILES/5/170/11.5.0/CommunityCommons_11.5.0.mpk"
  probe_head "HEAD CDN (MxToken)"  "$FILES/5/170/11.5.0/CommunityCommons_11.5.0.mpk" "${AUTH[@]}"

  echo "## 4. Does the static URL pattern generalize?"
  echo
  echo "If the '5' is a constant, DB Connector should follow the same shape."
  echo "DB Connector latest = 7.0.2 per earlier spike."
  echo

  probe_head "HEAD CDN — DB Connector at /5/2888/..." \
    "$FILES/5/2888/7.0.2/DatabaseConnector_7.0.2.mpk"
  probe_head "HEAD CDN — DB Connector alt name"  \
    "$FILES/5/2888/7.0.2/Database_Connector_7.0.2.mpk"

  echo "## Summary — what to confirm"
  echo
  echo "- [ ] Versions response contains downloadUrl (preferred — API-provided path)"
  echo "- [ ] If yes, does the URL point at files.appstore.mendix.com or elsewhere?"
  echo "- [ ] Files CDN returns 200 without MxToken (public), or requires auth"
  echo "- [ ] Static URL pattern \`/{?}/{contentId}/{version}/{Name}_{version}.mpk\`"
  echo "      generalizes across modules, or DB Connector/CommunityCommons diverge"
  echo
  echo "**Decision**: prefer the API-provided downloadUrl if present; fall back to"
  echo "constructing the files CDN URL only if we must."
} > "$REPORT"

echo "report written to $REPORT"
echo "run: ./scripts/auth-spike-summary.sh   # compact view for pasting"
