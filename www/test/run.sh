#!/bin/sh
# Template integration tests — run inside container against a live caddy.

PASS=0
FAIL=0

ok()   { PASS=$((PASS+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); echo "  FAIL: $1  (got: $2, expected: $3)"; }

# Start caddy with test config in background
echo "=== Starting caddy (test config) ==="
CADDY_CONF=/srv/www/test/Caddyfile
/bin/caddy run --config "$CADDY_CONF" --adapter caddyfile >/tmp/caddy.log 2>&1 &
CADDY_PID=$!
sleep 1

# Helper: fetch a URL and compare body
check() {
	name="$1"
	url="$2"
	expect="$3"
	body=$(/bin/toybox timeout 2 /bin/toybox wget -O - "http://127.0.0.1:2080${url}" 2>&1)
	code=$?
	if [ "$code" != 0 ]; then
		fail "$name" "wget exit $code: $body" "$expect"
		return
	fi
	if [ "$body" = "$expect" ]; then
		ok "$name"
	else
		fail "$name" "$body" "$expect"
	fi
}

echo ""
echo "=== Running template integration tests ==="

# 1. forward GET
check "forward GET"         "/t-forward"              "backend-ok"

# 2. forward with params
check "forward params"      "/t-forward-params"        "backend-ok"

# 3. forward with headers
check "forward headers"     "/t-forward-headers"       "backend-ok"

# 4. forward POST with body
check "forward POST"        "/t-forward-post"          "hello body"

# 5. template GET function
check "template GET"        "/t/template-get.html"     "backend-ok"

# 6. template .Status access
check "template Status"     "/t/template-status.html"  "200"

# 7. template pipe data into POST
check "template pipe"       "/t/template-pipe.html"    "piped-data"

# 8. template HEAD
check "template HEAD"       "/t/template-head.html"    ""

# 9. template DELETE
check "template DELETE"     "/t/template-delete.html"  "DELETE http://127.0.0.1:2082/"

# 10. template OPTIONS
check "template OPTIONS"    "/t/template-options.html" "OPTIONS http://127.0.0.1:2082/"

# 11. forward USE (serves exactly that template, with query and body)
check "forward USE"         "/t-use"                   "usefile:/t-use q=caddy b=hello body"

# 12. template USE (include with context passthrough)
check "template USE"        "/t/template-use.html"     "before(child:/template-use.html)after"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

# Stop caddy
kill "$CADDY_PID" 2>/dev/null
wait "$CADDY_PID" 2>/dev/null

if [ "$FAIL" -eq 0 ]; then
	exit 0
else
	exit 1
fi