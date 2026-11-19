#!/bin/sh

timeout 280 docker run --rm \
  -v caddy-sf-mod:/mod \
  -v "$PWD":/src \
  -w /src \
  -e GOMODCACHE=/mod \
  -e GOCACHE=/mod/gocache \
  -e CGO_ENABLED=0 caddy:2.11.9-builder-alpine \
  sh -c 'unformatted=$(gofmt -l .); if [ -n "$unformatted" ]; then echo "gofmt needed:"; echo "$unformatted"; exit 1; fi; go vet ./... && go test -timeout=120s ./...'
