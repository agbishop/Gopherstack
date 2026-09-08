#!/usr/bin/env bash
# svcmap.sh <service> — compact structural summary of one services/<service> dir.
# Run from anywhere inside the gopherstack repo.
set -euo pipefail

svc="${1:?usage: svcmap.sh <service-name> (e.g. grafana, networkmanager, account)}"

repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
dir="$repo_root/services/$svc"

if [[ ! -d "$dir" ]]; then
  echo "no such service dir: $dir" >&2
  exit 1
fi

echo "== $svc =="

# --- routing shape ---
handler="$dir/handler.go"
shape="unknown"
if [[ -f "$handler" ]]; then
  if grep -q "operationHandlers\[" "$handler" 2>/dev/null; then
    shape="fixed POST /OperationName map (operationHandlers)"
  elif grep -q "routeTable()" "$handler" 2>/dev/null; then
    shape="declarative route table (routeTable)"
  elif grep -q "routeRequest(" "$handler" 2>/dev/null; then
    shape="path-segment switch (routeRequest)"
  fi
fi
echo "routing shape: $shape"

# --- op count, from GetSupportedOperations ---
op_count="?"
if [[ -f "$handler" ]]; then
  body="$(awk '/func \(h \*Handler\) GetSupportedOperations/,/^}/' "$handler")"
  if grep -q 'return \[\]string{' <<<"$body"; then
    op_count="$(grep -oP '"[A-Z][A-Za-z0-9]*"' <<<"$body" | sort -u | wc -l)"
  else
    op_count="$(grep -hoP 'op:\s*"[A-Z][A-Za-z0-9]*"' "$dir"/handler*.go 2>/dev/null | sort -u | wc -l)"
  fi
fi
echo "ops: $op_count (GetSupportedOperations)"

# --- parity grade ---
parity="$dir/PARITY.md"
if [[ -f "$parity" ]]; then
  grade="$(grep -m1 '^overall:' "$parity" | awk '{print $2}')"
  audit_date="$(grep -m1 '^last_audit_date:' "$parity" | awk '{print $2}')"
  echo "parity grade: ${grade:-none} (last audit ${audit_date:-unknown})"
else
  echo "parity grade: NO PARITY.md"
fi

# --- optional per-service files ---
for f in persistence.go tags.go cross_service.go crossservice.go chaos_transitions.go; do
  [[ -f "$dir/$f" ]] && present+=("$f")
done
echo "present: ${present[*]:-none of persistence/tags/cross_service/chaos_transitions}"

# --- file counts ---
all_go=("$dir"/*.go)
test_go=("$dir"/*_test.go)
non_test_count=0
total_lines=0
for f in "${all_go[@]}"; do
  [[ -f "$f" ]] || continue
  total_lines=$((total_lines + $(wc -l <"$f")))
  [[ "$f" == *_test.go ]] || non_test_count=$((non_test_count + 1))
done
test_count=0
for f in "${test_go[@]}"; do [[ -f "$f" ]] && test_count=$((test_count + 1)); done
echo "files: $non_test_count non-test, $test_count test, $total_lines total lines"

echo "largest non-test files:"
wc -l "$dir"/*.go 2>/dev/null | grep -v _test.go | grep -v ' total$' \
  | sort -rn | head -6 | awk -v d="$dir/" '{sub(d,"",$2); printf "  %5d  %s\n", $1, $2}'

echo "test files:"
ls "$dir"/*_test.go 2>/dev/null | xargs -n1 basename | sed 's/^/  /' || echo "  none"
