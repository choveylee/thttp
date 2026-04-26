// Package thttp provides an HTTP client built on [net/http] with optional layered
// [http.RoundTripper] implementations for structured logging, retries, Prometheus
// latency metrics, and request/response hooks.
//
// Construct a dedicated client with [NewHttpClient] when you need custom transport
// settings, proxies, TLS, or hooks. For simple one-off calls, package-level helpers
// such as [Get] and [PostJson] use a process-wide default client.
//
// Per-request overrides are expressed with [RequestOption] and merged into each
// invocation of [HttpClient.Do].
//
// Errors returned by this package use the "thttp:" prefix. Configuration and
// option type mismatches follow a consistent "invalid <option> value: want
// <type>, got <dynamic type>" style to make misconfiguration easier to spot.
//
// Structured log messages emitted by the built-in transports also use a
// consistent "thttp ..." prefix, for example "thttp slow request" and
// "thttp access log".
package thttp
