#!/bin/sh

timeout 280 docker run --rm \
  -v caddy-sf-mod:/mod \
  -v "$PWD":/src \
  -w /src \
  -e GOMODCACHE=/mod \
  -e GOCACHE=/mod/gocache \
  -e CGO_ENABLED=1 caddy:2.11.9-builder-alpine \
  sh -c 'apk add --no-cache gcc musl-dev && go test -race -timeout=180s ./...'
