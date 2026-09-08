#!/usr/bin/env bash
# Driver for running and poking a live gopherstack server.
# All paths are relative to the repo root; run this from the repo root.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
STATE="${GOPHERSTACK_DRIVER_STATE:-${TMPDIR:-/tmp}/gopherstack-driver}"
BIN="$REPO/bin/gopherstack"
PIDFILE="$STATE/pid"
PORTFILE="$STATE/port"
LOGFILE="$STATE/server.log"
DEFAULT_PORT="${GOPHERSTACK_PORT:-8123}"

mkdir -p "$STATE"

port() { cat "$PORTFILE" 2>/dev/null || echo "$DEFAULT_PORT"; }
endpoint() { echo "http://localhost:$(port)"; }

running() {
	[ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null
}

cmd_build() {
	# The dashboard SPA is already committed under dashboard/static/spa and
	# embedded, so a plain go build is enough; `make build` re-runs npm.
	cd "$REPO" && go build -o bin/gopherstack .
	echo "built $BIN"
}

cmd_up() {
	local p="${1:-$DEFAULT_PORT}"
	running && { echo "already up on $(endpoint)"; return 0; }
	[ -x "$BIN" ] || cmd_build
	echo "$p" > "$PORTFILE"
	# Own data dir: keeps driver runs out of ~/.gopherstack.
	GOPHERSTACK_DATA_DIR="$STATE/data" "$BIN" serve --port "$p" "${@:2}" > "$LOGFILE" 2>&1 &
	echo $! > "$PIDFILE"
	for _ in $(seq 1 60); do
		if curl -sSf -m 2 "http://localhost:$p/_gopherstack/health" >/dev/null 2>&1; then
			echo "up on http://localhost:$p ($(curl -sS "http://localhost:$p/_gopherstack/health" | jq -r '.services|length') services)"
			return 0
		fi
		sleep 0.5
	done
	echo "FAILED to come up; log:" >&2
	tail -30 "$LOGFILE" >&2
	return 1
}

cmd_down() {
	running || { echo "not running"; rm -f "$PIDFILE"; return 0; }
	kill "$(cat "$PIDFILE")" 2>/dev/null || true
	for _ in $(seq 1 20); do running || break; sleep 0.25; done
	running && kill -9 "$(cat "$PIDFILE")" 2>/dev/null || true
	rm -f "$PIDFILE"
	echo "down"
}

cmd_logs() { tail -n "${1:-40}" "$LOGFILE"; }

cmd_health() { curl -sS "$(endpoint)/_gopherstack/health" | jq .; }

# aws CLI against the live server. --endpoint-url is passed explicitly on
# purpose; see the AWS_ENDPOINT_URL gotcha in SKILL.md.
cmd_aws() {
	AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_DEFAULT_REGION=us-east-1 \
		aws --endpoint-url "$(endpoint)" "$@"
}

# Raw HTTP against the server, for wire-shape inspection.
#   driver.sh api GET /dashboard/api/system/state
#   driver.sh api POST / 'Action=ListQueues&Version=2012-11-05' 'Content-Type: application/x-www-form-urlencoded'
cmd_api() {
	local method="$1" path="$2" data="${3:-}" ctype="${4:-application/x-amz-json-1.0}"
	if [ -n "$data" ]; then
		curl -sS -i -X "$method" "$(endpoint)$path" -H "Content-Type: $ctype" \
			-H 'Authorization: AWS4-HMAC-SHA256 Credential=test/20260806/us-east-1/x/aws4_request' \
			--data "$data"
	else
		curl -sS -i -X "$method" "$(endpoint)$path"
	fi
}

cmd_shot() {
	local out="${1:-$STATE/dashboard.png}"
	google-chrome --headless --disable-gpu --no-sandbox --hide-scrollbars \
		--virtual-time-budget=8000 --window-size=1440,900 \
		--screenshot="$out" "$(endpoint)/dashboard/" >/dev/null 2>&1
	echo "$out"
}

cmd_smoke() {
	local started=0
	running || { cmd_up "$DEFAULT_PORT"; started=1; }
	local fails=0
	check() { # check <label> <expected-substring> <cmd...>
		local label="$1" want="$2"; shift 2
		local got; got="$("$@" 2>&1 || true)"
		if printf '%s' "$got" | grep -q -- "$want"; then
			echo "  ok   $label"
		else
			echo "  FAIL $label (want '$want')"; printf '%s\n' "$got" | head -3 | sed 's/^/       /'
			fails=$((fails + 1))
		fi
	}
	echo "smoke against $(endpoint)"
	check health '"status":"ok"' curl -sS "$(endpoint)/_gopherstack/health"
	# The dashboard shell is client-rendered: assert on the SvelteKit bundle,
	# not on any visible text. Use `shot` to see the rendered page.
	check dashboard '/dashboard/_app/' curl -sSL "$(endpoint)/dashboard/"
	check sts-identity '000000000000' cmd_aws sts get-caller-identity
	check s3-mb 'make_bucket' cmd_aws s3 mb "s3://smoke-$$"
	printf 'smoke\n' > "$STATE/smoke.txt"
	check s3-cp 'upload:' cmd_aws s3 cp "$STATE/smoke.txt" "s3://smoke-$$/smoke.txt"
	check s3-ls 'smoke.txt' cmd_aws s3 ls "s3://smoke-$$/"
	check ddb-create 'ACTIVE' cmd_aws dynamodb create-table --table-name "smoke$$" \
		--attribute-definitions AttributeName=pk,AttributeType=S \
		--key-schema AttributeName=pk,KeyType=HASH --billing-mode PAY_PER_REQUEST
	check sqs-create 'QueueUrl' cmd_aws sqs create-queue --queue-name "smokeq$$"
	check lambda-list 'Functions' cmd_aws lambda list-functions
	[ "$started" = 1 ] && cmd_down
	[ "$fails" = 0 ] || { echo "$fails smoke check(s) failed"; return 1; }
	echo "smoke ok"
}

case "${1:-}" in
	build)  shift; cmd_build "$@" ;;
	up)     shift; cmd_up "$@" ;;
	down)   shift; cmd_down "$@" ;;
	logs)   shift; cmd_logs "$@" ;;
	health) shift; cmd_health "$@" ;;
	aws)    shift; cmd_aws "$@" ;;
	api)    shift; cmd_api "$@" ;;
	shot)   shift; cmd_shot "$@" ;;
	smoke)  shift; cmd_smoke "$@" ;;
	*)
		cat <<'EOF'
usage: driver.sh <command>
  build              go build -o bin/gopherstack .
  up [port] [flags]  start server (default 8123), block until healthy
  down               stop it
  logs [n]           tail server log
  health             GET /_gopherstack/health
  aws <args...>      aws CLI wired to the running server
  api <METHOD> <path> [body] [content-type]   raw curl, headers included
  shot [out.png]     headless-chrome screenshot of /dashboard/
  smoke              build+up (if needed), run cross-service checks, down
EOF
		exit 1 ;;
esac
