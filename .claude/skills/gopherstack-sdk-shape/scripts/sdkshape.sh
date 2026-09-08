#!/usr/bin/env bash
# Locate the authoritative AWS wire shape for a service (+ optional operation)
# in the pinned aws-sdk-go-v2 module cache. See ../SKILL.md for the recipe
# this automates.
set -euo pipefail
shopt -s nullglob

usage() {
	echo "usage: $(basename "$0") <service> [Operation]" >&2
	exit 1
}

[[ $# -ge 1 && $# -le 2 ]] || usage
svc="$1"
op="${2:-}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
gomod="$repo_root/go.mod"
[[ -f "$gomod" ]] || { echo "error: go.mod not found at $gomod" >&2; exit 1; }

version="$(grep -oP "aws-sdk-go-v2/service/${svc} \K\S+" "$gomod" || true)"
if [[ -z "$version" ]]; then
	echo "error: github.com/aws/aws-sdk-go-v2/service/${svc} not found in $gomod" >&2
	echo "hint: check the module name — some services nest, e.g. service/s3control" >&2
	exit 1
fi

modcache="$(go env GOMODCACHE)"
sdkdir="${modcache}/github.com/aws/aws-sdk-go-v2/service/${svc}@${version}"
[[ -d "$sdkdir" ]] || { echo "error: $sdkdir does not exist (run 'go mod download' or check version)" >&2; exit 1; }

detect_protocol() {
	local ser="$sdkdir/serializers.go"
	[[ -f "$ser" ]] || { echo "unknown (no serializers.go)"; return; }
	if grep -q "^func awsRestjson1_" "$ser" 2>/dev/null; then
		echo "REST-JSON (awsRestjson1_*)"
	elif grep -q "^func awsEc2query_" "$ser" 2>/dev/null; then
		echo "EC2-query (awsEc2query_*)"
	elif grep -q "^func awsAwsjson11_" "$ser" 2>/dev/null; then
		echo "JSON-RPC 1.1 (awsAwsjson11_*)"
	elif grep -q "^func awsAwsjson10_" "$ser" 2>/dev/null; then
		echo "JSON-RPC 1.0 (awsAwsjson10_*)"
	elif grep -q "^func awsRestxml_" "$ser" 2>/dev/null; then
		echo "REST-XML (awsRestxml_*)"
	elif grep -q "^func awsAwsquery_" "$ser" 2>/dev/null; then
		echo "Query (awsAwsquery_*)"
	else
		echo "unknown (no recognized serializer prefix)"
	fi
}

if [[ -z "$op" ]]; then
	echo "service:   $svc"
	echo "version:   $version"
	echo "sdk dir:   $sdkdir"
	echo "protocol:  $(detect_protocol)"
	echo
	opfiles=("$sdkdir"/api_op_*.go)
	count=${#opfiles[@]}
	echo "ops found: $count"
	for f in "${opfiles[@]:0:20}"; do
		base="$(basename "$f")"
		echo "  ${base#api_op_}"
	done | sed 's/\.go$//'
	if (( count > 20 )); then
		echo "  ... and $((count - 20)) more"
	fi
	exit 0
fi

opfile="$sdkdir/api_op_${op}.go"
[[ -f "$opfile" ]] || { echo "error: $opfile not found — check operation name casing" >&2; exit 1; }

echo "service:   $svc"
echo "version:   $version"
echo "protocol:  $(detect_protocol)"
echo "api file:  $opfile"
echo

# Matches serializeOp<Op>, serializeOpHttpBindings<Op>Input, serializeOpDocument<Op>Input
# etc, while excluding a longer sibling op name that merely starts with the same
# prefix (e.g. "CreateWorkspace" must not match "CreateWorkspaceApiKey"): after
# the op name, allow an optional Input/Output suffix, then require a non-letter
# or end of line.
opBoundary="${op}(Input|Output)?([^A-Za-z]|\$)"

echo "-- serializers.go --"
grep -n -E "^func .*erializeOp.*${opBoundary}" "$sdkdir/serializers.go" 2>/dev/null || echo "  (no match — check op name)"

echo
echo "-- deserializers.go --"
grep -n -E "^func .*eserializeOp.*${opBoundary}" "$sdkdir/deserializers.go" 2>/dev/null || echo "  (no match)"

echo
echo "-- error switch (authoritative per-op exceptions) --"
grep -n "func aws.*_deserializeOpError${op}\b" "$sdkdir/deserializers.go" 2>/dev/null || echo "  (no match)"
