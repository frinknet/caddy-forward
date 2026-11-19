#!/bin/sh
# End-to-end tests for the built caddy-forward image.
#
# Run ./build.sh first, then:
#   E2E_IMAGE=caddy-forward:test sh test/e2e.sh
#
# The script only runs Docker commands and host-side HTTP checks. It never
# builds the image and never edits the Dockerfile.

set -eu

HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$HERE/.." && pwd)

TMP=$(mktemp -d /tmp/caddy-forward-e2e.XXXXXX)
SLUG=$(sed -n '1s/^[^:]*: //p' "$ROOT/NOTICE")
IMAGE=${E2E_IMAGE:-"$SLUG:test"}
CONTAINER="caddy-forward-e2e-main-$$"
CONTAINER_SIZE="caddy-forward-e2e-size-$$"
CONTAINER_TIMEOUT="caddy-forward-e2e-timeout-$$"

PASS=0
FAIL=0

cleanup() {
	docker rm -f "$CONTAINER" "$CONTAINER_SIZE" "$CONTAINER_TIMEOUT" >/dev/null 2>&1 || true
	rm -rf "$TMP"
}
trap cleanup EXIT INT TERM

pass() {
	PASS=$((PASS + 1))
	printf '  PASS: %s\n' "$1"
}

fail() {
	FAIL=$((FAIL + 1))
	printf '  FAIL: %s\n' "$1"
	if [ "$#" -gt 1 ]; then
		printf '    %s\n' "$2"
	fi
}

check_eq() {
	if [ "$2" = "$3" ]; then
		pass "$1"
	else
		fail "$1" "expected: $2; got: $3"
	fi
}

body_eq() {
	printf '%s' "$2" > "$TMP/expected"
	if cmp -s "$TMP/expected" "$TMP/body"; then
		pass "$1"
	else
		fail "$1" "expected body: $(cat "$TMP/expected"); got: $(cat "$TMP/body")"
	fi
}

body_contains() {
	if grep -qF -- "$2" "$TMP/body"; then
		pass "$1"
	else
		fail "$1" "body does not contain: $2"
	fi
}

body_absent() {
	if grep -qF -- "$2" "$TMP/body"; then
		fail "$1" "body unexpectedly contains: $2"
	else
		pass "$1"
	fi
}

body_empty() {
	if [ -s "$TMP/body" ]; then
		fail "$1" "expected an empty body, got: $(cat "$TMP/body")"
	else
		pass "$1"
	fi
}

header_contains() {
	if grep -qiF -- "$2" "$TMP/headers"; then
		pass "$1"
	else
		fail "$1" "response headers missing: $2"
	fi
}

request() {
	REQ_PORT=$1
	REQ_METHOD=$2
	REQ_PATH=$3
	: > "$TMP/body"
	: > "$TMP/headers"
	if [ "$REQ_METHOD" = "HEAD" ]; then
		STATUS=$(curl -sS --max-time 10 -I \
			-D "$TMP/headers" -o /dev/null -w '%{http_code}' \
			"http://127.0.0.1:$REQ_PORT$REQ_PATH" 2>/dev/null) || STATUS=000
	else
		STATUS=$(curl -sS --max-time 10 -X "$REQ_METHOD" \
			-D "$TMP/headers" -o "$TMP/body" -w '%{http_code}' \
			"http://127.0.0.1:$REQ_PORT$REQ_PATH" 2>/dev/null) || STATUS=000
	fi
}

wait_ready() {
	WAIT_PORT=$1
	WAIT_PATH=$2
	WAIT_I=0
	while [ "$WAIT_I" -lt 50 ]; do
		WAIT_CODE=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 1 \
			"http://127.0.0.1:$WAIT_PORT$WAIT_PATH" 2>/dev/null || true)
		case "$WAIT_CODE" in
			""|000) ;;
			*) return 0 ;;
		esac
		WAIT_I=$((WAIT_I + 1))
		sleep 0.2
	done
	return 1
}

start_container() {
	START_NAME=$1
	START_CONFIG=$2
	docker run -d --name "$START_NAME" --no-healthcheck \
		-p "127.0.0.1::80" \
		-v "$START_CONFIG:/etc/caddy/Caddyfile:ro" \
		-v "$ROOT/test/e2e:/e2e:ro" \
		"$IMAGE" run --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null
	START_PORT=$(timeout 10 docker port "$START_NAME" 80/tcp | head -n1 | sed 's/.*://')
	if [ -z "$START_PORT" ]; then
		return 1
	fi
	printf '%s' "$START_PORT"
}

fail_start() {
	echo "container $1 did not become ready" >&2
	timeout 10 docker logs "$1" 2>&1 | tail -n 40 || true
	exit 1
}

validate_ok() {
	VALIDATE_DESC=$1
	VALIDATE_CFG=$2
	VALIDATE_STATUS=0
	VALIDATE_OUT=$(timeout 30 docker run --rm \
		-v "$VALIDATE_CFG:/etc/caddy/Caddyfile:ro" \
		--entrypoint /bin/caddy "$IMAGE" \
		validate --config /etc/caddy/Caddyfile --adapter caddyfile 2>&1) || VALIDATE_STATUS=$?
	if [ "$VALIDATE_STATUS" -eq 0 ]; then
		pass "$VALIDATE_DESC"
	else
		fail "$VALIDATE_DESC" "exit=$VALIDATE_STATUS output=$VALIDATE_OUT"
	fi
}

validate_fail() {
	VALIDATE_DESC=$1
	VALIDATE_CFG=$2
	VALIDATE_NEEDLE=$3
	VALIDATE_STATUS=0
	VALIDATE_OUT=$(timeout 30 docker run --rm \
		-v "$VALIDATE_CFG:/etc/caddy/Caddyfile:ro" \
		--entrypoint /bin/caddy "$IMAGE" \
		validate --config /etc/caddy/Caddyfile --adapter caddyfile 2>&1) || VALIDATE_STATUS=$?
	if [ "$VALIDATE_STATUS" -ne 0 ] && printf '%s' "$VALIDATE_OUT" | grep -qF -- "$VALIDATE_NEEDLE"; then
		pass "$VALIDATE_DESC"
	else
		fail "$VALIDATE_DESC" "exit=$VALIDATE_STATUS output=$VALIDATE_OUT"
	fi
}

# ── Test 0: image contents ────────────────────────────────────────────────
echo ""
echo "=== Test 0: image contents ==="

if ! MODULES=$(timeout 30 docker run --rm --entrypoint /bin/caddy "$IMAGE" list-modules 2>&1); then
	fail "caddy list-modules" "$MODULES"
else
	for MODULE in forward http.handlers.forward http.handlers.templates.functions.forward_methods; do
		if printf '%s\n' "$MODULES" | grep -qx "$MODULE"; then
			pass "module $MODULE is registered"
		else
			fail "module $MODULE is registered"
		fi
	done
fi

if ! VERSIONS=$(timeout 20 docker run --rm --entrypoint /bin/cat "$IMAGE" /usr/share/licenses/versions.txt 2>&1); then
	fail "read image versions" "$VERSIONS"
else
	BUILDER=$(sed -n 's/^FROM caddy:\(.*\)-builder-alpine.*$/\1/p' "$ROOT/Dockerfile" | head -n1)
	GOT=$(printf '%s\n' "$VERSIONS" | awk '$1 == "dep" && $2 == "github.com/caddyserver/caddy/v2" {print $3}')
	check_eq "caddy version matches the builder tag" "v$BUILDER" "$GOT"
	if printf '%s\n' "$VERSIONS" | grep -q 'github.com/frinknet/caddy-forward'; then
		pass "plugin is linked into the image"
	else
		fail "plugin is linked into the image"
	fi
fi

# ── Test 1: forward directive ─────────────────────────────────────────────
echo ""
echo "=== Test 1: forward directive ==="

if ! PORT=$(start_container "$CONTAINER" "$ROOT/test/e2e/Caddyfile"); then
	fail "start main container" "docker run failed"
	exit 1
fi
if ! wait_ready "$PORT" /t/get; then
	fail_start "$CONTAINER"
fi

request "$PORT" GET /t/get
check_eq "GET status" "200" "$STATUS"
body_eq "GET body" "backend-ok"

request "$PORT" GET /t/params
check_eq "param status" "200" "$STATUS"
body_eq "param reaches the upstream query" "method=GET uri=/base?q=caddy auth= x= ct="

request "$PORT" GET /t/headers
check_eq "header status" "200" "$STATUS"
body_eq "headers reach the upstream" "method=GET uri=/headers auth=Bearer test x=x-header ct="

request "$PORT" POST /t/post
check_eq "POST status" "200" "$STATUS"
body_eq "POST body reaches the upstream" "hello body"

request "$PORT" POST /t/post-text
check_eq "POST default content type status" "200" "$STATUS"
body_eq "POST string defaults to text/plain" "ct=text/plain body=text body"

request "$PORT" POST /t/post-json
check_eq "POST custom content type status" "200" "$STATUS"
body_eq "POST custom content type is kept" "ct=application/json body=hello-json"

request "$PORT" PUT /t/put
check_eq "PUT status" "200" "$STATUS"
body_eq "PUT body reaches the upstream" "put body"

request "$PORT" PATCH /t/patch
check_eq "PATCH status" "200" "$STATUS"
body_eq "PATCH body reaches the upstream" "patch body"

request "$PORT" DELETE /t/delete
check_eq "DELETE status" "200" "$STATUS"
body_eq "DELETE sends its param in the query" "method=DELETE uri=/q?q=caddy auth= x= ct="

request "$PORT" OPTIONS /t/options
check_eq "OPTIONS status" "200" "$STATUS"
body_eq "OPTIONS sends its param in the query" "method=OPTIONS uri=/q?q=caddy auth= x= ct="

request "$PORT" HEAD /t/head
check_eq "HEAD status" "200" "$STATUS"
body_empty "HEAD response has no body"

request "$PORT" GET /t/pass-status
check_eq "upstream status is passed through" "201" "$STATUS"
body_eq "upstream body is passed through" "created"
header_contains "upstream header is passed through" "X-Backend: header-value"
header_contains "upstream content type is passed through" "Content-Type: text/plain"

request "$PORT" GET /t/pass-5xx
check_eq "upstream 5xx is passed through" "500" "$STATUS"
body_eq "upstream 5xx body is passed through" "upstream-error"

request "$PORT" GET /t/network-error
check_eq "network error becomes 502" "502" "$STATUS"

request "$PORT" GET "/t/placeholders?q=hello&v=header"
check_eq "placeholder status" "200" "$STATUS"
body_eq "URL and header placeholders are replaced" "method=GET uri=/echo?q=hello auth= x=header ct="

request "$PORT" GET "/t/param-placeholder?v=abc"
check_eq "param placeholder status" "200" "$STATUS"
body_eq "param placeholders are replaced" "method=GET uri=/?q=abc auth= x= ct="

request "$PORT" GET "/t/body-placeholder?v=xyz"
check_eq "body placeholder status" "200" "$STATUS"
body_eq "body placeholders are replaced" "xyz"

request "$PORT" GET /t/terminal
check_eq "terminal status" "200" "$STATUS"
body_eq "forward answers before the next handler" "backend-ok"
body_absent "forward never calls the next handler" "next-handler"

# ── Test 2: template functions ────────────────────────────────────────────
echo ""
echo "=== Test 2: template functions ==="

request "$PORT" GET /t/tpl/template-get.html
check_eq "template GET status" "200" "$STATUS"
body_eq "template GET body" "backend-ok"
header_contains "templates answer as HTML" "Content-Type: text/html"

request "$PORT" GET /t/tpl/template-status.html
body_eq "template exposes the upstream status" "200"

request "$PORT" GET /t/tpl/template-headers.html
body_eq "template exposes upstream headers" "header-value"

request "$PORT" GET /t/tpl/template-pipe.html
body_eq "template pipe passes data to POST" "piped-data"

request "$PORT" GET /t/tpl/template-head.html
body_eq "template HEAD returns its status" "200"

request "$PORT" GET /t/tpl/template-delete.html
body_eq "template DELETE reaches the upstream" "method=DELETE uri=/q auth= x= ct="

request "$PORT" GET /t/tpl/template-options.html
body_eq "template OPTIONS reaches the upstream" "method=OPTIONS uri=/q auth= x= ct="

request "$PORT" GET /t/tpl/template-error.html
body_eq "template failure sets .Err without aborting" "error"

request "$PORT" GET /t/tpl/template-use.html
body_eq "template USE passes the context through" "before(child:/template-use.html)after"

request "$PORT" GET /t/tpl/template-use-pipe.html
body_eq "template USE works as a pipe target" "child:/template-use-pipe.html"

request "$PORT" GET /t/tpl/template-get-dict.html
body_eq "template GET accepts a header dict" "method=GET uri=/ auth= x=from-dict ct="

request "$PORT" GET /t/tpl/template-query-dict.html
body_eq "template GET accepts a query dict" "method=GET uri=/?a=b auth= x= ct="

request "$PORT" GET /t/tpl/template-post-form.html
body_eq "template POST encodes a form dict" "a=b"

request "$PORT" GET /t/tpl/template-post-multipart.html
body_contains "template POST encodes a multipart part" 'name="a"'
body_contains "template POST encodes a multipart value" "b"

request "$PORT" GET /t/tpl/template-post-invalid-ct.html
body_eq "template POST rejects a non-form content type" "error"

# ── Test 3: forward USE ───────────────────────────────────────────────────
echo ""
echo "=== Test 3: forward USE ==="

request "$PORT" GET /t/use
check_eq "forward USE status" "200" "$STATUS"
body_eq "forward USE renders the file with query and body" "usefile:/t/use q=caddy b=hello body"
header_contains "forward USE answers as HTML" "Content-Type: text/html"

request "$PORT" GET /t/use-api
check_eq "forward USE inherits template functions" "200" "$(cat "$TMP/body")"

request "$PORT" GET /t/use-missing
check_eq "forward USE reports a missing file" "500" "$STATUS"

# ── Test 4: max_response_size ─────────────────────────────────────────────
echo ""
echo "=== Test 4: max_response_size ==="

if ! SIZE_PORT=$(start_container "$CONTAINER_SIZE" "$ROOT/test/e2e/size.Caddyfile"); then
	fail "start size container" "docker run failed"
else
	if ! wait_ready "$SIZE_PORT" /small; then
		fail_start "$CONTAINER_SIZE"
	fi
	request "$SIZE_PORT" GET /small
	check_eq "response under the cap succeeds" "200" "$STATUS"
	body_eq "response under the cap is intact" "small-ok"
	request "$SIZE_PORT" GET /big
	check_eq "response over the cap becomes 502" "502" "$STATUS"
fi

# ── Test 5: timeout ───────────────────────────────────────────────────────
echo ""
echo "=== Test 5: timeout ==="

if ! TIMEOUT_PORT=$(start_container "$CONTAINER_TIMEOUT" "$ROOT/test/e2e/timeout.Caddyfile"); then
	fail "start timeout container" "docker run failed"
else
	if ! wait_ready "$TIMEOUT_PORT" /slow; then
		fail_start "$CONTAINER_TIMEOUT"
	fi
	request "$TIMEOUT_PORT" GET /slow
	check_eq "expired timeout becomes 502" "502" "$STATUS"
fi

# ── Test 6: validation ────────────────────────────────────────────────────
echo ""
echo "=== Test 6: validation ==="

validate_ok "main Caddyfile validates" "$ROOT/test/e2e/Caddyfile"
validate_ok "size Caddyfile validates" "$ROOT/test/e2e/size.Caddyfile"
validate_ok "timeout Caddyfile validates" "$ROOT/test/e2e/timeout.Caddyfile"

NEG="$TMP/neg"
mkdir -p "$NEG"

cat > "$NEG/top-level.Caddyfile" <<'CADDY'
{
	admin off
}
:18080 {
	forward GET http://127.0.0.1:2081/
}
:2081 {
	bind 127.0.0.1
	respond "ok"
}
CADDY

cat > "$NEG/valid-options.Caddyfile" <<'CADDY'
{
	admin off
	forward {
		timeout           1s
		max_response_size 100
	}
}
:18080 {
	route /x {
		forward POST http://127.0.0.1:2081/ {
			param  q caddy
			header X-Test yes
			body   "body"
		}
		forward USE page.html {
			param q caddy
			body  "body"
		}
	}
}
CADDY

cat > "$NEG/invalid-method.Caddyfile" <<'CADDY'
{
	admin off
}
:18080 {
	route /x {
		forward FOO http://127.0.0.1:2081/
	}
}
CADDY

cat > "$NEG/connect.Caddyfile" <<'CADDY'
{
	admin off
}
:18080 {
	route /x {
		forward CONNECT http://127.0.0.1:2081/
	}
}
CADDY

cat > "$NEG/trace.Caddyfile" <<'CADDY'
{
	admin off
}
:18080 {
	route /x {
		forward TRACE http://127.0.0.1:2081/
	}
}
CADDY

cat > "$NEG/body-on-get.Caddyfile" <<'CADDY'
{
	admin off
}
:18080 {
	route /x {
		forward GET http://127.0.0.1:2081/ {
			body "nope"
		}
	}
}
CADDY

cat > "$NEG/use-header.Caddyfile" <<'CADDY'
{
	admin off
}
:18080 {
	route /x {
		forward USE page.html {
			header X-Test y
		}
	}
}
CADDY

cat > "$NEG/use-no-file.Caddyfile" <<'CADDY'
{
	admin off
}
:18080 {
	route /x {
		forward USE
	}
}
CADDY

cat > "$NEG/missing-url.Caddyfile" <<'CADDY'
{
	admin off
}
:18080 {
	route /x {
		forward GET
	}
}
CADDY

cat > "$NEG/extra-arg.Caddyfile" <<'CADDY'
{
	admin off
}
:18080 {
	route /x {
		forward GET http://127.0.0.1:2081/ extra
	}
}
CADDY

cat > "$NEG/unknown-subdirective.Caddyfile" <<'CADDY'
{
	admin off
}
:18080 {
	route /x {
		forward GET http://127.0.0.1:2081/ {
			foo bar
		}
	}
}
CADDY

cat > "$NEG/invalid-timeout.Caddyfile" <<'CADDY'
{
	admin off
	forward {
		timeout nope
	}
}
:18080 {
	respond "ok"
}
CADDY

cat > "$NEG/invalid-max-size.Caddyfile" <<'CADDY'
{
	admin off
	forward {
		max_response_size nope
	}
}
:18080 {
	respond "ok"
}
CADDY

cat > "$NEG/unknown-option.Caddyfile" <<'CADDY'
{
	admin off
	forward {
		unknown 1
	}
}
:18080 {
	respond "ok"
}
CADDY

validate_ok "top-level forward is ordered" "$NEG/top-level.Caddyfile"
validate_ok "forward options and every subdirective validate" "$NEG/valid-options.Caddyfile"

validate_fail "invalid method is rejected" "$NEG/invalid-method.Caddyfile" "invalid method: FOO"
validate_fail "CONNECT is rejected" "$NEG/connect.Caddyfile" "invalid method: CONNECT"
validate_fail "TRACE is rejected" "$NEG/trace.Caddyfile" "invalid method: TRACE"
validate_fail "body on GET is rejected" "$NEG/body-on-get.Caddyfile" "body subdirective is only allowed on POST, PUT, PATCH"
validate_fail "USE header is rejected" "$NEG/use-header.Caddyfile" "USE takes query and body, not header"
validate_fail "USE without a file is rejected" "$NEG/use-no-file.Caddyfile" "wrong argument count"
validate_fail "missing URL is rejected" "$NEG/missing-url.Caddyfile" "wrong argument count"
validate_fail "extra argument is rejected" "$NEG/extra-arg.Caddyfile" "wrong argument count"
validate_fail "unknown subdirective is rejected" "$NEG/unknown-subdirective.Caddyfile" "unrecognized subdirective: foo"
validate_fail "invalid timeout is rejected" "$NEG/invalid-timeout.Caddyfile" "invalid timeout"
validate_fail "invalid max_response_size is rejected" "$NEG/invalid-max-size.Caddyfile" "invalid max_response_size"
validate_fail "unknown global option is rejected" "$NEG/unknown-option.Caddyfile" "unrecognized option: unknown"

# ── Summary ───────────────────────────────────────────────────────────────
echo ""
echo "=========================================="
printf '  PASS: %s   FAIL: %s\n' "$PASS" "$FAIL"
echo "=========================================="
if [ "$FAIL" -eq 0 ]; then
	echo "ALL TESTS PASSED"
	exit 0
fi
echo "SOME TESTS FAILED"
exit 1
