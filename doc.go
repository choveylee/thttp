// Package thttp provides HTTP client utilities built on [net/http], with optional
// layered [http.RoundTripper] implementations for structured logging, retries,
// Prometheus latency metrics, and request / response hooks.
//
// Construct a dedicated client with [NewHttpClient] when custom transport settings,
// proxies, TLS behavior, or hooks are required. For simple one-off calls, package-level
// helpers such as [Get] and [PostJson] use a process-wide default client.
//
// Per-request overrides are expressed with [RequestOption] and merged into each
// invocation of [HttpClient.Do].
//
// Errors returned by this package use the "thttp:" prefix. Configuration and
// option type mismatches follow the consistent form "invalid <option> value:
// want <type>, got <dynamic type>" so that misconfiguration can be identified
// quickly.
//
// Structured log messages emitted by the built-in transports use a consistent,
// descriptive style. Representative messages include "thttp slow request
// observed", "thttp request access log entry", and
// "thttp request failed or returned HTTP status >= 400".
package thttp
