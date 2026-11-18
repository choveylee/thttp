# thttp

HTTP client utilities for Go that extend [`net/http`](https://pkg.go.dev/net/http) with composable [`RoundTripper`](https://pkg.go.dev/net/http#RoundTripper) layers, optional retries, structured logging, Prometheus-compatible request latency metrics, and request / response hooks.

Module path: [`github.com/choveylee/thttp`](https://pkg.go.dev/github.com/choveylee/thttp).

---

## Requirements

- Go **1.27.4** or later (see [`go.mod`](go.mod)).

---

## Installation

```bash
go get github.com/choveylee/thttp
```

---

## Overview

The package offers two usage modes:

1. **Dedicated client** — Create an [`HttpClient`](https://pkg.go.dev/github.com/choveylee/thttp#HttpClient) with [`NewHttpClient`](https://pkg.go.dev/github.com/choveylee/thttp#NewHttpClient). You receive an isolated [`http.Transport`](https://pkg.go.dev/net/http#Transport), option map, and default headers. A zero-valued `HttpClient` is supported: the transport is allocated lazily on the first call to [`Do`](https://pkg.go.dev/github.com/choveylee/thttp#HttpClient.Do) or [`Transport`](https://pkg.go.dev/github.com/choveylee/thttp#HttpClient.Transport).

2. **Package-level API** — Functions such as [`Get`](https://pkg.go.dev/github.com/choveylee/thttp#Get) and [`PostJson`](https://pkg.go.dev/github.com/choveylee/thttp#PostJson) delegate to an internal default client. Use this only when shared process-wide defaults are acceptable.

Per-request settings are supplied through [`RequestOption`](https://pkg.go.dev/github.com/choveylee/thttp#RequestOption) and merged with client defaults on each `Do`.

---

## Features

| Area | Description |
|------|-------------|
| **Transport options** | Proxy URL or [`ProxyFunc`](https://pkg.go.dev/github.com/choveylee/thttp#ProxyFunc), connection pool limits, TLS (`InsecureSkipVerify`, custom [`tls.Config`](https://pkg.go.dev/crypto/tls#Config)). |
| **Logging transport** | [`LogTransOption`](https://pkg.go.dev/github.com/choveylee/thttp#LogTransOption): slow-request logs, optional access logs, latency in milliseconds, Prometheus histogram. |
| **Retry transport** | [`RetryTransOption`](https://pkg.go.dev/github.com/choveylee/thttp#RetryTransOption) configures policy, backoff, number of attempts and an optional error hook. |
| **Hooks** | [`RequestHookFunc`](https://pkg.go.dev/github.com/choveylee/thttp#RequestHookFunc) / [`ResponseHookFunc`](https://pkg.go.dev/github.com/choveylee/thttp#ResponseHookFunc) via client or per-request options. |
| **Helpers** | JSON and multipart helpers, query-string utilities, reverse-proxy-oriented accessors ([`GetRealIP`](https://pkg.go.dev/github.com/choveylee/thttp#GetRealIP), etc.). |

Transport decoration order for an outgoing request: **retry (outer)** → **logging** → **base `http.Transport`**.

Retries require an idempotent method or a non-empty `Idempotency-Key` or `X-Idempotency-Key` header. Use an idempotency key only when the downstream service honors it. The first attempt uses the supplied body. Later attempts reopen it through `Request.GetBody`. The standard library supplies this function for byte and string readers. A one-shot stream is sent once. The request context and client timeout also cover backoff, including waits requested by `Retry-After`.

Cookies supplied by `RequestOption` use a temporary jar for the HTTP call, including redirects. Domain, path, expiry and `Secure` checks apply before sending them. Explicit cookies take precedence over stored cookies of the same name when their scope matches. The default jar stores server cookies and enforces public-suffix rules. `WithCookieJar(nil)` disables server cookie storage for the selected scope. JSON and multipart helpers set their own content type for each call, so they can share request options.

When a retry error hook replaces a response body, it owns the original body. If the new body wraps the original, its `Close` method must propagate to the original. The transport closes a body when the hook returns no body.

`Response.ToBytes` and `ToString` close the response body. They return an empty result for HEAD, 204 and 304 responses. When a response still has `Content-Encoding` metadata, they decode supported gzip and deflate layers in reverse order. If an encoding is unsupported, they return the original representation.

The proxy header helpers assume a trusted reverse proxy that replaces client-supplied forwarding headers. `GetRealIP` parses IPv4 and IPv6 and uses the peer address when it finds no valid proxy-set IP.

---

## Errors and logs

Errors defined by this package use the **`thttp:`** prefix. Configuration and option type mismatches identify the option and expected type, for example `thttp: invalid OptTimeout value: want time.Duration, got string`.

Debug, slow, access and failure logs omit URL paths, query strings, user info, header values, bodies and raw transport errors. Debug logging shows header names with redacted values. `IncludeHeaders` enables the same presentation for access logs. The histogram measures each transport attempt through response headers or transport failure.

An invalid batch of transport options retains the current transport. Subsequent `Do` calls return the configuration error until a later transport update succeeds. In a batch, a proxy function takes precedence over a proxy URL, and `OptTransUnsafeTls` takes precedence over the matching TLS config field. An empty proxy URL clears the proxy.

[`RequestOption`](https://pkg.go.dev/github.com/choveylee/thttp#RequestOption) does not expose a generic option setter; use typed methods (e.g. [`WithLogTransOption`](https://pkg.go.dev/github.com/choveylee/thttp#RequestOption.WithLogTransOption)) so logging and retry options are always shallow-copied when stored.

---

## Example: dedicated client

```go
ctx := context.Background()

client := thttp.NewHttpClient().
	WithTimeout(10 * time.Second).
	WithLogTransOption(thttp.NewLogTransOption().WithAccessLog(true))

resp, err := client.Get(ctx, "https://example.com", thttp.NewRequestOption(), nil)
if err != nil {
	// handle error
}
code, body, err := resp.ToBytes()
if err != nil {
	// handle error
}
_ = code
_ = body
```

## Example: default client and JSON

```go
ctx := context.Background()

resp, err := thttp.PostJson(ctx, "https://example.com/api", nil, map[string]any{"key": "value"})
if err != nil {
	// handle error
}
defer resp.Body.Close()
```

---

## Documentation

- **Package overview:** see [`doc.go`](doc.go) and [pkg.go.dev](https://pkg.go.dev/github.com/choveylee/thttp) after publication.
- **Command line:** `go doc github.com/choveylee/thttp`

---

## License

Refer to the repository’s license file for terms of use.
