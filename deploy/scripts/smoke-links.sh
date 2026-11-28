#!/usr/bin/env bash
# Run manually after rollout with a dedicated smoke tenant's API key.
set +x # Keep credentials out of shell traces, including when called with bash -x.
set -euo pipefail

fail() {
  printf 'Link smoke failed: %s\n' "$*" >&2
  exit 1
}

[[ -z ${CI:-} ]] || fail 'run this manually outside CI'
api_base=${LINK_SMOKE_API_BASE:-https://api-otp.dikhanh.io.vn}
public_base=${LINK_SMOKE_PUBLIC_BASE:-https://link.dikhanh.io.vn}
api_base=${api_base%/}
public_base=${public_base%/}
origin_pattern='^(https://[A-Za-z0-9.-]+|http://(localhost|127\.0\.0\.1))(:[0-9]+)?$'
[[ $api_base =~ $origin_pattern && $public_base =~ $origin_pattern ]] ||
  fail 'API and public bases must be HTTPS origins (HTTP is allowed for loopback testing)'

key_pattern='^[A-Za-z0-9_~+./-]+=*$'
[[ ${LINK_SMOKE_API_KEY:-} =~ $key_pattern ]] || fail 'set LINK_SMOKE_API_KEY to the smoke tenant API key'
if [[ -n ${LINK_SMOKE_OTHER_API_KEY:-} ]]; then
  [[ $LINK_SMOKE_OTHER_API_KEY =~ $key_pattern && $LINK_SMOKE_OTHER_API_KEY != "$LINK_SMOKE_API_KEY" ]] ||
    fail 'LINK_SMOKE_OTHER_API_KEY must be a different tenant API key'
fi
if ! command -v curl >/dev/null || ! command -v jq >/dev/null; then
  fail 'curl and jq are required'
fi

tmp=$(mktemp -d)
body=$tmp/body
headers=$tmp/headers
needs_disable=false

fetch() {
  local method=$1 url=$2
  shift 2
  local method_args=(--request "$method")
  if [[ $method == HEAD ]]; then
    method_args=(--head)
  fi
  status=$(curl --disable --silent --show-error --max-redirs 0 --max-time 5 \
    --dump-header "$headers" --output "$body" --write-out '%{http_code}' \
    "${method_args[@]}" "$@" --url "$url") || fail "$method $url did not complete"
}

expect() {
  local expected=$1 method=$2 url=$3
  shift 3
  fetch "$method" "$url" "$@"
  [[ $status == "$expected" ]] || fail "$method $url returned $status, expected $expected"
}

api() {
  local expected=$1 method=$2 url=$3
  shift 3
  # Curl reads the header from stdin so the API key stays out of process arguments.
  expect "$expected" "$method" "$url" --header @- "$@" <<< "Authorization: Bearer $LINK_SMOKE_API_KEY"
}

cleanup() {
  if [[ $needs_disable == true ]]; then
    printf 'Attempting to disable smoke code %s after failure.\n' "$code" >&2
    (api 200 POST "$detail/disable") || printf 'Disable smoke code %s manually.\n' "$code" >&2
  fi
  rm -f -- "$body" "$headers"
  rmdir -- "$tmp"
}
trap cleanup EXIT

header() {
  awk -v field="$1" '
    /^HTTP\// { value = "" }
    tolower($0) ~ "^" field ":" {
      sub(/^[^:]+:[ \t]*/, "")
      sub(/\r$/, "")
      value = $0
    }
    END { print value }
  ' "$headers"
}

links=$api_base/v1/links
destination=https://example.com/worklane-link-smoke/$(date -u +%Y%m%dT%H%M%SZ)-${tmp: -6}
payload=$(jq -nc --arg url "$destination" '{long_url: $url}')
json=(--header 'Content-Type: application/json' --data-binary "$payload")

expect 200 GET "$public_base/healthz"
expect 200 GET "$api_base/healthz"
expect 401 GET "$api_base/v1/otp/requests"
expect 401 GET "$links"
jq -e '.error == "missing credentials"' "$body" >/dev/null || fail 'unauthenticated request did not reach Link Service'

api 201 POST "$links" "${json[@]}"
code=$(jq -er '.code | select(type == "string" and test("^[0-9A-Za-z]{1,11}$"))' "$body") || fail 'create did not return a Link code'
detail=$links/$code
needs_disable=true
analytics=$detail/analytics
short_url=$public_base/$code
printf 'Created smoke code %s\n' "$code"
jq -e --arg url "$short_url" '.short_url == $url' "$body" >/dev/null || fail 'unexpected short URL'
api 200 POST "$links" "${json[@]}"
jq -e --arg code "$code" --arg url "$short_url" '.code == $code and .short_url == $url' "$body" >/dev/null || fail 'duplicate create returned a different link'

api 200 GET "$links"
jq -e --arg code "$code" 'any(.items[]; .code == $code and .state == "active")' "$body" >/dev/null || fail 'active link missing from list'
api 200 GET "$detail"
jq -e --arg code "$code" '.code == $code and .state == "active"' "$body" >/dev/null || fail 'unexpected link detail'

expect 302 HEAD "$short_url"
[[ $(header location) == "$destination" && $(header cache-control) == no-store ]] || fail 'HEAD redirect headers differ'
sleep 15
api 200 GET "$analytics"
jq -e '.total_clicks == 0 and (.series | length) == 14 and .recent == []' "$body" >/dev/null || fail 'analytics after HEAD did not show a real zero'

# A single GET to the short URL records the click. Curl leaves the redirect unfollowed.
deadline=$((SECONDS + 10))
expect 302 GET "$short_url" --header 'Referer:'
[[ $(header location) == "$destination" && $(header cache-control) == no-store ]] || fail 'GET redirect headers differ'
while :; do
  remaining=$((deadline - SECONDS))
  (( remaining > 0 )) || fail 'click did not appear within 10 seconds'
  api 200 GET "$analytics" --max-time "$((remaining < 5 ? remaining : 5))"
  if jq -e '.total_clicks == 1 and (.series | length) == 14 and
    ([.series[].clicks] | add) == 1 and (.recent | length) == 1 and
    .recent[0].referrer_origin == ""' "$body" >/dev/null; then
    break
  fi
  jq -e '.total_clicks == 0' "$body" >/dev/null || fail 'analytics did not show exactly one visit with unknown referrer'
  sleep 1
done

api 200 POST "$detail/disable"
needs_disable=false
deadline=$((SECONDS + 60))
while :; do
  remaining=$((deadline - SECONDS))
  (( remaining > 0 )) || fail 'redirect did not return 410 within 60 seconds'
  fetch HEAD "$short_url" --max-time "$((remaining < 5 ? remaining : 5))"
  [[ $status != 410 ]] || break
  [[ $status == 302 ]] || fail "disabled redirect returned $status, expected 302 or 410"
  sleep 1
done
api 409 POST "$links" "${json[@]}"
jq -e '.error == "LINK_DISABLED"' "$body" >/dev/null || fail 're-create did not return LINK_DISABLED'
api 200 GET "$analytics"
jq -e '.total_clicks == 1' "$body" >/dev/null || fail 'disabled link did not retain its one click'

for path in /metrics /v1/links /auth/me /internal/introspect; do
  expect 404 GET "$public_base$path"
done
expect 404 HEAD "$public_base/zzzzzzzzzzz"
# The redirect handler sets no-store even on its bodyless HEAD 404.
[[ $(header cache-control) == no-store ]] || fail 'unknown code did not reach Link Service'
if [[ -n ${LINK_SMOKE_OTHER_API_KEY:-} ]]; then
  expect 404 GET "$detail" --header @- <<< "Authorization: Bearer $LINK_SMOKE_OTHER_API_KEY"
fi

printf 'Link smoke passed for %s (disabled, one recorded click).\n' "$code"
