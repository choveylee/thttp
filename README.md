# thttp

Package [`github.com/choveylee/thttp`](https://pkg.go.dev/github.com/choveylee/thttp) provides an HTTP client built on [`net/http`](https://pkg.go.dev/net/http) with optional logging, retries, Prometheus latency metrics, and request/response hooks.

## Requirements

- Go 1.25 or later (see `go.mod`).

## Features

- **Configurable client**: `NewHttpClient` with defaults for transport, timeouts, and cookie jar.
- **Per-request overrides**: `RequestOption` merges with client defaults for headers, cookies, timeout, logging, and retry settings.
- **Transport middleware**: logging (`LogTransOption`) and retry (`RetryTransOption`) wrap the base `http.Transport`.
- **Metrics**: request latency histogram (`http_client_request_latency` via `tmetric`).
- **Helpers**: JSON and multipart helpers, query-string utilities, and reverse-proxy-oriented request accessors (`GetRealIP`, `GetRealHost`, `GetRealPort`).

## Installation

```bash
go get github.com/choveylee/thttp
```

## Usage

Use the package-level functions with the internal default client for simple calls:

```go
ctx := context.Background()
resp, err := thttp.Get(ctx, "https://example.com", nil, nil)
if err != nil {
    // handle error
}
defer resp.Body.Close()
code, body, err := resp.ToBytes()
```

Configure a dedicated client when you need custom transports, proxies, or hooks:

```go
client := thttp.NewHttpClient().
    WithTimeout(10 * time.Second).
    WithLogTransOption(thttp.NewLogTransOption().WithAccessLog(true))

resp, err := client.Get(ctx, "https://example.com", thttp.NewRequestOption(), nil)
```

For JSON POST:

```go
resp, err := thttp.PostJson(ctx, "https://example.com/api", nil, map[string]any{"key": "value"})
```

## Documentation

- Run `go doc github.com/choveylee/thttp` or open the package on [pkg.go.dev](https://pkg.go.dev/github.com/choveylee/thttp) after publishing.
- Package overview and exported symbols are documented in English in the source (`doc.go` and per-symbol comments).

## License

See the repository license file (if any) for terms of use.
