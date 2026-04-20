# thttp

Formal HTTP client utilities for Go, extending [`net/http`](https://pkg.go.dev/net/http) with composable [`RoundTripper`](https://pkg.go.dev/net/http#RoundTripper) layers, optional retries, structured logging, Prometheus-compatible request latency metrics, and request/response hooks.

Module path: [`github.com/choveylee/thttp`](https://pkg.go.dev/github.com/choveylee/thttp).

---

## Requirements

- Go **1.25** or later (see [`go.mod`](go.mod)).

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
| **Retry transport** | [`RetryTransOption`](https://pkg.go.dev/github.com/choveylee/thttp#RetryTransOption): configurable policy, backoff, optional error hook; respects [`Request.GetBody`](https://pkg.go.dev/net/http#Request.GetBody) when set. |
| **Hooks** | [`RequestHookFunc`](https://pkg.go.dev/github.com/choveylee/thttp#RequestHookFunc) / [`ResponseHookFunc`](https://pkg.go.dev/github.com/choveylee/thttp#ResponseHookFunc) via client or per-request options. |
| **Helpers** | JSON and multipart helpers, query-string utilities, reverse-proxy-oriented accessors ([`GetRealIP`](https://pkg.go.dev/github.com/choveylee/thttp#GetRealIP), etc.). |

Transport decoration order for an outgoing request: **retry (outer)** → **logging** → **base `http.Transport`**.

---

## Configuration errors

Invalid option types passed through [`HttpClient.WithOption`](https://pkg.go.dev/github.com/choveylee/thttp#HttpClient.WithOption), [`HttpClient.Defaults`](https://pkg.go.dev/github.com/choveylee/thttp#HttpClient.Defaults), or merged maps during `Do` are reported with errors prefixed by **`thttp:`** and, where applicable, the expected type and `got %T` for the value provided. Transport-related failures from `WithOption` are also recorded; subsequent `Do` calls return that error until a transport option applies successfully.

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
defer resp.Body.Close()

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

- **Package overview:** see [`doc.go`](doc.go) and [pkg.go.dev](https://pkg.go.dev/github.com/choveylee/thttp) after publishing.
- **Command line:** `go doc github.com/choveylee/thttp`

---

## License

Refer to the repository’s license file for terms of use.
