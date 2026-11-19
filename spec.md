# caddy-forward specification

This is the target design. `README.md` documents what ships today; this file
documents where the plugin is going. Where the two disagree, the README wins
for current behavior and this file wins for intent. `BACKLOG.md` holds the
concrete gap between them.

## Purpose

`caddy-forward` makes one outbound HTTP request from Caddy configuration and
returns that response to the client, without a sidecar, a second service, or a
`reverse_proxy` block. It also exposes the same request engine to Caddy
templates, so a rendered page can fetch an API during rendering.

The plugin is for request shaping and small integrations. It is not a general
purpose reverse proxy, load balancer, or API gateway. If the requirement is
long-lived proxying, use `reverse_proxy`.

## Principles

1. **One request, one answer.** The handler is terminal. It does not call
   `next` and has no fallthrough mode.
2. **Configuration is the API.** Everything a request needs is declared in the
   Caddyfile or JSON. No Go code, no environment-specific plugins.
3. **Predictable passthrough.** The upstream status, headers, and body reach
   the client unchanged unless a feature explicitly says otherwise.
4. **Bounded work.** Every outbound call has a timeout and a response size
   limit. Neither can be unbounded.
5. **Loud configuration errors.** A bad method, a misplaced body, or an
   unknown subdirective fails at parse or validate time, not at request time.
6. **Small surface.** Features land only when they compose with the existing
   directive and do not turn the plugin into `reverse_proxy`.

## Public surface

### Caddyfile

```caddyfile
forward METHOD url {
    param  NAME VALUE
    header NAME VALUE
    body   STRING
}
```

- `METHOD` is one of `GET`, `HEAD`, `POST`, `PUT`, `PATCH`, `DELETE`,
  `OPTIONS`, or the pseudo-method `USE`.
- `url`, param names and values, header values, and `body` support Caddy
  placeholders.
- `param` and `header` are repeatable. Repeated entries keep order and
  multiplicity.
- `header` names are literal.
- `body` is rejected on query methods and on `USE` targets that do not read a
  body.

Global options:

```caddyfile
{
    forward {
        timeout           5s
        max_response_size 10485760
    }
}
```

### JSON

The handler module ID is `http.handlers.forward`. The global app ID is
`forward`. Param and header entries are arrays of name/value objects. The
template extension module ID is
`http.handlers.templates.functions.forward_methods` and takes no configuration.

JSON field names are stable. The handler currently uses its Go field names
(capitalized) while the global app uses lowercase tags; decoding is
case-insensitive. Omitted fields take their default or zero value. Giving the
handler explicit lowercase tags is a backlog item, not a shipped behavior.

### Template functions

The `forward_methods` template extension registers:

```gotemplate
{{ GET     url headers data }}
{{ HEAD    url headers data }}
{{ POST    url headers data }}
{{ PUT     url headers data }}
{{ PATCH   url headers data }}
{{ DELETE  url headers data }}
{{ OPTIONS url headers data }}
{{ USE     "file" context }}
```

Each network call returns an object with `Body`, `Status`, `Headers`, and
`Err`. A failed call never aborts rendering; it reports the failure through
`Err`.

## Behavior contract

### Forward handler

1. The method is uppercased and validated.
2. Placeholders are resolved against the current request.
3. Params are merged into the target URL query string.
4. Headers are set on the outbound request.
5. Body methods send the configured body; query methods send params only.
6. The outbound request runs with the global timeout and size cap.
7. The response status, headers, and body are written to the client.
8. Any transport failure or size-cap violation is a `502 Bad Gateway`.
9. An upstream `4xx` or `5xx` is passed through unchanged.
10. `forward` never calls the next handler.

### USE handler

1. The file path is resolved against `{http.vars.root}`.
2. Params are merged into the receiving request's query string.
3. A configured body replaces the receiving request body.
4. The template renders with the full Caddy template context and the
   `forward_methods` functions.
5. The result is written as `text/html; charset=utf-8` unless the template sets
   another content type.
6. A missing file or template error is a `500`.

### Template calls

1. Query methods merge string data into the query string verbatim, or a dict
   as re-encoded key/value pairs.
2. Body methods send a string as raw content, or a dict as form or multipart
   content depending on the `Content-Type` and value types.
3. `HEAD` discards the body but keeps status and headers.
4. Errors land in `.Err`; `.Status` is zero and `.Body` is empty.
5. `USE` renders a file with the passed context and inherits every template
   function, including nested `USE`.

### Limits

- Default timeout: 5 seconds.
- Default maximum response size: 10 MiB.
- Exceeding either limit is a request failure, never a partial success.
- A configured value of zero or less currently means "use the default".

## Target behavior

These are the intended additions. None are guaranteed to ship; each must earn
its place against the principles above.

### Requests

- **Retries.** A configurable retry count and backoff for idempotent methods,
  with a clear rule for which status codes and errors are retryable.
- **Per-route overrides.** `timeout` and `max_response_size` inside the
  directive, overriding the global values.
- **Forward the incoming body.** A way to send the request body that arrived
  at Caddy, instead of a literal string, without buffering it twice.
- **Redirect control.** A policy for following, limiting, or refusing
  redirects. Today the default Go policy follows up to 10.
- **TLS options.** Custom roots, client certificates, and an explicit
  `insecure_skip_verify` switch for internal backends.
- **Cookie jar.** Optional per-handler or shared storage so a sequence of
  requests can carry cookies.
- **More methods.** `TRACE` and `CONNECT` remain out of scope unless a concrete
  use case appears.
- **Response shaping.** Optional header allow/deny lists and status remapping
  for cases where passthrough is not the right answer.

### Templates

- **Request-aware calls.** Template fetches should inherit the request context
  so a client disconnect cancels the outbound call.
- **Per-call options.** A headers map plus timeout or encoding hints, instead
  of only the global defaults.
- **Structured data.** A response object that can expose multiple values per
  header and a typed body (bytes or JSON) instead of a string.
- **Streaming uploads.** Multipart construction that does not require
  `[]byte` to already fit in memory.

### Operations

- **Metrics.** Request count, latency, upstream status, and failure class per
  directive, in a form Caddy can expose.
- **Structured logs.** Debug-level outbound request summaries without leaking
  secrets or bodies.
- **Config schema.** A JSON schema and validation for direct JSON configs,
  plus stable lowercase JSON field names on the handler.

## Security

- Never log configured header values, bodies, or URLs at info level; query
  strings and headers routinely carry tokens.
- Keep placeholders out of the outbound body unless the operator asks for
  them.
- Do not add an option that disables TLS verification without making the
  operator write it explicitly.
- Bound every response and every request. No unbounded reads.
- Treat the upstream response as untrusted: do not reflect its headers into
  Caddy's own control headers.

## Performance

- The handler performs one outbound call and at most one buffered response.
- The shared client must be safe for concurrent use.
- A slow upstream must not block other requests beyond its own timeout.
- Memory per request is bounded by `max_response_size`, plus encoding overhead
  for multipart bodies.
- Streaming, when it lands, must not change the bounded-memory guarantee.

## Testing

- Unit tests cover parsing, validation, encoding, limits, and template
  behavior next to the code they exercise.
- Unit tests run in the pinned builder image through `sh test/unit.sh`.
- The race detector runs whenever concurrency changes.
- End-to-end tests run against the image built by `./build.sh` through
  `sh test/e2e.sh`, using loopback backends and exact byte comparisons.
- Every new behavior needs an assertion that fails without the change.
- Broken Caddyfiles and JSON configs are validated for the exact error text.

## Non-goals

- Load balancing across upstreams.
- Long-lived streaming proxying or WebSocket tunneling.
- Rewriting request or response bodies as a general transform.
- Acting as a caching layer.
- Replacing `reverse_proxy` for normal reverse-proxy duties.
