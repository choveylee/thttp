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
// Errors returned by this package that describe invalid configuration or option
// types use the "thttp:" prefix and, where applicable, include the dynamic type of
// the offending value (via %T) to aid debugging.
package thttp
