# caddy-forward

Imagine calling an API was easy again. No sidecar, no second service, no
`reverse_proxy` gymnastics just to make one outbound request from config. That
is the goal of `caddy-forward`.

1. **Forward handler** (`pkg/forward/`) — the `forward` directive. One outbound
   request per match, and the response status, headers, and body go straight
   back to the client. It does not call `next`. It is the endpoint now.
2. **Template functions** (`pkg/template/`) — `GET`, `HEAD`, `POST`, `PUT`,
   `PATCH`, `DELETE`, `OPTIONS`, and `USE` inside Caddy templates. Fetch an API
   while you render a page, because apparently we do network I/O in templates
   now. `USE` renders another template file in place and carries the current
   variables through, so you are not remapping them by hand.
3. **Request engine** (`pkg/subreq/`) — the shared HTTP client, the response
   size cap, and the body encoding (form, multipart, raw). The part that
   actually talks to the internet.
4. **Global config** (`pkg/config/`) — `forward { timeout max_response_size }`,
   the two dials you will actually want to turn.

---

## Quickstart

```bash
xcaddy build --with github.com/frinknet/caddy-forward
```

```caddyfile
:80 {
    route /api/* {
        forward POST https://api.example.com/v1/echo {
            header Content-Type application/json
            body   `{"msg":"hello"}`
        }
    }
}
```

```bash
curl -i http://localhost/api/
# → whatever api.example.com replied, status and headers and all
```

Placeholders are fair game in the URL, params, headers, and body:

```caddyfile
:80 {
    route /lookup/* {
        forward GET "https://api.example.com/search?q={http.request.uri.query.q}"
    }
}
```

---

## Use Cases

### Expose an upstream API behind your own domain

`forward` terminates the request. Whatever the outbound call returns is what
the client gets, headers and status included. Use it to reshape a request, add
a secret `Authorization` header, or hit a backend nobody else can reach.

```caddyfile
api.example.com {
    route /v1/* {
        forward GET "https://internal.backend/v1{http.request.uri.path}" {
            header Authorization "Bearer {env.API_TOKEN}"
            header X-Forwarded-For "{http.request.remote.host}"
        }
    }
}
```

### Inject request context into a template fetch

The template functions take a URL, a headers map, and a data value, and hand
back a response you can read field by field. Great for server-side rendering a
dashboard from three internal APIs and a prayer.

```caddyfile
example.com {
    root * /srv/www
    templates {
        extensions {
            forward_methods
        }
    }
    file_server
}
```

```gotemplate
Status: {{ (GET "https://api.example.com/status" nil nil).Status }}
Body:   {{ GET "https://api.example.com/status" nil nil }}
CT:     {{ (GET "https://api.example.com/status" nil nil).Headers.Get "Content-Type" }}
```

### Relay a form submission

POST a dict and let the plugin pick the encoding. No `Content-Type` means
`application/x-www-form-urlencoded`; a `multipart/form-data` header (or any
`[]byte` value) means multipart.

```gotemplate
{{ POST "https://api.example.com/contact" (dict "Content-Type" "application/x-www-form-urlencoded") (dict "name" "Bob" "msg" "hi") }}
```

### Pipe a value into a POST

Pipe passes its value as the data argument, because that is how Go templates
work and we are not fighting it.

```gotemplate
{{ "piped-data" | POST "https://api.example.com/echo" nil }}
```

### Docker

```bash
./build.sh          # builds caddy-forward:test
docker run -d -p 80:80 -v /path/to/Caddyfile:/etc/caddy/Caddyfile caddy-forward:test
```

See the `Dockerfile` for the scratch image — a few MB, caddy, toybox, and a
prayer.

---

## Forward Directive Reference

```caddyfile
forward METHOD url {
    param  NAME VALUE
    header NAME VALUE
    body   STRING
}
```

| Subdirective | Default | Description |
|---|---|---|
| `param`  | — | Append a URL query parameter. Repeatable. |
| `header` | — | Set an outbound request header. Repeatable. |
| `body`   | — | Outbound request body. POST, PUT, PATCH only. |

There is also `USE`, which renders a template file instead of making a request.
It is covered in its own section below.

Methods fall into two camps, and where your data goes depends on which you pick:

| Class | Methods | Data goes to |
|---|---|---|
| Query | `GET` `HEAD` `DELETE` `OPTIONS` | the query string (`param`) |
| Body  | `POST` `PUT` `PATCH` | the request body (`body`) |
| File  | `USE` | the request, which the rendered template reads |

Rules and consequences:

- The method is uppercased for you. Anything outside the eight above is a
  config error, and there is no `TRACE`.
- `body` on a query method is a hard error at startup. You have been warned.
- `url` is required and goes through the Caddy replacer, as do every param
  name, param value, header value, and body string.
- The outbound status code, headers (added, not replaced), and body come back
  to the client unchanged.
- An upstream `5xx` is passed through as-is. It is not the plugin's problem.
- A network failure (DNS, TLS, timeout, response over the size cap) becomes a
  `502 Bad Gateway`.

---

## USE: serve exactly that template

`USE` is the eighth method, and the only one that does not touch the network. It
renders a template file and returns the result, so it works as a one-file
`file_server` with templating, or as a component inside a bigger page.

### In the Caddyfile

```caddyfile
route /t-use {
    root * /srv/www
    forward USE page.html {
        param q caddy
        body  "hello body"
    }
}
```

`root` sets the file root (`{http.vars.root}`); the path is relative to it. Like
the other methods, `param` and `body` are accepted and run through the replacer.
Since there is no outbound request, they land on the request itself, which the
template can read:

```gotemplate
usefile:{{ placeholder "http.request.uri.path" }}
q={{ placeholder "http.request.uri.query.q" }}
b={{ placeholder "http.request.body" }}
```

`header` is rejected on `USE`, because there is no outbound request to set it on.

### In a template

`{{ USE "file" . }}` works like Caddy's `include`, with one difference: it hands
the current template context to the child, so existing variables are already in
scope. Pass the dot explicitly; Go template functions never see it otherwise.

```gotemplate
{{ USE "partials/header.html" . }}
{{ . | USE "partials/row.html" }}
```

The child gets everything the parent has: `.Req`, `.Args`, placeholders, and all
the `forward_methods` functions, including a nested `USE`. To hand the child a
different dot while still resolving the path against the site root, pass the
context first and the data last:

```gotemplate
{{ range .Items }}{{ . | USE "partials/row.html" $ctx }}{{ end }}
```

Without a context to find the file root, `USE` errors and the page fails to
render, which is the loud failure you want.

---

## Template Functions Reference

The `forward_methods` extension registers one function per method:

```gotemplate
{{ GET     url headers data }}
{{ HEAD    url headers data }}
{{ POST    url headers data }}
{{ PUT     url headers data }}
{{ PATCH   url headers data }}
{{ DELETE  url headers data }}
{{ OPTIONS url headers data }}
{{ USE     "file" . }}
```

`headers` is a map (or `nil`). `data` is a string, a dict, or `nil`. Each call
returns a response with these fields:

| Field | Type | Notes |
|---|---|---|
| `.Body`   | `string` | Empty for `HEAD`. |
| `.Status` | `int` | The upstream status code. |
| `.Headers`| `http.Header` | Call `.Get "Content-Type"` etc. |
| `.Err`    | `error` | Set on failure. |

Printing the response directly (`{{ GET ... }}`) prints the body. A failed
request does **not** abort rendering — it sets `.Err` and prints an empty
string. Check `.Err` if you care.

```caddyfile
templates {
    extensions {
        forward_methods
    }
}
```

### Data encoding

| Method | `data` type | Result |
|---|---|---|
| Query | `string` | Appended verbatim to the query string. |
| Query | `dict` | Merged into the query string and re-encoded. |
| Body  | `string` | Raw body. Defaults `Content-Type: text/plain`. |
| Body  | `dict`, no CT or form CT | `application/x-www-form-urlencoded`. |
| Body  | `dict` with `multipart/form-data` CT or any `[]byte` value | `multipart/form-data`. |
| Body  | `dict` with any other CT | Error: `dict body needs a form Content-Type`. |

`[]byte` values in a body dict become file parts named after their keys. Build
your uploads in a template and go.

---

## Global Config Reference

```caddyfile
{
    forward {
        timeout           10s
        max_response_size 10485760
    }
}
```

| Option | Default | Description |
|---|---|---|
| `timeout` | `5s` | Outbound request timeout for the handler and template functions. |
| `max_response_size` | `10485760` (10 MiB) | Response body cap, in bytes. Exceeding it is a `502` (handler) or `.Err` (template). |

Both apply to the `forward` handler and every template function. Skip the block
and you get the defaults.

---

## Troubleshooting

| Error | Fix |
|---|---|
| `forward: url is required` | Give the directive a URL. |
| `invalid method: FOO` | Only `GET HEAD POST PUT PATCH DELETE OPTIONS USE`. |
| `forward: unsupported method "CONNECT"` | Same list, enforced again at validate time. |
| `forward: body subdirective is only allowed on POST, PUT, PATCH` | Move the body, or the method. |
| `forward: USE requires a template file` | `USE` needs a path, not nothing. |
| `forward: USE takes query and body, not header` | Drop the `header` block; there is no outbound request. |
| `USE ...: needs the template context` | Pass it: `{{ USE "file" . }}`. |
| `unrecognized subdirective: foo` | Only `param`, `header`, `body` exist. |
| `502 Bad Gateway` | The outbound call failed. Check logs for DNS, TLS, or timeout. |
| `response body exceeds max size` | Raise `max_response_size`. |
| `dict body needs a form Content-Type` | Drop the odd `Content-Type`, or send a string. |
| Template prints the call literally | The template handler is not configured with `forward_methods`, or the file is not being processed. |

---

## Limitations

| What | Status | Deal with it |
|---|---|---|
| Streaming responses | ❌ | The whole body is buffered, up to the cap. |
| Retries, load balancing | ❌ | This is one request. Use `reverse_proxy` for the grown-up stuff. |
| Per-route timeout | ❌ | It is global. Set it and live with it. |
| Calling `next` | ❌ | `forward` is terminal. Chain it last. |
| Extra methods (`TRACE`, `CONNECT`) | ❌ | Not today. |
| `USE` with a header | ❌ | It takes query and body, not outbound headers. |
| Implicit parent context in `USE` | ❌ | Go template functions cannot see the dot; pass it. |
| Rich header types in templates | ⏳ | Flat maps only. |
| Streaming uploads from `forward` | ⏳ | A string body or nothing. |

---

## Testing

Unit tests run inside the pinned builder image:

```bash
sh test/unit.sh
```

The end-to-end suite runs against the image built by `./build.sh`:

```bash
./build.sh
E2E_IMAGE=caddy-forward:test sh test/e2e.sh
```

The e2e suite exercises every method, placeholders, response passthrough, the
template functions, `USE`, the response-size cap, timeouts, and Caddyfile
validation. It starts Caddy with `test/e2e/Caddyfile` and uses loopback
backends inside the container, so it needs no external network.

